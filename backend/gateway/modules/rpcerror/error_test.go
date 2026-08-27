package rpcerror

import (
	"context"
	"testing"

	"noplanner/backend/gateway/modules/observability"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewBuildsStablePublicErrorContract(t *testing.T) {
	ctx := observability.WithIDs(context.Background(), "request-error", "evaluation-error")
	err := New(ctx, commonv1.ErrorCode_ERROR_CODE_CONFIGURATION_REQUIRED, "설정이 필요합니다.", false)

	result := status.Convert(err)
	if result.Code() != codes.FailedPrecondition || result.Message() != "설정이 필요합니다." {
		t.Fatalf("unexpected status: %v", result)
	}
	detail, ok := result.Details()[0].(*commonv1.ErrorDetail)
	if !ok || detail.RequestId != "request-error" || detail.EvaluationId != "evaluation-error" || detail.Retryable {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}
