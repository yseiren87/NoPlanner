package assessment

import (
	"errors"
	"strings"
)

func Summarize(validationPlanID string, results []ValidationResult) (ValidationResultSummary, error) {
	if strings.TrimSpace(validationPlanID) == "" || len(results) == 0 {
		return ValidationResultSummary{}, ErrInvalid
	}
	summary := ValidationResultSummary{ValidationPlanID: validationPlanID, Results: results}
	var points float64
	seen := map[string]struct{}{}
	for index := range summary.Results {
		result := &summary.Results[index]
		if result.Area == "" || strings.TrimSpace(result.Reason) == "" {
			return ValidationResultSummary{}, ErrInvalid
		}
		if _, exists := seen[result.Area]; exists {
			return ValidationResultSummary{}, errors.New("duplicate validation area")
		}
		seen[result.Area] = struct{}{}
		switch result.Status {
		case "satisfied":
			result.Scored = true
			points++
		case "partially_satisfied":
			result.Scored, result.CountedAsDefect = true, true
			points += .5
		case "not_satisfied":
			result.Scored, result.CountedAsDefect = true, true
		case "unverified":
			result.Scored = true
		case "not_applicable", "skipped":
			summary.ExcludedCount++
		default:
			return ValidationResultSummary{}, ErrInvalid
		}
		if result.Scored {
			summary.ScoredCount++
		}
		if result.CountedAsDefect {
			summary.DefectCount++
		}
	}
	if summary.ScoredCount > 0 {
		summary.SatisfactionRate = points / float64(summary.ScoredCount)
	}
	return summary, nil
}

func InternalVerdict(findings []Finding, criticalAssumptionUnverified bool) string {
	high := false
	for _, finding := range findings {
		switch finding.Severity {
		case "blocking":
			return "not_executable"
		case "high":
			high = true
		}
	}
	if high {
		return "rewrite_required"
	}
	if criticalAssumptionUnverified {
		return "conditionally_executable"
	}
	return "executable"
}
