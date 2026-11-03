package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestVerifyRejectsInvalidSecurityClaims(t *testing.T) {
	verifier := New(testSecret, "issuer", "audience")
	now := time.Now().UTC()

	tests := []struct {
		name     string
		secret   string
		issuer   string
		audience string
		expires  *jwt.NumericDate
	}{
		{"tampered signature", "abcdef0123456789abcdef0123456789", "issuer", "audience", jwt.NewNumericDate(now.Add(time.Hour))},
		{"wrong issuer", testSecret, "other-issuer", "audience", jwt.NewNumericDate(now.Add(time.Hour))},
		{"wrong audience", testSecret, "issuer", "other-audience", jwt.NewNumericDate(now.Add(time.Hour))},
		{"expired", testSecret, "issuer", "audience", jwt.NewNumericDate(now.Add(-time.Minute))},
		{"missing exp", testSecret, "issuer", "audience", nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
				Subject: "subject", Issuer: test.issuer, Audience: jwt.ClaimStrings{test.audience},
				IssuedAt: jwt.NewNumericDate(now), ExpiresAt: test.expires, ID: "token-id",
			}}
			raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(test.secret))
			if err != nil {
				t.Fatal(err)
			}
			if subject, ok := verifier.verify(raw); ok || subject != "" {
				t.Fatalf("verify() accepted %s", test.name)
			}
		})
	}
}

func TestVerifyAcceptsRequiredClaims(t *testing.T) {
	now := time.Now().UTC()
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: "subject", Issuer: "issuer", Audience: jwt.ClaimStrings{"audience"},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), ID: "token-id",
	}}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	if subject, ok := New(testSecret, "issuer", "audience").verify(raw); !ok || subject != "subject" {
		t.Fatalf("verify() = %q, %t", subject, ok)
	}
}
