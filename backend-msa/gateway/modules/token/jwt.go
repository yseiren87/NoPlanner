package token

import (
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Verifier struct {
	secret           []byte
	issuer, audience string
}

func New(secret, issuer, audience string) *Verifier {
	return &Verifier{[]byte(secret), issuer, audience}
}

func (v *Verifier) Verify(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("missing token")
	}
	claims := jwt.MapClaims{}
	t, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("invalid signing method")
		}
		return v.secret, nil
	}, jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !t.Valid {
		return "", errors.New("invalid token")
	}
	sub, err := claims.GetSubject()
	if err != nil || strings.TrimSpace(sub) == "" {
		return "", errors.New("missing subject")
	}
	for _, key := range []string{"jti", "iat", "exp"} {
		if _, ok := claims[key]; !ok {
			return "", errors.New("missing claim")
		}
	}
	return sub, nil
}
