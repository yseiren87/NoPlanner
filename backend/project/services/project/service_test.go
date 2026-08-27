package project

import (
	"context"
	"testing"
	"time"

	projectdomain "noplanner/backend/project/domains/project"
	"noplanner/backend/project/modules/token"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	projectv1 "noplanner/backend/proto/dist/golang/project/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeStore struct {
	projects map[string]projectdomain.Project
	roles    map[string]map[string]projectdomain.Role
}

func newFakeStore() *fakeStore {
	return &fakeStore{projects: map[string]projectdomain.Project{}, roles: map[string]map[string]projectdomain.Role{}}
}

func (s *fakeStore) Create(_ context.Context, subject, name string) (projectdomain.Project, error) {
	value := projectdomain.Project{ID: "project-1", Name: name, CreatedAt: time.Now(), UpdatedAt: time.Now(), Role: projectdomain.RoleOwner}
	s.projects[value.ID] = value
	s.roles[value.ID] = map[string]projectdomain.Role{subject: projectdomain.RoleOwner}
	return value, nil
}
func (s *fakeStore) List(_ context.Context, subject string) ([]projectdomain.Project, error) {
	var values []projectdomain.Project
	for id, value := range s.projects {
		if role, ok := s.roles[id][subject]; ok {
			value.Role = role
			values = append(values, value)
		}
	}
	return values, nil
}

func (s *fakeStore) Get(_ context.Context, subject, projectID string) (projectdomain.Project, error) {
	value, ok := s.projects[projectID]
	role, member := s.roles[projectID][subject]
	if !ok || !member {
		return projectdomain.Project{}, projectdomain.ErrNotFound
	}
	value.Role = role
	return value, nil
}

func (s *fakeStore) Update(_ context.Context, subject, projectID, name string) (projectdomain.Project, error) {
	role := s.roles[projectID][subject]
	if !role.CanEdit() {
		return projectdomain.Project{}, projectdomain.ErrForbidden
	}
	value := s.projects[projectID]
	value.Name, value.Role = name, role
	s.projects[projectID] = value
	return value, nil
}

func (s *fakeStore) SetMemberRole(_ context.Context, actor, projectID, subject string, role projectdomain.Role) (projectdomain.Member, error) {
	if !s.roles[projectID][actor].CanManageMembers() {
		return projectdomain.Member{}, projectdomain.ErrForbidden
	}
	s.roles[projectID][subject] = role
	return projectdomain.Member{ProjectID: projectID, Subject: subject, Role: role}, nil
}

func (s *fakeStore) UpdateOutputLanguage(_ context.Context, subject, projectID, language string) (projectdomain.Project, error) {
	role := s.roles[projectID][subject]
	if !role.CanEdit() {
		return projectdomain.Project{}, projectdomain.ErrForbidden
	}
	value := s.projects[projectID]
	value.OutputLanguage = language
	value.Role = role
	s.projects[projectID] = value
	return value, nil
}

func TestProjectRoleAccess(t *testing.T) {
	store := newFakeStore()
	service := New(store, "project", "test")
	owner := token.WithSubject(context.Background(), "owner")
	created, err := service.CreateProject(owner, &projectv1.CreateProjectRequest{Name: "검증 프로젝트"})
	if err != nil || created.CallerRole != projectv1.ProjectRole_PROJECT_ROLE_OWNER {
		t.Fatalf("CreateProject() = %#v, %v", created, err)
	}
	if _, err := service.SetProjectMemberRole(owner, &projectv1.SetProjectMemberRoleRequest{ProjectId: created.Id, Subject: "editor", Role: projectv1.ProjectRole_PROJECT_ROLE_EDITOR}); err != nil {
		t.Fatalf("add editor: %v", err)
	}
	if _, err := service.SetProjectMemberRole(owner, &projectv1.SetProjectMemberRoleRequest{ProjectId: created.Id, Subject: "viewer", Role: projectv1.ProjectRole_PROJECT_ROLE_VIEWER}); err != nil {
		t.Fatalf("add viewer: %v", err)
	}
	if _, err := service.UpdateProject(token.WithSubject(context.Background(), "editor"), &projectv1.UpdateProjectRequest{ProjectId: created.Id, Name: "수정됨"}); err != nil {
		t.Fatalf("editor update: %v", err)
	}
	viewer := token.WithSubject(context.Background(), "viewer")
	if _, err := service.GetProject(viewer, &projectv1.GetProjectRequest{ProjectId: created.Id}); err != nil {
		t.Fatalf("viewer read: %v", err)
	}
	if _, err := service.UpdateProject(viewer, &projectv1.UpdateProjectRequest{ProjectId: created.Id, Name: "금지"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("viewer update code = %s", status.Code(err))
	}
	if _, err := service.SetProjectMemberRole(token.WithSubject(context.Background(), "editor"), &projectv1.SetProjectMemberRoleRequest{ProjectId: created.Id, Subject: "other", Role: projectv1.ProjectRole_PROJECT_ROLE_OWNER}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("editor role management code = %s", status.Code(err))
	}
	if _, err := service.GetProject(token.WithSubject(context.Background(), "outsider"), &projectv1.GetProjectRequest{ProjectId: created.Id}); status.Code(err) != codes.NotFound {
		t.Fatalf("outsider read code = %s", status.Code(err))
	}
}

func TestProjectRPCRequiresIdentity(t *testing.T) {
	service := New(newFakeStore(), "project", "test")
	if _, err := service.CreateProject(context.Background(), &projectv1.CreateProjectRequest{Name: "project"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CreateProject() code = %s", status.Code(err))
	}
}

func TestEditorSelectsProjectOutputLanguageAndViewerCannot(t *testing.T) {
	store := newFakeStore()
	service := New(store, "project", "test")
	owner := token.WithSubject(context.Background(), "owner")
	created, err := service.CreateProject(owner, &projectv1.CreateProjectRequest{Name: "project"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = service.SetProjectMemberRole(owner, &projectv1.SetProjectMemberRoleRequest{ProjectId: created.Id, Subject: "editor", Role: projectv1.ProjectRole_PROJECT_ROLE_EDITOR})
	_, _ = service.SetProjectMemberRole(owner, &projectv1.SetProjectMemberRoleRequest{ProjectId: created.Id, Subject: "viewer", Role: projectv1.ProjectRole_PROJECT_ROLE_VIEWER})
	updated, err := service.UpdateOutputLanguage(token.WithSubject(context.Background(), "editor"), &projectv1.UpdateOutputLanguageRequest{ProjectId: created.Id, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_ENGLISH})
	if err != nil || updated.OutputLanguage != commonv1.OutputLanguage_OUTPUT_LANGUAGE_ENGLISH {
		t.Fatalf("language=%v,%v", updated, err)
	}
	_, err = service.UpdateOutputLanguage(token.WithSubject(context.Background(), "viewer"), &projectv1.UpdateOutputLanguageRequest{ProjectId: created.Id, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("viewer language code=%s", status.Code(err))
	}
}
