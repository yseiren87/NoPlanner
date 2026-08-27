package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnlyMissingConnectorIsDisabledAndWritesNeedApproval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("token missing")
		}
		w.Header().Set("Location", "issue-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	registry := New([]Config{{Name: "github", Endpoint: server.URL, Token: "secret", Capabilities: []Capability{WriteWorkItem}}, {Name: "drive", Capabilities: []Capability{ReadDocuments}}})
	statuses := registry.List()
	if statuses[0].Name != "drive" || statuses[0].Enabled {
		t.Fatalf("drive must be disabled: %#v", statuses[0])
	}
	if _, err := registry.Deliver(context.Background(), "github", "repo", "payload", ""); !errorsIs(err, ErrApprovalRequired) {
		t.Fatalf("approval required: %v", err)
	}
	result, err := registry.Deliver(context.Background(), "github", "repo", "payload", "approval-1")
	if err != nil || result.ExternalReference != "issue-1" {
		t.Fatalf("delivery: %#v %v", result, err)
	}
}
func errorsIs(value, target error) bool { return value == target }
