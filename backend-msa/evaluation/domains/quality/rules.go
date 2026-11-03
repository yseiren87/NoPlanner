package quality

import (
	"fmt"
	"strings"
)

func Evaluate(run Run, baseline *Run) (Run, error) {
	if strings.TrimSpace(run.DatasetVersion) == "" || strings.TrimSpace(run.Model) == "" || strings.TrimSpace(run.PromptVersion) == "" || strings.TrimSpace(run.CriteriaVersion) == "" || len(run.Cases) < 10 {
		return Run{}, ErrInvalid
	}
	var expectedBlocking, detectedBlocking, truePositive, falsePositive, evidenceChecks, correctEvidence, citationChecks, correctCitations, verdictRuns, matchingVerdicts uint64
	categories := map[string]bool{}
	for _, item := range run.Cases {
		if item.CaseID == "" || item.Category == "" || item.DetectedBlocking > item.ExpectedBlocking || item.CorrectEvidence > item.EvidenceChecks || item.CorrectCitations > item.CitationChecks || item.MatchingVerdicts > item.VerdictRuns {
			return Run{}, ErrInvalid
		}
		categories[item.Category] = true
		expectedBlocking += uint64(item.ExpectedBlocking)
		detectedBlocking += uint64(item.DetectedBlocking)
		truePositive += uint64(item.TruePositiveFindings)
		falsePositive += uint64(item.FalsePositiveFindings)
		evidenceChecks += uint64(item.EvidenceChecks)
		correctEvidence += uint64(item.CorrectEvidence)
		citationChecks += uint64(item.CitationChecks)
		correctCitations += uint64(item.CorrectCitations)
		verdictRuns += uint64(item.VerdictRuns)
		matchingVerdicts += uint64(item.MatchingVerdicts)
	}
	for _, required := range []string{"normal", "partial", "not_executable", "before", "after"} {
		if !categories[required] {
			return Run{}, ErrInvalid
		}
	}
	run.Metrics = Metrics{BlockingRecall: ratio(detectedBlocking, expectedBlocking), FindingPrecision: ratio(truePositive, truePositive+falsePositive), EvidenceAccuracy: ratio(correctEvidence, evidenceChecks), CitationAccuracy: ratio(correctCitations, citationChecks), VerdictConsistency: ratio(matchingVerdicts, verdictRuns)}
	checks := []struct {
		name            string
		actual, minimum float64
	}{{"blocking_recall", run.Metrics.BlockingRecall, run.Thresholds.BlockingRecall}, {"finding_precision", run.Metrics.FindingPrecision, run.Thresholds.FindingPrecision}, {"evidence_accuracy", run.Metrics.EvidenceAccuracy, run.Thresholds.EvidenceAccuracy}, {"citation_accuracy", run.Metrics.CitationAccuracy, run.Thresholds.CitationAccuracy}, {"verdict_consistency", run.Metrics.VerdictConsistency, run.Thresholds.VerdictConsistency}}
	run.ReleaseGatePassed = true
	for _, check := range checks {
		if check.minimum <= 0 || check.actual < check.minimum {
			run.ReleaseGatePassed = false
			run.Failures = append(run.Failures, fmt.Sprintf("%s %.4f < %.4f", check.name, check.actual, check.minimum))
		}
	}
	for _, item := range run.Cases {
		for _, exception := range item.Exceptions {
			run.Failures = append(run.Failures, item.CaseID+": "+exception)
		}
	}
	if baseline != nil {
		run.BaselineRunID = baseline.ID
		base := baseline.Metrics
		run.Deltas = []Delta{{"blocking_recall", base.BlockingRecall, run.Metrics.BlockingRecall, run.Metrics.BlockingRecall - base.BlockingRecall}, {"finding_precision", base.FindingPrecision, run.Metrics.FindingPrecision, run.Metrics.FindingPrecision - base.FindingPrecision}, {"evidence_accuracy", base.EvidenceAccuracy, run.Metrics.EvidenceAccuracy, run.Metrics.EvidenceAccuracy - base.EvidenceAccuracy}, {"citation_accuracy", base.CitationAccuracy, run.Metrics.CitationAccuracy, run.Metrics.CitationAccuracy - base.CitationAccuracy}, {"verdict_consistency", base.VerdictConsistency, run.Metrics.VerdictConsistency, run.Metrics.VerdictConsistency - base.VerdictConsistency}}
	}
	return run, nil
}
func DefaultThresholds() Thresholds {
	return Thresholds{BlockingRecall: .90, FindingPrecision: .80, EvidenceAccuracy: .95, CitationAccuracy: .95, VerdictConsistency: .90}
}
func ratio(numerator, denominator uint64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
