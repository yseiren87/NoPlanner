package authtransaction

import (
	"testing"
	"time"
)

func TestTransactionCanOnlyBeConsumedOnce(t *testing.T) {
	store := NewStore(10 * time.Minute)
	state, verifier, challenge, err := store.Begin()
	if err != nil || state == "" || verifier == "" || challenge == "" {
		t.Fatalf("Begin() = %q, %q, %q, %v", state, verifier, challenge, err)
	}
	consumed, err := store.Consume(state)
	if err != nil || consumed != verifier {
		t.Fatalf("Consume() = %q, %v", consumed, err)
	}
	if _, err := store.Consume(state); err != ErrInvalidState {
		t.Fatalf("second Consume() error = %v, want ErrInvalidState", err)
	}
}

func TestExpiredTransactionIsRejected(t *testing.T) {
	now := time.Now()
	store := NewStore(time.Minute)
	store.now = func() time.Time { return now }
	state, _, _, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := store.Consume(state); err != ErrInvalidState {
		t.Fatalf("Consume() error = %v, want ErrInvalidState", err)
	}
}
