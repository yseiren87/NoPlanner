package design

import "testing"

func TestGenerateProductDesignTraceAndStates(t *testing.T) {
	result, err := Generate("P-1", "abc123", []Requirement{{ID: "REQ-1", Statement: "평가 확인", AcceptanceCriteria: "결과 표시"}, {ID: "REQ-2", Statement: "수정 제출", AcceptanceCriteria: "재검증 시작"}}, []string{"결과를 연다", "수정을 제출한다"}, passedValidations())
	if err != nil || !result.Executable || len(result.Traceability) != 2 || len(result.Screens[0].States) != 5 || len(result.ChangeSet.Files) != 2 {
		t.Fatalf("invalid design: %#v %v", result, err)
	}
}
func TestFailedValidationBlocksDesign(t *testing.T) {
	result, err := Generate("P-1", "abc123", []Requirement{{ID: "REQ-1", Statement: "평가 확인", AcceptanceCriteria: "결과 표시"}}, []string{"결과를 연다"}, []Validation{{ID: "render", Kind: "render", Status: "failed", Detail: "렌더링 실패"}})
	if err != nil || result.Executable || len(result.Findings) == 0 || result.Findings[0].Status != "open" {
		t.Fatalf("failure must be tracked: %#v %v", result, err)
	}
}
func TestServerAutomaticallyRunsRequiredDesignValidations(t *testing.T) {
	result, err := Generate("P-1", "abc123", []Requirement{{ID: "REQ-1", Statement: "평가 확인", AcceptanceCriteria: "결과 표시"}}, []string{"결과를 연다"}, nil)
	if err != nil || !result.Executable || len(result.Validations) != len(requiredValidations) {
		t.Fatalf("automatic validations = %#v, %v", result.Validations, err)
	}
	for _, validation := range result.Validations {
		if validation.Status != "passed" || validation.CommandOrViewport == "" {
			t.Fatalf("validation = %#v", validation)
		}
	}
}
func passedValidations() []Validation {
	return []Validation{{ID: "build", Kind: "build", Status: "passed"}, {ID: "render", Kind: "render", Status: "passed"}, {ID: "interaction", Kind: "interaction", Status: "passed"}, {ID: "a11y", Kind: "accessibility", Status: "passed"}, {ID: "responsive", Kind: "responsive", Status: "passed"}}
}
