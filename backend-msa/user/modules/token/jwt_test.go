package token

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestIssueAndVerifyRequiredClaims(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 2, 3, 0, time.UTC)
	manager := New(testSecret, "issuer", "audience", time.Hour)
	manager.now = func() time.Time { return now }

	raw, expires, err := manager.Issue("google-subject")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := manager.Verify(raw)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Subject != "google-subject" || !identity.Expires.Equal(expires) || !expires.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected identity: %#v, expires=%v", identity, expires)
	}

	claims := &Claims{}
	if _, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return []byte(testSecret), nil }); err != nil {
		t.Fatal(err)
	}
	if claims.Subject == "" || claims.Issuer != "issuer" || len(claims.Audience) != 1 || claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.ID == "" {
		t.Fatalf("required claims are incomplete: %#v", claims)
	}
}

func TestVerifyRejectsExpiredTamperedAndMissingClaims(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 2, 3, 0, time.UTC)
	manager := New(testSecret, "issuer", "audience", time.Hour)
	manager.now = func() time.Time { return now }
	raw, _, err := manager.Issue("google-subject")
	if err != nil {
		t.Fatal(err)
	}

	manager.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := manager.Verify(raw); err != ErrInvalid {
		t.Fatalf("expired Verify() error = %v", err)
	}
	manager.now = func() time.Time { return now }
	if _, err := manager.Verify(raw + "tampered"); err != ErrInvalid {
		t.Fatalf("tampered Verify() error = %v", err)
	}

	missingExpiry := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: "subject", Issuer: "issuer", Audience: jwt.ClaimStrings{"audience"},
		IssuedAt: jwt.NewNumericDate(now), ID: "id",
	}})
	missingExpiryRaw, err := missingExpiry.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Verify(missingExpiryRaw); err != ErrInvalid {
		t.Fatalf("missing exp Verify() error = %v", err)
	}
}

func TestUnaryServerProtectsSelectedMethod(t *testing.T) {
	manager := New(testSecret, "issuer", "audience", time.Hour)
	raw, _, err := manager.Issue("google-subject")
	if err != nil {
		t.Fatal(err)
	}
	interceptor := manager.UnaryServer("/protected")
	handler := func(ctx context.Context, _ any) (any, error) {
		identity, ok := IdentityFromContext(ctx)
		if !ok || identity.Subject != "google-subject" {
			t.Fatalf("missing identity: %#v", identity)
		}
		return "ok", nil
	}

	if _, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/protected"}, handler); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing header code = %s", status.Code(err))
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+raw))
	result, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/protected"}, handler)
	if err != nil || result != "ok" {
		t.Fatalf("protected call = %v, %v", result, err)
	}
	result, err = interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/public"}, func(context.Context, any) (any, error) { return "public", nil })
	if err != nil || result != "public" {
		t.Fatalf("public call = %v, %v", result, err)
	}
}
