package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func testQueue(t *testing.T) *Queue {
	t.Helper()
	server := miniredis.RunT(t)
	q, e := New("redis://" + server.Addr() + "/0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = q.Close() })
	return q
}
func TestEnqueueIsIdempotentAndOwnerScoped(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	first, e := q.Enqueue(ctx, Payload{Owner: "user-1", FileName: "plan.md", Document: []byte("x")}, "same")
	if e != nil {
		t.Fatal(e)
	}
	second, e := q.Enqueue(ctx, Payload{Owner: "user-1", FileName: "plan.md", Document: []byte("x")}, "same")
	if e != nil || first.ID != second.ID {
		t.Fatalf("idempotency failed: %v %q %q", e, first.ID, second.ID)
	}
	if second.Payload.FileName != "plan.md" || string(second.Payload.Document) != "x" {
		t.Fatalf("payload was not preserved: %+v", second.Payload)
	}
	if _, e = q.Get(ctx, first.ID, "user-2"); e == nil {
		t.Fatal("another owner read the job")
	}
}
func TestWorkerRetriesAndCompletes(t *testing.T) {
	q := testQueue(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job, e := q.Enqueue(ctx, Payload{Owner: "user-1", Document: []byte("x")}, "")
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	go q.Run(ctx, func(context.Context, string, Payload) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("temporary")
		}
		return "report-1", nil
	})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, e := q.Get(ctx, job.ID, "user-1")
		if e == nil && current.Status == "completed" {
			if current.ReportID != "report-1" || current.Attempts != 2 {
				t.Fatalf("unexpected job: %+v", current)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}

func TestRecoverMovesInterruptedJobBackToPending(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	job, err := q.Enqueue(ctx, Payload{Owner: "user-1", Document: []byte("x")}, "recover")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.client.LMove(ctx, pending, processing, "RIGHT", "LEFT").Result(); err != nil {
		t.Fatal(err)
	}
	job.Status = "running"
	job.Stage = "researching"
	job.Attempts = 1
	if err = q.save(ctx, job); err != nil {
		t.Fatal(err)
	}

	q.recover(ctx)

	recovered, err := q.Get(ctx, job.ID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "queued" || recovered.Stage != "recovered" || recovered.Attempts != 1 {
		t.Fatalf("unexpected recovered job: %+v", recovered)
	}
	if got, err := q.client.RPop(ctx, pending).Result(); err != nil || got != job.ID {
		t.Fatalf("job was not returned to pending: got=%q err=%v", got, err)
	}
	if count, err := q.client.LLen(ctx, processing).Result(); err != nil || count != 0 {
		t.Fatalf("processing list was not cleared: count=%d err=%v", count, err)
	}
}

func TestRecoverDoesNotExceedAttemptLimit(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	job, err := q.Enqueue(ctx, Payload{Owner: "user-1", Document: []byte("x")}, "exhausted")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.client.LMove(ctx, pending, processing, "RIGHT", "LEFT").Result(); err != nil {
		t.Fatal(err)
	}
	job.Status = "running"
	job.Attempts = job.MaxAttempts
	if err = q.save(ctx, job); err != nil {
		t.Fatal(err)
	}

	q.recover(ctx)

	recovered, err := q.Get(ctx, job.ID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "failed" || recovered.Stage != "failed" || recovered.Attempts != recovered.MaxAttempts {
		t.Fatalf("unexpected exhausted job: %+v", recovered)
	}
	if count, err := q.client.LLen(ctx, pending).Result(); err != nil || count != 0 {
		t.Fatalf("exhausted job was requeued: count=%d err=%v", count, err)
	}
}
