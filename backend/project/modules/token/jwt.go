package token

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Claims struct {
	jwt.RegisteredClaims
}

type Verifier struct {
	secret   []byte
	issuer   string
	audience string
}

type identityKey struct{}

func New(secret, issuer, audience string) *Verifier {
	return &Verifier{secret: []byte(secret), issuer: issuer, audience: audience}
}

func (v *Verifier) UnaryServer(publicMethods ...string) grpc.UnaryServerInterceptor {
	public := make(map[string]struct{}, len(publicMethods))
	for _, method := range publicMethods {
		public[method] = struct{}{}
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, ok := public[info.FullMethod]; ok {
			return handler(ctx, req)
		}
		values := metadata.ValueFromIncomingContext(ctx, "authorization")
		if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "인증이 필요합니다.")
		}
		subject, ok := v.verify(strings.TrimSpace(strings.TrimPrefix(values[0], "Bearer ")))
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "유효하지 않거나 만료된 인증입니다.")
		}
		return handler(context.WithValue(ctx, identityKey{}, subject), req)
	}
}

func Subject(ctx context.Context) (string, bool) {
	subject, ok := ctx.Value(identityKey{}).(string)
	return subject, ok && subject != ""
}

func WithSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, identityKey{}, subject)
}

func (v *Verifier) verify(raw string) (string, bool) {
	if len(v.secret) < 32 || v.issuer == "" || v.audience == "" {
		return "", false
	}
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(value *jwt.Token) (any, error) {
		if value.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrSignatureInvalid
		}
		return v.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !parsed.Valid || claims.Subject == "" || claims.ID == "" || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return "", false
	}
	return claims.Subject, true
}
