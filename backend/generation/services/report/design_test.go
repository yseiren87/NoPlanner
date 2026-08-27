package report

import (
	"context"
	design "noplanner/backend/generation/domains/design"
	"noplanner/backend/generation/modules/uivalidator"
	"os"
	"testing"

	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
)

type passingUIValidator struct{}

func (passingUIValidator) Validate(context.Context, uivalidator.Product) []uivalidator.Validation {
	return []uivalidator.Validation{{ID: "execution-build", Kind: "build", Status: "passed"}, {ID: "execution-render", Kind: "render", Status: "passed"}, {ID: "execution-interaction", Kind: "interaction", Status: "passed"}, {ID: "execution-accessibility", Kind: "accessibility", Status: "passed"}, {ID: "execution-responsive", Kind: "responsive", Status: "passed"}}
}

func TestProductDesignE2E(t *testing.T) {
	svc := New(nil, "generation", "test")
	svc.designStore = design.NewMemoryStore()
	svc.SetUIValidator(passingUIValidator{})
	result, err := svc.GenerateProductDesign(context.Background(), &generationv1.GenerateProductDesignRequest{PlanId: "P-1", BaseRevision: "abc123", Requirements: []*generationv1.DesignRequirement{{Id: "REQ-1", Statement: "리포트를 확인한다", AcceptanceCriteria: "판정과 근거가 표시된다"}}, UserFlows: []string{"리포트를 연다"}, ValidationResults: []*generationv1.ValidationCheck{{Id: "build", Kind: "build", Status: "passed", CommandOrViewport: "npm run build:development"}, {Id: "render", Kind: "render", Status: "passed"}, {Id: "interaction", Kind: "interaction", Status: "passed"}, {Id: "a11y", Kind: "accessibility", Status: "passed"}, {Id: "responsive", Kind: "responsive", Status: "passed", CommandOrViewport: "320,768,1280"}}})
	if err != nil || !result.GetExecutable() || len(result.GetScreens()) != 1 || len(result.GetScreens()[0].GetStates()) != 5 || len(result.GetTraceability()) != 1 || result.GetChangeSet().GetBranchName() == "" {
		t.Fatalf("product design not executable: %#v %v", result, err)
	}
	loaded, err := svc.GetProductDesign(context.Background(), &generationv1.GetProductDesignRequest{DesignId: result.GetId()})
	if err != nil || loaded.GetId() != result.GetId() {
		t.Fatalf("stored design unavailable: %#v %v", loaded, err)
	}
}

func TestProductDesignCannotBeExecutableWithoutUIValidator(t *testing.T) {
	svc := New(nil, "generation", "test")
	svc.designStore = design.NewMemoryStore()
	result, err := svc.GenerateProductDesign(context.Background(), &generationv1.GenerateProductDesignRequest{PlanId: "P-1", BaseRevision: "abc123", Requirements: []*generationv1.DesignRequirement{{Id: "REQ-1", Statement: "리포트를 확인한다", AcceptanceCriteria: "판정과 근거가 표시된다"}}, UserFlows: []string{"리포트를 연다"}})
	if err != nil || result.GetExecutable() || len(result.GetBlockers()) != 1 || result.GetBlockers()[0] != "execution-validator-unavailable" {
		t.Fatalf("missing validator must block design: %#v %v", result, err)
	}
}

func TestRealGeneratedProductDesignIsExecutable(t *testing.T) {
	if os.Getenv("NOPLANNER_UI_VALIDATOR_INTEGRATION") != "1" {
		t.Skip("set NOPLANNER_UI_VALIDATOR_INTEGRATION=1 to run npm and Chromium")
	}
	svc := New(nil, "generation", "test")
	svc.designStore = design.NewMemoryStore()
	svc.SetUIValidator(uivalidator.New())
	result, err := svc.GenerateProductDesign(context.Background(), &generationv1.GenerateProductDesignRequest{PlanId: "P-1", BaseRevision: "abc123", Requirements: []*generationv1.DesignRequirement{{Id: "REQ-1", Statement: "평가 결과를 확인한다", AcceptanceCriteria: "판정과 근거가 표시된다"}}, UserFlows: []string{"리포트를 연다"}})
	if err != nil || !result.GetExecutable() || len(result.GetValidations()) != 5 {
		for _, validation := range result.GetValidations() {
			t.Logf("%s: %s: %s", validation.GetKind(), validation.GetStatus(), validation.GetDetail())
		}
		t.Fatalf("real generated UI was not executable: %v", err)
	}
	for _, validation := range result.GetValidations() {
		if validation.GetStatus() != "passed" {
			t.Fatalf("validation failed: %#v", validation)
		}
	}
}
