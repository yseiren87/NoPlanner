package planning

import (
	"fmt"
	"sort"
	"strings"
)

func BuildImprovementPlan(evaluationID string, findings []Finding) (ImprovementPlan, error) {
	if strings.TrimSpace(evaluationID) == "" || len(findings) == 0 {
		return ImprovementPlan{}, ErrInvalid
	}
	result := ImprovementPlan{ID: "improvement-" + evaluationID, EvaluationID: evaluationID}
	seen := map[string]bool{}
	for _, finding := range findings {
		if finding.ID == "" || finding.RequiredAction == "" || finding.VerificationMethod == "" || seen[finding.ID] {
			return ImprovementPlan{}, ErrInvalid
		}
		seen[finding.ID] = true
		task := ImprovementTask{ID: "task-" + finding.ID, FindingIDs: []string{finding.ID}, Action: finding.RequiredAction, VerificationMethod: finding.VerificationMethod, Blocking: finding.Severity == "blocking"}
		if strings.Contains(strings.ToLower(finding.RequiredAction), "조사") || strings.Contains(strings.ToLower(finding.RequiredAction), "확인") {
			task.RequiredResearch = []string{finding.RequiredAction}
		}
		result.Tasks = append(result.Tasks, task)
	}
	sort.SliceStable(result.Tasks, func(i, j int) bool { return result.Tasks[i].Blocking && !result.Tasks[j].Blocking })
	for index := range result.Tasks {
		result.Tasks[index].Order = uint32(index + 1)
	}
	return result, nil
}

func Improve(intent string, original []Section, plan ImprovementPlan, claims []Claim) (ImprovedPlan, error) {
	if strings.TrimSpace(intent) == "" || len(original) == 0 || len(plan.Tasks) == 0 {
		return ImprovedPlan{}, ErrInvalid
	}
	result := ImprovedPlan{ID: "improved-" + plan.ID, PreservedIntent: intent, Sections: append([]Section(nil), original...)}
	for _, task := range plan.Tasks {
		var grounded *Claim
		for index := range claims {
			claim := &claims[index]
			if claim.Verified && len(claim.EvidenceIDs) > 0 && strings.TrimSpace(claim.Statement) != "" && overlaps(claim.FindingIDs, task.FindingIDs) {
				grounded = claim
				break
			}
		}
		if grounded == nil {
			result.UnresolvedFindingIDs = append(result.UnresolvedFindingIDs, task.FindingIDs...)
			continue
		}
		index := sectionForTask(result.Sections, task)
		before := result.Sections[index].Content
		result.Sections[index].Content = strings.TrimSpace(before + "\n\n보완: " + grounded.Statement)
		result.Sections[index].SourceFindingIDs = appendUnique(result.Sections[index].SourceFindingIDs, task.FindingIDs...)
		result.Sections[index].EvidenceIDs = appendUnique(result.Sections[index].EvidenceIDs, grounded.EvidenceIDs...)
		result.Changes = append(result.Changes, Change{SectionKey: result.Sections[index].Key, FindingIDs: task.FindingIDs, Before: before, After: result.Sections[index].Content, Reason: task.Action + "; 재검증: " + task.VerificationMethod})
	}
	return result, nil
}

func MissingQuestions(idea string, context Context) ([]Question, error) {
	if strings.TrimSpace(idea) == "" {
		return nil, ErrInvalid
	}
	var result []Question
	add := func(field, question, reason string) {
		result = append(result, Question{ID: "question-" + field, Field: field, Question: question, Reason: reason})
	}
	if strings.TrimSpace(context.Purpose) == "" {
		add("purpose", "이 아이디어가 해결해야 하는 문제와 기대 결과는 무엇입니까?", "목적과 성공 여부를 판단하기 위해 필요합니다.")
	}
	if strings.TrimSpace(context.TargetUser) == "" {
		add("target_user", "누가 어떤 상황에서 이 결과를 사용합니까?", "대상과 사용 맥락을 확정하기 위해 필요합니다.")
	}
	if strings.TrimSpace(context.Environment) == "" {
		add("environment", "적용할 국가·도메인·조직 환경은 어디입니까?", "현실 적합성과 제약을 조사하기 위해 필요합니다.")
	}
	if len(context.Constraints) == 0 {
		add("constraints", "반드시 지켜야 할 비용·기간·법·운영 제약이 있습니까?", "실행 불가능한 대안을 제외하기 위해 필요합니다.")
	}
	return result, nil
}

func Generate(idea string, context Context, research []ResearchFact, alternatives []Alternative, criteriaVersion, model string) (AutonomousPlan, error) {
	questions, err := MissingQuestions(idea, context)
	if err != nil || len(questions) > 0 || strings.TrimSpace(criteriaVersion) == "" {
		return AutonomousPlan{}, ErrInvalid
	}
	plan := AutonomousPlan{ID: "plan-" + stableID(idea), Idea: idea, Context: context, Research: research, Alternatives: alternatives, CriteriaVersion: criteriaVersion, Model: model}
	plan.Specification = Specification{Problem: idea, Purpose: context.Purpose, TargetUser: context.TargetUser, Scope: "검증된 문제와 선택 대안의 최소 실행 범위", OutOfScope: "근거가 확인되지 않은 확장 범위", UserFlow: []string{"대상 사용자가 문제 상황에서 진입한다.", "핵심 기능을 수행하고 결과를 확인한다.", "실패 시 원인과 후속 행동을 안내받는다."}, Policies: []string{"미검증 전제는 사실로 표시하지 않는다.", "필수 데이터와 권한이 없으면 실행을 차단한다."}, SuccessCriteria: []string{"요구사항별 인수 조건을 충족한다."}}
	for _, fact := range research {
		plan.TraceIDs = append(plan.TraceIDs, fact.ID)
		if fact.Verified && len(fact.EvidenceIDs) > 0 {
			plan.Specification.Assumptions = append(plan.Specification.Assumptions, "검증됨: "+fact.Conclusion)
		} else {
			plan.SelfEvaluation.UnresolvedBlockers = append(plan.SelfEvaluation.UnresolvedBlockers, fact.ID)
			plan.SelfEvaluation.Findings = append(plan.SelfEvaluation.Findings, Finding{ID: "finding-" + fact.ID, Severity: "blocking", RequiredAction: "근거를 확보해 전제를 검증한다.", VerificationMethod: "독립된 공식 또는 1차 출처로 재검증한다."})
		}
	}
	selected := 0
	for _, alternative := range alternatives {
		plan.TraceIDs = append(plan.TraceIDs, alternative.ID)
		if alternative.Selected {
			selected++
			plan.Specification.Scope = alternative.Description
			plan.Specification.Requirements = append(plan.Specification.Requirements, Requirement{ID: "REQ-001", Statement: alternative.Name + "을 구현한다.", AcceptanceCriteria: "선택 근거와 연결된 핵심 사용자 흐름이 정상·실패 조건에서 동작한다.", EvidenceIDs: alternative.EvidenceIDs})
		}
		if alternative.DecisionReason == "" {
			plan.SelfEvaluation.Findings = append(plan.SelfEvaluation.Findings, Finding{ID: "finding-" + alternative.ID, Severity: "high", RequiredAction: "선택 또는 비선택 이유를 기록한다.", VerificationMethod: "모든 대안의 결정 이유 존재 여부를 확인한다."})
		}
	}
	if len(research) == 0 {
		plan.SelfEvaluation.UnresolvedBlockers = append(plan.SelfEvaluation.UnresolvedBlockers, "research-missing")
	}
	if selected != 1 {
		plan.SelfEvaluation.UnresolvedBlockers = append(plan.SelfEvaluation.UnresolvedBlockers, "alternative-selection")
	}
	plan.SelfEvaluation.Verdict, plan.SelfEvaluation.Confidence = selfVerdict(plan.SelfEvaluation)
	return plan, nil
}

func sectionForTask(sections []Section, task ImprovementTask) int {
	for i, section := range sections {
		if strings.Contains(strings.ToLower(task.Action), strings.ToLower(section.Title)) {
			return i
		}
	}
	return 0
}
func appendUnique(base []string, values ...string) []string {
	seen := map[string]bool{}
	for _, v := range base {
		seen[v] = true
	}
	for _, v := range values {
		if !seen[v] {
			base = append(base, v)
			seen[v] = true
		}
	}
	return base
}
func overlaps(first, second []string) bool {
	values := map[string]bool{}
	for _, value := range first {
		values[value] = true
	}
	for _, value := range second {
		if values[value] {
			return true
		}
	}
	return false
}
func stableID(value string) string {
	var sum uint32 = 2166136261
	for _, b := range []byte(value) {
		sum ^= uint32(b)
		sum *= 16777619
	}
	return fmt.Sprintf("%08x", sum)
}
func selfVerdict(value SelfEvaluation) (string, string) {
	if len(value.UnresolvedBlockers) > 0 {
		return "not_executable", "high"
	}
	if len(value.Findings) > 0 {
		return "rewrite_required", "medium"
	}
	return "executable", "high"
}

func MergeSelfEvaluation(plan *AutonomousPlan, findings []Finding) {
	if plan == nil {
		return
	}
	seen := map[string]bool{}
	for _, finding := range plan.SelfEvaluation.Findings {
		seen[finding.ID] = true
	}
	for _, finding := range findings {
		if finding.ID == "" || seen[finding.ID] {
			continue
		}
		seen[finding.ID] = true
		plan.SelfEvaluation.Findings = append(plan.SelfEvaluation.Findings, finding)
		if finding.Severity == "blocking" {
			plan.SelfEvaluation.UnresolvedBlockers = appendUnique(plan.SelfEvaluation.UnresolvedBlockers, finding.ID)
		}
	}
	plan.SelfEvaluation.Verdict, plan.SelfEvaluation.Confidence = selfVerdict(plan.SelfEvaluation)
}
