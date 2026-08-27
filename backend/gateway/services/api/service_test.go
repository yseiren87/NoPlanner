package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"noplanner/backend/gateway/modules/clients"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	projectv1 "noplanner/backend/proto/dist/golang/project/v1"
)

type projectClientStub struct {
	projectv1.ProjectServiceClient
	allowed bool
}

func (stub projectClientStub) GetProject(_ context.Context, request *projectv1.GetProjectRequest, _ ...grpc.CallOption) (*projectv1.Project, error) {
	if !stub.allowed {
		return nil, status.Error(codes.NotFound, "not found")
	}
	return &projectv1.Project{Id: request.GetProjectId(), CallerRole: projectv1.ProjectRole_PROJECT_ROLE_VIEWER}, nil
}

func TestCORSAllowsLocalFrontendAndIdempotencyHeader(t *testing.T) {
	handler := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodOptions, "/api/evaluation-jobs", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("allowed origin = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type, Idempotency-Key" {
		t.Fatalf("allowed headers = %q", got)
	}
}

func TestAuthorizeReportUsesProjectMembership(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/reports/report-1", nil)
	request = request.WithContext(context.WithValue(request.Context(), subjectKey{}, "viewer"))
	report := &generationv1.EvaluationReport{Id: "report-1", ProjectId: "project-1", OwnerSubject: "owner"}

	allowed := New(&clients.Clients{Project: projectClientStub{allowed: true}}, nil, nil)
	if !allowed.authorizeReport(httptest.NewRecorder(), request, report) {
		t.Fatal("project member was denied")
	}

	denied := New(&clients.Clients{Project: projectClientStub{allowed: false}}, nil, nil)
	response := httptest.NewRecorder()
	if denied.authorizeReport(response, request, report) {
		t.Fatal("non-member was allowed")
	}
	if response.Code != http.StatusNotFound {
		t.Fatalf("denied status = %d", response.Code)
	}
}

func TestAuthorizeLegacyReportRequiresExactOwner(t *testing.T) {
	service := New(&clients.Clients{}, nil, nil)
	report := &generationv1.EvaluationReport{Id: "legacy", OwnerSubject: "owner"}
	ownerRequest := httptest.NewRequest(http.MethodGet, "/api/reports/legacy", nil)
	ownerRequest = ownerRequest.WithContext(context.WithValue(ownerRequest.Context(), subjectKey{}, "owner"))
	if !service.authorizeReport(httptest.NewRecorder(), ownerRequest, report) {
		t.Fatal("legacy owner was denied")
	}

	otherRequest := httptest.NewRequest(http.MethodGet, "/api/reports/legacy", nil)
	otherRequest = otherRequest.WithContext(context.WithValue(otherRequest.Context(), subjectKey{}, "other"))
	if service.authorizeReport(httptest.NewRecorder(), otherRequest, report) {
		t.Fatal("non-owner was allowed to read legacy report")
	}
}
