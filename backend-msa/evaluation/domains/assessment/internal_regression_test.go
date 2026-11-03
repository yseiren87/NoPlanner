package assessment

import "testing"

func TestInternalEvaluationGoldenRegressions(t *testing.T) {
	tests := []struct {
		name       string
		findings   []Finding
		unverified bool
		verdict    string
	}{
		{"purpose missing", []Finding{{Type: "missing", Severity: "high"}}, false, "rewrite_required"},
		{"contradiction", []Finding{{Type: "contradiction", Severity: "blocking"}}, false, "not_executable"},
		{"requirement gap", []Finding{{Type: "incomplete_requirement", Severity: "high"}}, false, "rewrite_required"},
		{"critical assumption unverified", nil, true, "conditionally_executable"},
		{"clean", nil, false, "executable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := InternalVerdict(test.findings, test.unverified); got != test.verdict {
				t.Fatalf("verdict = %s, want %s", got, test.verdict)
			}
		})
	}
}

func TestBlockingFindingCannotBeOffsetByManySatisfiedResults(t *testing.T) {
	findings := []Finding{{Type: "contradiction", Severity: "blocking"}}
	for index := 0; index < 100; index++ {
		findings = append(findings, Finding{Type: "quality", Severity: "low"})
	}
	if verdict := InternalVerdict(findings, false); verdict != "not_executable" {
		t.Fatalf("blocking finding was offset: %s", verdict)
	}
}
