package status

import (
	"context"

	commonv1 "noplanner/proto/common/v1"
	researchv1 "noplanner/proto/research/v1"
)

type Service struct {
	researchv1.UnimplementedResearchServiceServer
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
