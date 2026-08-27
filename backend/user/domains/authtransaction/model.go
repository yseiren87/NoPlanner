package authtransaction

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

var ErrInvalidState = errors.New("invalid or expired oauth state")

type transaction struct {
	verifier  string
	expiresAt time.Time
}

type Store struct {
	mu           sync.Mutex
	transactions map[string]transaction
	now          func() time.Time
	ttl          time.Duration
}

func NewStore(ttl time.Duration) *Store {
	return &Store{transactions: make(map[string]transaction), now: time.Now, ttl: ttl}
}

func (s *Store) Begin() (state, verifier, challenge string, err error) {
	state, err = randomValue(32)
	if err != nil {
		return "", "", "", err
	}
	verifier, err = randomValue(48)
	if err != nil {
		return "", "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpired()
	s.transactions[state] = transaction{verifier: verifier, expiresAt: s.now().Add(s.ttl)}
	return state, verifier, challenge, nil
}

func (s *Store) Consume(state string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, found := s.transactions[state]
	delete(s.transactions, state)
	if !found || !s.now().Before(item.expiresAt) {
		return "", ErrInvalidState
	}
	return item.verifier, nil
}

func (s *Store) removeExpired() {
	for state, item := range s.transactions {
		if !s.now().Before(item.expiresAt) {
			delete(s.transactions, state)
		}
	}
}

func randomValue(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
