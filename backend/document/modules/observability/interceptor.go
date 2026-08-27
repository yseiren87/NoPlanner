package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	commonv1 "noplanner/backend/proto/dist/golang/common/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	RequestIDHeader    = "x-request-id"
	EvaluationIDHeader = "x-evaluation-id"
)

type contextKey string

const (
	requestIDKey    contextKey = RequestIDHeader
	evaluationIDKey contextKey = EvaluationIDHeader
)

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func EvaluationID(ctx context.Context) string {
	value, _ := ctx.Value(evaluationIDKey).(string)
	return value
}

func WithIDs(ctx context.Context, requestID, evaluationID string) context.Context {
	ctx = context.WithValue(ctx, requestIDKey, requestID)
	return context.WithValue(ctx, evaluationIDKey, evaluationID)
}

func UnaryServer(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		requestID := metadataValue(ctx, RequestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
		}
		evaluationID := metadataValue(ctx, EvaluationIDHeader)
		ctx = WithIDs(ctx, requestID, evaluationID)
		_ = grpc.SetHeader(ctx, metadata.Pairs(RequestIDHeader, requestID))

		started := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				err = internalError(ctx)
			}
			logger.InfoContext(ctx, "grpc_request",
				"method", info.FullMethod,
				"grpc_code", status.Code(err).String(),
				"duration_ms", time.Since(started).Milliseconds(),
				"request_id", requestID,
				"evaluation_id", evaluationID,
			)
		}()
		return handler(ctx, req)
	}
}

func UnaryClient() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		pairs := make([]string, 0, 4)
		if requestID := RequestID(ctx); requestID != "" {
			pairs = append(pairs, RequestIDHeader, requestID)
		}
		if evaluationID := EvaluationID(ctx); evaluationID != "" {
			pairs = append(pairs, EvaluationIDHeader, evaluationID)
		}
		if len(pairs) > 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func metadataValue(ctx context.Context, key string) string {
	values := metadata.ValueFromIncomingContext(ctx, key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(value[:])
}

func internalError(ctx context.Context) error {
	base := status.New(codes.Internal, "요청을 처리하지 못했습니다.")
	detail := &commonv1.ErrorDetail{
		Code:         commonv1.ErrorCode_ERROR_CODE_INTERNAL,
		RequestId:    RequestID(ctx),
		EvaluationId: EvaluationID(ctx),
		Retryable:    true,
	}
	withDetail, err := base.WithDetails(detail)
	if err != nil {
		return base.Err()
	}
	return withDetail.Err()
}
