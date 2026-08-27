package project

import (
	"context"
	"errors"

	projectdomain "noplanner/backend/project/domains/project"
	"noplanner/backend/project/modules/rpcerror"
	"noplanner/backend/project/modules/token"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	projectv1 "noplanner/backend/proto/dist/golang/project/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	projectv1.UnimplementedProjectServiceServer
	store   projectdomain.Store
	name    string
	version string
}

func New(store projectdomain.Store, name, version string) *Service {
	return &Service{store: store, name: name, version: version}
}

func (s *Service) GetStatus(context.Context, *commonv1.StatusRequest) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{Service: s.name, Version: s.version, State: "ready"}, nil
}

func (s *Service) CreateProject(ctx context.Context, request *projectv1.CreateProjectRequest) (*projectv1.Project, error) {
	subject, ok := token.Subject(ctx)
	if !ok {
		return nil, unauthenticated(ctx)
	}
	value, err := s.store.Create(ctx, subject, request.GetName())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	return projectMessage(value), nil
}

func (s *Service) ListProjects(ctx context.Context, _ *projectv1.ListProjectsRequest) (*projectv1.ProjectList, error) {
	subject, ok := token.Subject(ctx)
	if !ok {
		return nil, unauthenticated(ctx)
	}
	values, err := s.store.List(ctx, subject)
	if err != nil {
		return nil, mapError(ctx, err)
	}
	result := &projectv1.ProjectList{}
	for _, value := range values {
		result.Projects = append(result.Projects, projectMessage(value))
	}
	return result, nil
}

func (s *Service) GetProject(ctx context.Context, request *projectv1.GetProjectRequest) (*projectv1.Project, error) {
	subject, ok := token.Subject(ctx)
	if !ok {
		return nil, unauthenticated(ctx)
	}
	value, err := s.store.Get(ctx, subject, request.GetProjectId())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	if !value.Role.CanRead() {
		return nil, mapError(ctx, projectdomain.ErrForbidden)
	}
	return projectMessage(value), nil
}

func (s *Service) UpdateProject(ctx context.Context, request *projectv1.UpdateProjectRequest) (*projectv1.Project, error) {
	subject, ok := token.Subject(ctx)
	if !ok {
		return nil, unauthenticated(ctx)
	}
	value, err := s.store.Update(ctx, subject, request.GetProjectId(), request.GetName())
	if err != nil {
		return nil, mapError(ctx, err)
	}
	return projectMessage(value), nil
}

func (s *Service) SetProjectMemberRole(ctx context.Context, request *projectv1.SetProjectMemberRoleRequest) (*projectv1.ProjectMember, error) {
	subject, ok := token.Subject(ctx)
	if !ok {
		return nil, unauthenticated(ctx)
	}
	member, err := s.store.SetMemberRole(ctx, subject, request.GetProjectId(), request.GetSubject(), domainRole(request.GetRole()))
	if err != nil {
		return nil, mapError(ctx, err)
	}
	return &projectv1.ProjectMember{ProjectId: member.ProjectID, Subject: member.Subject, Role: protoRole(member.Role)}, nil
}

func (s *Service) UpdateOutputLanguage(ctx context.Context, request *projectv1.UpdateOutputLanguageRequest) (*projectv1.Project, error) {
	subject, ok := token.Subject(ctx)
	if !ok {
		return nil, unauthenticated(ctx)
	}
	language := domainLanguage(request.GetOutputLanguage())
	value, err := s.store.UpdateOutputLanguage(ctx, subject, request.GetProjectId(), language)
	if err != nil {
		return nil, mapError(ctx, err)
	}
	return projectMessage(value), nil
}

func projectMessage(value projectdomain.Project) *projectv1.Project {
	return &projectv1.Project{
		Id: value.ID, Name: value.Name, CallerRole: protoRole(value.Role),
		CreatedAt: timestamppb.New(value.CreatedAt), UpdatedAt: timestamppb.New(value.UpdatedAt),
		OutputLanguage: protoLanguage(value.OutputLanguage),
	}
}

func domainLanguage(value commonv1.OutputLanguage) string {
	if value == commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN {
		return "ko"
	}
	if value == commonv1.OutputLanguage_OUTPUT_LANGUAGE_ENGLISH {
		return "en"
	}
	return ""
}
func protoLanguage(value string) commonv1.OutputLanguage {
	if value == "ko" {
		return commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN
	}
	if value == "en" {
		return commonv1.OutputLanguage_OUTPUT_LANGUAGE_ENGLISH
	}
	return commonv1.OutputLanguage_OUTPUT_LANGUAGE_UNSPECIFIED
}

func domainRole(role projectv1.ProjectRole) projectdomain.Role {
	switch role {
	case projectv1.ProjectRole_PROJECT_ROLE_OWNER:
		return projectdomain.RoleOwner
	case projectv1.ProjectRole_PROJECT_ROLE_EDITOR:
		return projectdomain.RoleEditor
	case projectv1.ProjectRole_PROJECT_ROLE_VIEWER:
		return projectdomain.RoleViewer
	default:
		return ""
	}
}

func protoRole(role projectdomain.Role) projectv1.ProjectRole {
	switch role {
	case projectdomain.RoleOwner:
		return projectv1.ProjectRole_PROJECT_ROLE_OWNER
	case projectdomain.RoleEditor:
		return projectv1.ProjectRole_PROJECT_ROLE_EDITOR
	case projectdomain.RoleViewer:
		return projectv1.ProjectRole_PROJECT_ROLE_VIEWER
	default:
		return projectv1.ProjectRole_PROJECT_ROLE_UNSPECIFIED
	}
}

func unauthenticated(ctx context.Context) error {
	return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "인증이 필요합니다.", false)
}

func mapError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, projectdomain.ErrInvalid):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "프로젝트 입력값을 확인해 주세요.", false)
	case errors.Is(err, projectdomain.ErrNotFound):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_NOT_FOUND, "프로젝트를 찾을 수 없습니다.", false)
	case errors.Is(err, projectdomain.ErrForbidden):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "프로젝트에 대한 권한이 없습니다.", false)
	case errors.Is(err, projectdomain.ErrLastOwner):
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_CONFLICT, "프로젝트에는 소유자가 한 명 이상 필요합니다.", false)
	default:
		return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "프로젝트 요청을 처리하지 못했습니다.", true)
	}
}
