package status

import (
	"context"

	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
)

type Service struct {
	generationv1.UnimplementedGenerationServiceServer
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
