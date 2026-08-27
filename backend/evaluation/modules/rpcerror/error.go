package rpcerror

import (
	"context"

	"noplanner/backend/evaluation/modules/observability"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func New(ctx context.Context, code commonv1.ErrorCode, userMessage string, retryable bool) error {
	base := status.New(grpcCode(code), userMessage)
	detail := &commonv1.ErrorDetail{
		Code:         code,
		RequestId:    observability.RequestID(ctx),
		EvaluationId: observability.EvaluationID(ctx),
		Retryable:    retryable,
	}
	withDetail, err := base.WithDetails(detail)
	if err != nil {
		return base.Err()
	}
	return withDetail.Err()
}

func grpcCode(code commonv1.ErrorCode) codes.Code {
	switch code {
	case commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT:
		return codes.InvalidArgument
	case commonv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED:
		return codes.Unauthenticated
	case commonv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED:
		return codes.PermissionDenied
	case commonv1.ErrorCode_ERROR_CODE_NOT_FOUND:
		return codes.NotFound
	case commonv1.ErrorCode_ERROR_CODE_CONFLICT:
		return codes.AlreadyExists
	case commonv1.ErrorCode_ERROR_CODE_DEPENDENCY_UNAVAILABLE:
		return codes.Unavailable
	case commonv1.ErrorCode_ERROR_CODE_CONFIGURATION_REQUIRED:
		return codes.FailedPrecondition
	case commonv1.ErrorCode_ERROR_CODE_RATE_LIMITED:
		return codes.ResourceExhausted
	default:
		return codes.Internal
	}
}
