package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	implementation "noplanner/backend/generation/domains/implementation"
	"noplanner/backend/generation/modules/connectors"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

type proposalIntelligenceStub struct {
	intelligencev1.IntelligenceServiceClient
}

func (proposalIntelligenceStub) Generate(context.Context, *intelligencev1.GenerateRequest, ...grpc.CallOption) (*intelligencev1.GenerateResponse, error) {
	return &intelligencev1.GenerateResponse{Text: `{"patches":[{"path":"src/app.test.tsx","unified_diff":"--- /dev/null\n+++ b/src/app.test.tsx\n@@ -0,0 +1 @@\n+test('works',()=>{})","mismatch_ids":["mismatch-test-REQ-1"],"requirement_ids":["REQ-1"]}]}`}, nil
}

func TestImplementationProposalIsGeneratedWhenCallerProvidesNoPatch(t *testing.T) {
	root, revision := testRepository(t)
	svc := NewWithImplementation(nil, nil, nil, nil, nil, proposalIntelligenceStub{}, nil, []string{root}, "generation", "test")
	svc.intelligence = proposalIntelligenceStub{}
	review := &generationv1.ImplementationReview{Id: "review-1", ProjectId: "P-1", EvaluationId: "EV-1", RepositoryRevision: revision, RepositoryRoot: root, Mismatches: []*generationv1.ImplementationMismatch{{Id: "mismatch-test-REQ-1", RequirementId: "REQ-1", Kind: "missing_acceptance_test", RequiredChange: "테스트 추가"}}}
	proposal, err := svc.CreateImplementationProposal(context.Background(), &generationv1.CreateImplementationProposalRequest{Review: review, BaseRevision: revision, ActorId: "user-1"})
	if err != nil || len(proposal.GetPatches()) != 1 || proposal.GetWorkingTreeModified() {
		t.Fatalf("generated proposal = %#v, %v", proposal, err)
	}
}

func TestImplementationClosedLoopAndAudit(t *testing.T) {
	root, revision := testRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "issue-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	store := implementation.NewMemoryStore()
	registry := connectors.New([]connectors.Config{{Name: "github", Endpoint: server.URL, Token: "super-secret-token", Capabilities: []connectors.Capability{connectors.WriteWorkItem}}})
	svc := NewWithImplementation(nil, store, nil, nil, registry, nil, nil, []string{root}, "generation", "test")
	ctx := context.Background()
	review, err := svc.CompareImplementation(ctx, &generationv1.CompareImplementationRequest{EvaluationId: "EV-1", Repository: &generationv1.RepositoryAnalysis{Id: "R-1", ProjectId: "P-1", RepositoryRoot: root, Revision: revision, ReadOnly: true, Files: []*generationv1.RepositoryFile{{Path: "main.go"}}}, Expectations: []*generationv1.ImplementationExpectation{{RequirementId: "REQ-1", Statement: "로그인", AcceptanceCriteria: "테스트 통과", ExpectedPaths: []string{"main.go"}, ExpectedTestPaths: []string{"main_test.go"}}}})
	if err != nil || len(review.GetMismatches()) != 1 {
		t.Fatalf("review: %#v %v", review, err)
	}
	proposal, err := svc.CreateImplementationProposal(ctx, &generationv1.CreateImplementationProposalRequest{Review: review, BaseRevision: revision, ActorId: "user-1", Patches: []*generationv1.ProposedFilePatch{{Path: "main_test.go", UnifiedDiff: "--- /dev/null\n+++ b/main_test.go\n@@ -0,0 +1,3 @@\n+package main\n+\n+// acceptance test\n", MismatchIds: []string{review.GetMismatches()[0].GetId()}, RequirementIds: []string{"REQ-1"}}}})
	if err != nil || proposal.GetWorkingTreeModified() {
		t.Fatalf("proposal: %#v %v", proposal, err)
	}
	pending, err := svc.DeliverIntegration(ctx, &generationv1.DeliverIntegrationRequest{ActorId: "user-1", Connector: "github", Target: "owner/repo", Payload: "company secret body", ProjectId: "P-1", TraceId: proposal.GetId()})
	if err != nil || !pending.GetApprovalRequired() {
		t.Fatalf("approval: %#v %v", pending, err)
	}
	delivered, err := svc.DeliverIntegration(ctx, &generationv1.DeliverIntegrationRequest{ActorId: "user-1", Connector: "github", Target: "owner/repo", Payload: "company secret body", ProjectId: "P-1", TraceId: proposal.GetId(), ApprovalId: "approval-1"})
	if err != nil || delivered.GetStatus() != "delivered" {
		t.Fatalf("delivery: %#v %v", delivered, err)
	}
	duplicate, err := svc.DeliverIntegration(ctx, &generationv1.DeliverIntegrationRequest{ActorId: "user-1", Connector: "github", Target: "owner/repo", Payload: "company secret body", ProjectId: "P-1", TraceId: proposal.GetId(), ApprovalId: "approval-1"})
	if err != nil || duplicate.GetStatus() != "already_delivered" || duplicate.GetId() != delivered.GetId() {
		t.Fatalf("duplicate delivery: %#v %v", duplicate, err)
	}
	history, err := svc.GetImplementationHistory(ctx, &generationv1.GetImplementationHistoryRequest{ProjectId: "P-1"})
	if err != nil || len(history.GetItems()) != 4 {
		t.Fatalf("history: %#v %v", history, err)
	}
	for _, item := range history.GetItems() {
		if item.GetPayloadHash() == "company secret body" || item.GetTarget() == "super-secret-token" {
			t.Fatal("audit exposed secret")
		}
	}
}

func testRepository(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "test"}, {"add", "main.go"}, {"commit", "-m", "initial"}} {
		command := exec.Command("git", args...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
		}
	}
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = root
	value, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return root, strings.TrimSpace(string(value))
}
