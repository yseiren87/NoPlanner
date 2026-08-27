package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	commonv1 "noplanner/backend/proto/dist/golang/common/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestUnaryServerPreservesTraceIDsWithoutLoggingSensitiveInput(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	interceptor := UnaryServer(logger)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		RequestIDHeader, "request-123",
		EvaluationIDHeader, "evaluation-456",
		"authorization", "Bearer secret-token",
	))

	_, err := interceptor(ctx, "confidential document body", &grpc.UnaryServerInfo{FullMethod: "/test.Service/Test"},
		func(ctx context.Context, _ any) (any, error) {
			if RequestID(ctx) != "request-123" || EvaluationID(ctx) != "evaluation-456" {
				t.Fatal("trace identifiers were not added to context")
			}
			return &commonv1.StatusResponse{}, nil
		})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}

	logLine := output.String()
	for _, expected := range []string{"request-123", "evaluation-456", "/test.Service/Test"} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("log does not contain %q: %s", expected, logLine)
		}
	}
	for _, secret := range []string{"secret-token", "confidential document body", "authorization"} {
		if strings.Contains(logLine, secret) {
			t.Fatalf("log leaked %q: %s", secret, logLine)
		}
	}
}

func TestUnaryServerConvertsPanicToPublicError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	interceptor := UnaryServer(logger)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(RequestIDHeader, "request-panic"))

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Panic"},
		func(context.Context, any) (any, error) { panic("database password") })
	if status.Code(err).String() != "Internal" {
		t.Fatalf("panic code = %s, want Internal", status.Code(err))
	}
	if strings.Contains(err.Error(), "database password") {
		t.Fatal("panic detail leaked to client error")
	}

	var detail *commonv1.ErrorDetail
	for _, item := range status.Convert(err).Details() {
		if value, ok := item.(*commonv1.ErrorDetail); ok {
			detail = value
		}
	}
	if detail == nil || detail.RequestId != "request-panic" || detail.Code != commonv1.ErrorCode_ERROR_CODE_INTERNAL {
		t.Fatalf("unexpected error detail: %#v", detail)
	}
}

func TestUnaryClientPropagatesTraceIDs(t *testing.T) {
	ctx := WithIDs(context.Background(), "request-client", "evaluation-client")
	err := UnaryClient()(ctx, "/test.Service/Test", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			values, ok := metadata.FromOutgoingContext(ctx)
			if !ok || values.Get(RequestIDHeader)[0] != "request-client" || values.Get(EvaluationIDHeader)[0] != "evaluation-client" {
				t.Fatalf("trace metadata not propagated: %v", values)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("client interceptor returned error: %v", err)
	}
}
