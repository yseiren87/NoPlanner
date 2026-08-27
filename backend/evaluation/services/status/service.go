package status

import (
	"context"

	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
)

type Service struct {
	evaluationv1.UnimplementedEvaluationServiceServer
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
