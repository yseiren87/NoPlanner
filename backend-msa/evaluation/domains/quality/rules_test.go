package quality

import "testing"

func TestQualityGateAndBaselineDelta(t *testing.T) {
	cases := tenCases()
	baseline, _ := Evaluate(Run{ID: "baseline", DatasetVersion: "v1", Model: "m1", PromptVersion: "p1", CriteriaVersion: "c1", Cases: cases, Thresholds: DefaultThresholds()}, nil)
	cases[0].DetectedBlocking = 0
	cases[1].DetectedBlocking = 0
	current, err := Evaluate(Run{DatasetVersion: "v1", Model: "m2", PromptVersion: "p2", CriteriaVersion: "c1", Cases: cases, Thresholds: DefaultThresholds()}, &baseline)
	if err != nil || current.ReleaseGatePassed || len(current.Failures) == 0 || len(current.Deltas) != 5 {
		t.Fatalf("quality gate: %#v %v", current, err)
	}
}
func tenCases() []CaseResult {
	categories := []string{"normal", "partial", "not_executable", "before", "after", "normal", "partial", "not_executable", "before", "after"}
	out := make([]CaseResult, 10)
	for i, category := range categories {
		out[i] = CaseResult{CaseID: category + string(rune('a'+i)), Category: category, ExpectedBlocking: 1, DetectedBlocking: 1, TruePositiveFindings: 4, FalsePositiveFindings: 0, EvidenceChecks: 4, CorrectEvidence: 4, CitationChecks: 4, CorrectCitations: 4, VerdictRuns: 3, MatchingVerdicts: 3}
	}
	return out
}
