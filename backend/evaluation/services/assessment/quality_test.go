package assessment

import (
	"context"
	qualitydomain "noplanner/backend/evaluation/domains/quality"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	"testing"
)

func TestQualityRunBaselineComparison(t *testing.T) {
	store := qualitydomain.NewMemoryStore()
	svc := &Service{quality: store}
	baseline, err := svc.RecordQualityRun(context.Background(), qualityRequest("model-a", "prompt-a", 1))
	if err != nil || !baseline.GetReleaseGatePassed() {
		t.Fatalf("baseline: %#v %v", baseline, err)
	}
	currentRequest := qualityRequest("model-b", "prompt-b", 0)
	currentRequest.BaselineRunId = baseline.GetId()
	current, err := svc.RecordQualityRun(context.Background(), currentRequest)
	if err != nil || current.GetReleaseGatePassed() || len(current.GetDeltas()) != 5 || len(current.GetFailures()) == 0 {
		t.Fatalf("current: %#v %v", current, err)
	}
	loaded, err := svc.GetQualityRun(context.Background(), &evaluationv1.GetQualityRunRequest{QualityRunId: current.GetId()})
	if err != nil || loaded.GetId() != current.GetId() {
		t.Fatalf("loaded: %#v %v", loaded, err)
	}
}
func qualityRequest(model, prompt string, blockingRate float64) *evaluationv1.RecordQualityRunRequest {
	categories := []string{"normal", "partial", "not_executable", "before", "after", "normal", "partial", "not_executable", "before", "after"}
	cases := make([]*evaluationv1.GoldenCaseResult, 10)
	for index, category := range categories {
		detected := uint32(10 * blockingRate)
		cases[index] = &evaluationv1.GoldenCaseResult{CaseId: category + string(rune('a'+index)), Category: category, ExpectedBlocking: 10, DetectedBlocking: detected, TruePositiveFindings: 8, FalsePositiveFindings: 1, EvidenceChecks: 20, CorrectEvidence: 20, CitationChecks: 20, CorrectCitations: 20, VerdictRuns: 10, MatchingVerdicts: 10}
	}
	return &evaluationv1.RecordQualityRunRequest{DatasetVersion: "company-anonymized-v1", Model: model, PromptVersion: prompt, CriteriaVersion: "criteria-v1", Cases: cases, Thresholds: &evaluationv1.QualityThresholds{BlockingRecall: .9, FindingPrecision: .8, EvidenceAccuracy: .95, CitationAccuracy: .95, VerdictConsistency: .9}}
}
