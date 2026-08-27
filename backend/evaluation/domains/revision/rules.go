package revision

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var ErrInvalid = errors.New("invalid revision")

func newID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "id-unavailable"
	}
	return hex.EncodeToString(raw[:])
}
func Compare(documentID string, previousVersion, currentVersion uint32, previous, current []Item, affectedFindingIDs map[string][]string) (Comparison, error) {
	if strings.TrimSpace(documentID) == "" || previousVersion == 0 || currentVersion <= previousVersion {
		return Comparison{}, ErrInvalid
	}
	result := Comparison{ID: newID(), DocumentID: documentID, PreviousVersion: previousVersion, CurrentVersion: currentVersion}
	old := map[string]Item{}
	used := map[string]bool{}
	for _, item := range previous {
		if item.ID == "" || item.Kind == "" {
			return Comparison{}, ErrInvalid
		}
		old[item.ID] = item
	}
	for _, item := range current {
		if item.ID == "" || item.Kind == "" {
			return Comparison{}, ErrInvalid
		}
		prior, ok := old[item.ID]
		if !ok {
			prior, ok = closest(item, previous, used)
		}
		change := Change{StableID: item.ID, ItemKind: item.Kind, Current: &item}
		if ok {
			used[prior.ID] = true
			change.Previous = &prior
			change.StableID = prior.ID
			change.Kind = classify(prior.Content, item.Content)
		} else {
			change.Kind = "added"
		}
		enrich(&change, affectedFindingIDs)
		result.Changes = append(result.Changes, change)
	}
	for _, item := range previous {
		if !used[item.ID] && !containsCurrentID(current, item.ID) {
			change := Change{StableID: item.ID, ItemKind: item.Kind, Kind: "removed", Previous: &item}
			enrich(&change, affectedFindingIDs)
			result.Changes = append(result.Changes, change)
		}
	}
	for _, change := range result.Changes {
		if change.Kind != "unchanged" && change.Kind != "expression_only" {
			result.AffectedAreas = unique(result.AffectedAreas, change.AffectedAreas...)
			result.AffectedFindingIDs = unique(result.AffectedFindingIDs, change.AffectedFindingIDs...)
			if change.RequiresResearch {
				result.ResearchFirstIDs = unique(result.ResearchFirstIDs, change.StableID)
			}
		}
	}
	return result, nil
}
func classify(a, b string) string {
	if a == b {
		return "unchanged"
	}
	if canonical(a) == canonical(b) {
		return "expression_only"
	}
	if similarity(a, b) >= .82 || characterSimilarity(canonical(a), canonical(b)) >= .9 {
		return "expression_only"
	}
	return "meaning_changed"
}

func characterSimilarity(a, b string) float64 {
	left, right := []rune(a), []rune(b)
	if len(left) == 0 && len(right) == 0 {
		return 1
	}
	previous := make([]int, len(right)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, l := range left {
		current := make([]int, len(right)+1)
		current[0] = i + 1
		for j, r := range right {
			cost := 0
			if l != r {
				cost = 1
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	longest := len(left)
	if len(right) > longest {
		longest = len(right)
	}
	return 1 - float64(previous[len(right)])/float64(longest)
}

func min(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
func canonical(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
}

var tokenPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)

func similarity(a, b string) float64 {
	left, right := map[string]bool{}, map[string]bool{}
	for _, v := range tokenPattern.FindAllString(strings.ToLower(a), -1) {
		left[v] = true
	}
	for _, v := range tokenPattern.FindAllString(strings.ToLower(b), -1) {
		right[v] = true
	}
	intersection, union := 0, len(left)
	for v := range right {
		if left[v] {
			intersection++
		} else {
			union++
		}
	}
	if union == 0 {
		return 1
	}
	return float64(intersection) / float64(union)
}
func closest(item Item, items []Item, used map[string]bool) (Item, bool) {
	best := 0.0
	var found Item
	for _, candidate := range items {
		if used[candidate.ID] || candidate.Kind != item.Kind {
			continue
		}
		score := similarity(candidate.Content, item.Content)
		if score > best {
			best, found = score, candidate
		}
	}
	return found, best >= .45
}
func containsCurrentID(items []Item, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
func enrich(change *Change, links map[string][]string) {
	change.AffectedFindingIDs = append(change.AffectedFindingIDs, links[change.StableID]...)
	switch change.ItemKind {
	case "purpose", "goal", "problem":
		change.AffectedAreas = []string{"purpose", "problem", "context"}
	case "assumption", "claim", "evidence":
		change.AffectedAreas = []string{"evidence", "context"}
		change.RequiresResearch = true
	case "requirement", "policy", "state", "exception":
		change.AffectedAreas = []string{"completeness", "consistency", "operations"}
	case "data":
		change.AffectedAreas = []string{"data", "evidence"}
		change.RequiresResearch = true
	case "api":
		change.AffectedAreas = []string{"api", "technology"}
		change.RequiresResearch = true
	default:
		change.AffectedAreas = []string{"consistency"}
	}
	change.Reason = "내용 변경으로 연결된 평가 영역과 기존 문제만 재검증"
	if change.Kind == "expression_only" || change.Kind == "unchanged" {
		change.AffectedAreas = nil
		change.AffectedFindingIDs = nil
		change.RequiresResearch = false
		change.Reason = "의미 변화가 없어 기존 판정을 보존"
	}
}
func Reevaluate(documentID, previousEvaluationID, currentEvaluationID string, comparison Comparison, previous, current []Finding, previousVerdict, criteriaVersion, model, objectionID, objectionFindingID string, newEvidenceIDs []string) (Reevaluation, error) {
	if documentID == "" || previousEvaluationID == "" || currentEvaluationID == "" || criteriaVersion == "" || model == "" {
		return Reevaluation{}, ErrInvalid
	}
	currentVerdict := deriveVerdict(current)
	result := Reevaluation{ID: newID(), DocumentID: documentID, PreviousEvaluationID: previousEvaluationID, CurrentEvaluationID: currentEvaluationID, RerunAreas: append([]string(nil), comparison.AffectedAreas...), RerunResearchIDs: append([]string(nil), comparison.ResearchFirstIDs...), PreviousVerdict: previousVerdict, CurrentVerdict: currentVerdict, VerdictChanged: previousVerdict != currentVerdict, CriteriaVersion: criteriaVersion, Model: model, ObjectionID: objectionID, NewEvidenceIDs: newEvidenceIDs}
	old, newer := map[string]Finding{}, map[string]Finding{}
	for _, f := range previous {
		old[f.ID] = f
	}
	for _, f := range current {
		newer[f.ID] = f
	}
	for id, prior := range old {
		now, exists := newer[id]
		change := FindingChange{FindingID: id, Previous: &prior, PreservedEvidenceIDs: append([]string(nil), prior.EvidenceIDs...)}
		if !exists {
			if objectionID != "" && objectionFindingID == id && len(newEvidenceIDs) > 0 {
				change.Status = "withdrawn_by_evidence"
				change.Reason = "새 근거로 기존 판정이 성립하지 않아 철회"
			} else {
				change.Status = "resolved"
				change.Reason = "수정본에서 기존 문제 조건이 제거됨"
			}
		} else {
			change.Current = &now
			change.PreservedEvidenceIDs = unique(change.PreservedEvidenceIDs, now.EvidenceIDs...)
			change.Status = "unresolved"
			change.Reason = "동일 문제와 심각도가 유지됨"
			if severityRank(now.Severity) > severityRank(prior.Severity) {
				change.Status = "worsened"
				change.Reason = "수정 후 영향 또는 심각도가 증가함"
			} else if severityRank(now.Severity) < severityRank(prior.Severity) || now.Status == "partially_resolved" {
				change.Status = "partially_resolved"
				change.Reason = "일부 조건은 개선됐으나 문제가 남아 있음"
			}
		}
		result.FindingChanges = append(result.FindingChanges, change)
	}
	for id, now := range newer {
		if _, exists := old[id]; !exists {
			copy := now
			result.FindingChanges = append(result.FindingChanges, FindingChange{FindingID: id, Status: "new", Current: &copy, Reason: "수정본 재평가에서 새로 발견됨", PreservedEvidenceIDs: append([]string(nil), now.EvidenceIDs...)})
		}
	}
	allAreas := []string{"purpose", "problem", "context", "evidence", "data", "api", "technology", "resource", "consistency", "completeness", "verifiability", "operations"}
	for _, area := range allAreas {
		if !contains(result.RerunAreas, area) {
			result.PreservedAreas = append(result.PreservedAreas, area)
		}
	}
	if result.VerdictChanged {
		result.VerdictChangeReasons = verdictReasons(result.FindingChanges, criteriaVersion, model)
	}
	sort.Slice(result.FindingChanges, func(i, j int) bool { return result.FindingChanges[i].FindingID < result.FindingChanges[j].FindingID })
	return result, nil
}

func deriveVerdict(findings []Finding) string {
	high, broad := false, 0
	for _, finding := range findings {
		if finding.Status == "withdrawn" || finding.Status == "resolved" {
			continue
		}
		if finding.Severity == "blocking" {
			return "not_executable"
		}
		if finding.Severity == "high" {
			high = true
			if finding.Area == "purpose" || finding.Area == "consistency" || finding.Area == "completeness" {
				broad++
			}
		}
	}
	if broad >= 2 {
		return "rewrite_required"
	}
	if high {
		return "conditionally_executable"
	}
	return "executable"
}
func NewObjection(evaluationID, findingID, explanation string, evidence []Evidence) (Objection, error) {
	if evaluationID == "" || findingID == "" || strings.TrimSpace(explanation) == "" || len(evidence) == 0 {
		return Objection{}, ErrInvalid
	}
	for _, item := range evidence {
		if item.ID == "" || item.URLOrLocation == "" || item.ApplicableScope == "" || item.ContentHash == "" {
			return Objection{}, ErrInvalid
		}
	}
	return Objection{ID: newID(), EvaluationID: evaluationID, FindingID: findingID, Explanation: explanation, Evidence: evidence, Status: "pending_reevaluation"}, nil
}
func verdictReasons(changes []FindingChange, criteriaVersion, model string) []string {
	out := []string{}
	for _, change := range changes {
		if change.Status == "resolved" || change.Status == "withdrawn_by_evidence" || change.Status == "worsened" || change.Status == "new" {
			out = append(out, change.FindingID+": "+change.Reason)
		}
	}
	if len(out) == 0 {
		out = append(out, "평가 기준 "+criteriaVersion+" 및 모델 "+model+"에 따른 재평가 결과")
	}
	return out
}
func severityRank(v string) int {
	switch v {
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
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func unique(values []string, additional ...string) []string {
	for _, v := range additional {
		if v != "" && !contains(values, v) {
			values = append(values, v)
		}
	}
	return values
}
