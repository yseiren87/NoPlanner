package planning

import "testing"

func TestPhase7Rules(t *testing.T) {
	plan, err := BuildImprovementPlan("eval-1", []Finding{{ID: "F-2", Severity: "high", RequiredAction: "정책 수정", VerificationMethod: "상충 재검사"}, {ID: "F-1", Severity: "blocking", RequiredAction: "데이터 조사", VerificationMethod: "공식 출처 확인"}})
	if err != nil || plan.Tasks[0].FindingIDs[0] != "F-1" {
		t.Fatalf("blocking task must be first: %#v %v", plan, err)
	}
	improved, err := Improve("원래 목적", []Section{{Key: "purpose", Title: "목적", Content: "원문"}}, plan, []Claim{{Statement: "공식 근거", EvidenceIDs: []string{"E-1"}, FindingIDs: []string{"F-1", "F-2"}, Verified: true}})
	if err != nil || improved.PreservedIntent != "원래 목적" || len(improved.Changes) != 2 || len(improved.Changes[0].FindingIDs) == 0 {
		t.Fatalf("traceable improvement required: %#v %v", improved, err)
	}
	questions, _ := MissingQuestions("아이디어", Context{Purpose: "목적", TargetUser: "사용자", Environment: "한국", Constraints: []string{"예산"}})
	if len(questions) != 0 {
		t.Fatalf("must not ask fixed questions: %#v", questions)
	}
}

func TestGenerateKeepsUnverifiedBlockers(t *testing.T) {
	result, err := Generate("아이디어", Context{Purpose: "목적", TargetUser: "사용자", Environment: "한국", Constraints: []string{"예산"}}, []ResearchFact{{ID: "R-1", Conclusion: "확인 안 됨"}}, []Alternative{{ID: "A-1", Name: "대안", Description: "범위", Selected: true, DecisionReason: "비용"}}, "criteria-1", "model")
	if err != nil || result.SelfEvaluation.Verdict != "not_executable" || len(result.SelfEvaluation.UnresolvedBlockers) == 0 {
		t.Fatalf("blocker must be visible: %#v %v", result, err)
	}
}
