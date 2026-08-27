package status

import (
	"context"
	"testing"

	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
)

func TestGetStatus(t *testing.T) {
	service := New("research", "0.1.0")
	response, err := service.GetStatus(context.Background(), &commonv1.StatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if response.GetService() != "research" || response.GetVersion() != "0.1.0" || response.GetState() != "ready" {
		t.Fatalf("GetStatus() response = %v", response)
	}
}
