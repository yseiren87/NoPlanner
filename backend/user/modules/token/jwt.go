package token

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	ErrUnavailable = errors.New("jwt is not configured")
	ErrInvalid     = errors.New("invalid jwt")
)

type Identity struct {
	Subject string
	Expires time.Time
}

type Claims struct {
	jwt.RegisteredClaims
}

type Manager struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
	now      func() time.Time
}

type identityKey struct{}

func New(secret, issuer, audience string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), issuer: issuer, audience: audience, ttl: ttl, now: time.Now}
}

func (m *Manager) Issue(subject string) (string, time.Time, error) {
	if !m.configured() {
		return "", time.Time{}, ErrUnavailable
	}
	if subject == "" {
		return "", time.Time{}, ErrInvalid
	}
	now := m.now().UTC()
	expires := now.Add(m.ttl)
	identifier, err := randomID()
	if err != nil {
		return "", time.Time{}, err
	}
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: subject, Issuer: m.issuer, Audience: jwt.ClaimStrings{m.audience},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), ID: identifier,
	}}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

func (m *Manager) Verify(raw string) (Identity, error) {
	if !m.configured() {
		return Identity{}, ErrUnavailable
	}
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalid
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(m.now))
	if err != nil || !parsed.Valid || claims.Subject == "" || claims.ID == "" || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return Identity{}, ErrInvalid
	}
	return Identity{Subject: claims.Subject, Expires: claims.ExpiresAt.Time}, nil
}

func (m *Manager) UnaryServer(protectedMethods ...string) grpc.UnaryServerInterceptor {
	protected := make(map[string]struct{}, len(protectedMethods))
	for _, method := range protectedMethods {
		protected[method] = struct{}{}
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, required := protected[info.FullMethod]; !required {
			return handler(ctx, req)
		}
		values := metadata.ValueFromIncomingContext(ctx, "authorization")
		if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "인증이 필요합니다.")
		}
		identity, err := m.Verify(strings.TrimSpace(strings.TrimPrefix(values[0], "Bearer ")))
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "유효하지 않거나 만료된 인증입니다.")
		}
		return handler(context.WithValue(ctx, identityKey{}, identity), req)
	}
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok
}

func (m *Manager) configured() bool {
	return len(m.secret) >= 32 && m.issuer != "" && m.audience != "" && m.ttl > 0
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
