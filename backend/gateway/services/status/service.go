package status

import (
	"context"

	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	gatewayv1 "noplanner/backend/proto/dist/golang/gateway/v1"
)

type Service struct {
	gatewayv1.UnimplementedGatewayServiceServer
	name    string
	version string
}

func New(name, version string) *Service {
	return &Service{name: name, version: version}
}

func (s *Service) GetStatus(context.Context, *commonv1.StatusRequest) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{
		Service: s.name,
		Version: s.version,
		State:   "ready",
	}, nil
}
