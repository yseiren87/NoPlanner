package report

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

var ErrInvalid = errors.New("invalid report")
var ErrNotFound = errors.New("report not found")

func NewID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "id-unavailable"
	}
	return hex.EncodeToString(raw[:])
}

func Assemble(input Report, inputs []FindingInput) (Report, error) {
	if strings.TrimSpace(input.EvaluationID) == "" || strings.TrimSpace(input.ProjectName) == "" || strings.TrimSpace(input.DocumentName) == "" || input.DocumentVersion == 0 {
		return Report{}, ErrInvalid
	}
	input.ID = NewID()
	criticalUnverified, blocking, high, broadRewrite := false, false, false, 0
	for _, raw := range inputs {
		// Internal findings use the immutable document location as provenance;
		// external findings use evidence links.
		if raw.ID == "" || raw.Area == "" || raw.Finding == "" || raw.Impact == "" || raw.RequiredAction == "" || (len(raw.Evidence) == 0 && strings.TrimSpace(raw.SourceLocation) == "" && raw.Status != "unverified") {
			return Report{}, ErrInvalid
		}
		severity := severity(raw)
		confidence := confidence(raw)
		input.Findings = append(input.Findings, Finding{Detail: raw, Severity: severity, Confidence: confidence})
		blocking = blocking || severity == "blocking"
		high = high || severity == "high"
		if severity == "high" && (raw.Area == "purpose" || raw.Area == "completeness" || raw.Area == "consistency") {
			broadRewrite++
		}
		criticalUnverified = criticalUnverified || (raw.CriticalAssumption && raw.Status == "unverified")
		if severity == "blocking" || severity == "high" {
			input.RequiredActions = appendUnique(input.RequiredActions, raw.RequiredAction)
		} else {
			input.Recommendations = appendUnique(input.Recommendations, raw.RequiredAction)
		}
	}
	for _, assumption := range input.Assumptions {
		criticalUnverified = criticalUnverified || assumption.Status == "unverified"
	}
	input.Verdict = "executable"
	if blocking {
		input.Verdict = "not_executable"
	} else if broadRewrite >= 2 {
		input.Verdict = "rewrite_required"
	} else if high || criticalUnverified {
		input.Verdict = "conditionally_executable"
	}
	input.ImplementationMayStart = input.Verdict == "executable"
	input.Confidence = overallConfidence(input.Findings, criticalUnverified)
	input.Conclusion = conclusion(input.Verdict)
	input.VerdictChangeConditions = append([]string(nil), input.RequiredActions...)
	if criticalUnverified {
		input.VerdictChangeConditions = appendUnique(input.VerdictChangeConditions, "핵심 미검증 전제를 신뢰할 수 있는 원출처로 확인")
	}
	sort.SliceStable(input.Findings, func(i, j int) bool {
		return severityRank(input.Findings[i].Severity) > severityRank(input.Findings[j].Severity)
	})
	return input, nil
}

func severity(f FindingInput) string {
	risk := (f.Likelihood * .3) + (f.ImpactScore * .4) + (f.RecoveryCost * .2) + ((1 - f.Reversibility) * .1)
	if f.ConfirmedFalse && f.CriticalAssumption {
		return "blocking"
	}
	if risk >= .75 {
		return "blocking"
	}
	if risk >= .55 {
		return "high"
	}
	if risk >= .3 {
		return "medium"
	}
	return "low"
}
func confidence(f FindingInput) string {
	direct := 0
	conflict := false
	for _, e := range f.Evidence {
		if e.Grade == "A" || e.Grade == "B" {
			direct++
		}
		conflict = conflict || e.Relation == "contradicts"
	}
	if direct > 0 && !conflict {
		return "high"
	}
	if len(f.Evidence) >= 2 {
		return "medium"
	}
	return "low"
}
func overallConfidence(items []Finding, unverified bool) string {
	if unverified {
		return "low"
	}
	low := 0
	for _, item := range items {
		if item.Confidence == "low" {
			low++
		}
	}
	if low == 0 {
		return "high"
	}
	if low*2 <= len(items) {
		return "medium"
	}
	return "low"
}
func conclusion(verdict string) string {
	switch verdict {
	case "not_executable":
		return "핵심 전제가 성립하지 않거나 구현을 차단하는 문제가 있어 실행할 수 없습니다."
	case "rewrite_required":
		return "목적·전제·구조 또는 핵심 명세가 광범위하게 부족하여 재작성이 필요합니다."
	case "conditionally_executable":
		return "명시된 선행 조건과 미검증 전제를 해결한 후 실행할 수 있습니다."
	default:
		return "차단 또는 높은 문제가 없고 핵심 전제가 검증되어 구현을 시작할 수 있습니다."
	}
}
func severityRank(value string) int {
	switch value {
	case "blocking":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}
func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}
