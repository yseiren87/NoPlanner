package intelligence

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"noplanner/backend/intelligence/modules/provider"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

type Service struct {
	intelligencev1.UnimplementedIntelligenceServiceServer
	client        Client
	name, version string
}

type Client interface {
	Generate(context.Context, string, bool, uint32) (string, error)
	Search(context.Context, string) (provider.SearchResult, error)
	LLMIdentity() (string, string)
	SearchIdentity() (string, string)
}

func New(client Client, name, version string) *Service {
	return &Service{client: client, name: name, version: version}
}

func (s *Service) GetStatus(context.Context, *commonv1.StatusRequest) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{Service: s.name, Version: s.version, State: "ready"}, nil
}

func (s *Service) Generate(ctx context.Context, request *intelligencev1.GenerateRequest) (*intelligencev1.GenerateResponse, error) {
	if strings.TrimSpace(request.GetPrompt()) == "" {
		return nil, status.Error(codes.InvalidArgument, "prompt is required")
	}
	text, err := s.client.Generate(ctx, request.GetPrompt(), request.GetJsonResponse(), request.GetMaxTokens())
	if errors.Is(err, provider.ErrNotConfigured) {
		return nil, status.Error(codes.FailedPrecondition, "llm provider is not configured")
	}
	var requestError *provider.RequestError
	if errors.As(err, &requestError) {
		if requestError.StatusCode == 401 || requestError.StatusCode == 403 {
			return nil, status.Error(codes.Unauthenticated, "llm provider rejected its credentials")
		}
		if requestError.StatusCode == 400 || requestError.StatusCode == 404 {
			return nil, status.Error(codes.FailedPrecondition, "llm provider rejected its model or request configuration")
		}
		if requestError.StatusCode == 429 {
			return nil, status.Error(codes.ResourceExhausted, "llm provider quota is exhausted")
		}
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "llm provider request failed")
	}
	providerName, model := s.client.LLMIdentity()
	return &intelligencev1.GenerateResponse{Text: text, Provider: providerName, Model: model}, nil
}

func (s *Service) SearchWeb(ctx context.Context, request *intelligencev1.SearchWebRequest) (*intelligencev1.SearchWebResponse, error) {
	if strings.TrimSpace(request.GetQuery()) == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	result, err := s.client.Search(ctx, request.GetQuery())
	if errors.Is(err, provider.ErrNotConfigured) {
		return nil, status.Error(codes.FailedPrecondition, "search provider is not configured")
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "search provider request failed")
	}
	providerName, model := s.client.SearchIdentity()
	response := &intelligencev1.SearchWebResponse{ExecutedQueries: result.ExecutedQueries, Provider: providerName, Model: model}
	for _, source := range result.Sources {
		response.Sources = append(response.Sources, &intelligencev1.WebSource{Title: source.Title, Url: source.URL, Snippet: source.Snippet, Publisher: source.Publisher})
	}
	return response, nil
}
