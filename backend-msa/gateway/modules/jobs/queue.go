package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const pending = "noplanner:evaluation:pending"
const processing = "noplanner:evaluation:processing"

type Payload struct {
	Owner       string `json:"owner"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	FileName    string `json:"fileName"`
	MediaType   string `json:"mediaType"`
	Country     string `json:"country"`
	Domain      string `json:"domain"`
	TargetUser  string `json:"targetUser"`
	Document    []byte `json:"document"`
}
type Job struct {
	ID          string    `json:"id"`
	Owner       string    `json:"owner"`
	Status      string    `json:"status"`
	Stage       string    `json:"stage"`
	ReportID    string    `json:"reportId,omitempty"`
	Error       string    `json:"error,omitempty"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"maxAttempts"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Payload     Payload   `json:"payload"`
}
type Queue struct {
	client *redis.Client
	lease  time.Duration
}

func New(rawURL string) (*Queue, error) {
	o, e := redis.ParseURL(rawURL)
	if e != nil {
		return nil, e
	}
	c := redis.NewClient(o)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = c.Ping(ctx).Err(); e != nil {
		_ = c.Close()
		return nil, e
	}
	return &Queue{client: c, lease: 5 * time.Minute}, nil
}
func (q *Queue) Close() error { return q.client.Close() }
func key(id string) string    { return "noplanner:evaluation:job:" + id }

func (q *Queue) Enqueue(ctx context.Context, p Payload, idempotency string) (Job, error) {
	if idempotency == "" {
		sum := sha256.Sum256(append([]byte(p.Owner+p.ProjectID+p.FileName), p.Document...))
		idempotency = hex.EncodeToString(sum[:])
	}
	idemKey := "noplanner:evaluation:idem:" + idempotency
	if id, e := q.client.Get(ctx, idemKey).Result(); e == nil {
		return q.Get(ctx, id, p.Owner)
	} else if !errors.Is(e, redis.Nil) {
		return Job{}, e
	}
	now := time.Now().UTC()
	j := Job{ID: uuid.NewString(), Owner: p.Owner, Status: "queued", Stage: "queued", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now, Payload: p}
	raw, _ := json.Marshal(j)
	ok, e := q.client.SetNX(ctx, idemKey, j.ID, 24*time.Hour).Result()
	if e != nil {
		return Job{}, e
	}
	if !ok {
		id, e := q.client.Get(ctx, idemKey).Result()
		if e != nil {
			return Job{}, e
		}
		return q.Get(ctx, id, p.Owner)
	}
	pipe := q.client.TxPipeline()
	pipe.Set(ctx, key(j.ID), raw, 7*24*time.Hour)
	pipe.LPush(ctx, pending, j.ID)
	_, e = pipe.Exec(ctx)
	return j, e
}
func (q *Queue) Get(ctx context.Context, id, owner string) (Job, error) {
	raw, e := q.client.Get(ctx, key(id)).Bytes()
	if e != nil {
		return Job{}, e
	}
	var j Job
	if e = json.Unmarshal(raw, &j); e != nil {
		return Job{}, e
	}
	if owner != "" && j.Owner != owner {
		return Job{}, errors.New("job not found")
	}
	return j, nil
}
func (q *Queue) save(ctx context.Context, j Job) error {
	j.UpdatedAt = time.Now().UTC()
	raw, _ := json.Marshal(j)
	return q.client.Set(ctx, key(j.ID), raw, 7*24*time.Hour).Err()
}
func (q *Queue) Progress(ctx context.Context, id, stage string) error {
	j, e := q.Get(ctx, id, "")
	if e != nil {
		return e
	}
	j.Stage = stage
	j.Status = "running"
	return q.save(ctx, j)
}

type Handler func(context.Context, string, Payload) (string, error)

func (q *Queue) Run(ctx context.Context, h Handler) {
	q.recover(ctx)
	for {
		if ctx.Err() != nil {
			return
		}
		id, e := q.client.BLMove(ctx, pending, processing, "RIGHT", "LEFT", 5*time.Second).Result()
		if errors.Is(e, redis.Nil) {
			continue
		}
		if e != nil {
			time.Sleep(time.Second)
			continue
		}
		q.execute(ctx, id, h)
	}
}
func (q *Queue) execute(ctx context.Context, id string, h Handler) {
	j, e := q.Get(ctx, id, "")
	if e != nil {
		_ = q.client.LRem(ctx, processing, 1, id).Err()
		return
	}
	if j.Attempts >= j.MaxAttempts {
		j.Status = "failed"
		j.Stage = "failed"
		if j.Error == "" {
			j.Error = "maximum attempts reached"
		}
		_ = q.save(ctx, j)
		_ = q.client.LRem(ctx, processing, 1, id).Err()
		return
	}
	j.Attempts++
	j.Status = "running"
	j.Stage = "starting"
	_ = q.save(ctx, j)
	report, e := h(ctx, id, j.Payload)
	if e == nil {
		j.Status = "completed"
		j.Stage = "completed"
		j.ReportID = report
		j.Error = ""
		_ = q.save(ctx, j)
		_ = q.client.LRem(ctx, processing, 1, id).Err()
		return
	}
	j.Error = e.Error()
	if j.Attempts < j.MaxAttempts {
		j.Status = "queued"
		j.Stage = "retrying"
		_ = q.save(ctx, j)
		p := q.client.TxPipeline()
		p.LRem(ctx, processing, 1, id)
		p.LPush(ctx, pending, id)
		_, _ = p.Exec(ctx)
		return
	}
	j.Status = "failed"
	j.Stage = "failed"
	_ = q.save(ctx, j)
	_ = q.client.LRem(ctx, processing, 1, id).Err()
}
func (q *Queue) recover(ctx context.Context) {
	ids, e := q.client.LRange(ctx, processing, 0, -1).Result()
	if e != nil {
		return
	}
	for _, id := range ids {
		j, e := q.Get(ctx, id, "")
		if e != nil {
			continue
		}
		if j.Attempts >= j.MaxAttempts {
			j.Status = "failed"
			j.Stage = "failed"
			j.Error = fmt.Sprintf("worker interrupted after final attempt %d", j.Attempts)
			_ = q.save(ctx, j)
			_ = q.client.LRem(ctx, processing, 1, id).Err()
			continue
		}
		j.Status = "queued"
		j.Stage = "recovered"
		j.Error = fmt.Sprintf("worker interrupted after attempt %d", j.Attempts)
		_ = q.save(ctx, j)
		p := q.client.TxPipeline()
		p.LRem(ctx, processing, 1, id)
		p.LPush(ctx, pending, id)
		_, _ = p.Exec(ctx)
	}
}
