package report

import "testing"

func finding(id string, impact float64) FindingInput {
	return FindingInput{ID: id, Area: "data", ProblemType: "unavailable", Finding: "필수 데이터 확보 불가", Impact: "핵심 기능 중단", Likelihood: 1, ImpactScore: impact, RecoveryCost: 1, Reversibility: 0, RequiredAction: "대체 데이터 확보", VerificationMethod: "샘플 데이터 확인", Status: "not_satisfied", CriticalAssumption: true, ConfirmedFalse: true, Evidence: []Evidence{{ID: "E-1", URL: "https://data.go.kr", SourceLocation: "필드 목록", Grade: "A"}}}
}
func TestBlockingCannotBeOffset(t *testing.T) {
	inputs := []FindingInput{finding("DAT-1", 1)}
	for i := 0; i < 100; i++ {
		f := finding("LOW", .1)
		f.CriticalAssumption = false
		f.ConfirmedFalse = false
		f.Likelihood = .1
		f.RecoveryCost = 0
		inputs = append(inputs, f)
	}
	r, err := Assemble(Report{EvaluationID: "e", ProjectName: "p", DocumentName: "d", DocumentVersion: 1}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != "not_executable" || r.ImplementationMayStart {
		t.Fatalf("report=%#v", r)
	}
}
func TestCriticalUnverifiedPreventsExecutable(t *testing.T) {
	r, err := Assemble(Report{EvaluationID: "e", ProjectName: "p", DocumentName: "d", DocumentVersion: 1, Assumptions: []Assumption{{ID: "a", Statement: "API exists", Status: "unverified"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != "conditionally_executable" {
		t.Fatalf("verdict=%s", r.Verdict)
	}
}
func TestSeverityIgnoresWritingLength(t *testing.T) {
	a, b := finding("a", .6), finding("b", .6)
	b.Statement = "매우 " + string(make([]byte, 1000))
	ra, _ := Assemble(Report{EvaluationID: "e", ProjectName: "p", DocumentName: "d", DocumentVersion: 1}, []FindingInput{a})
	rb, _ := Assemble(Report{EvaluationID: "e", ProjectName: "p", DocumentName: "d", DocumentVersion: 1}, []FindingInput{b})
	if ra.Findings[0].Severity != rb.Findings[0].Severity {
		t.Fatal("writing length affected severity")
	}
}

func TestInternalFindingMayUseDocumentLocationAsProvenance(t *testing.T) {
	input := Report{EvaluationID: "e", ProjectName: "p", DocumentName: "d", DocumentVersion: 1}
	f := FindingInput{ID: "f", Area: "purpose", Finding: "목적 연결 누락", Impact: "우선순위 불명확", RequiredAction: "목적 연결을 명시", SourceLocation: "page:1 paragraph:2", Status: "open"}
	if _, err := Assemble(input, []FindingInput{f}); err != nil {
		t.Fatalf("document location should be accepted as provenance: %v", err)
	}
}
