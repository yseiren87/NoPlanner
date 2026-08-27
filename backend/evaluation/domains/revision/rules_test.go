package revision

import "testing"

func TestCompareSeparatesExpressionAndMeaningAndTargetsImpact(t *testing.T) {
	previous := []Item{{ID: "purpose-1", Kind: "purpose", Content: "고객의 대기 시간을 20% 줄인다."}, {ID: "data-1", Kind: "data", Content: "사용자별 실시간 이동 경로 데이터를 사용한다."}, {ID: "policy-1", Kind: "policy", Content: "관리자만 삭제할 수 있다."}}
	current := []Item{{ID: "purpose-1", Kind: "purpose", Content: "고객 대기 시간을 20% 줄인다"}, {ID: "data-1", Kind: "data", Content: "지역별 월간 집계 이동량 데이터를 사용한다."}, {ID: "policy-1", Kind: "policy", Content: "관리자만 삭제할 수 있다."}}
	comparison, err := Compare("doc-1", 1, 2, previous, current, map[string][]string{"data-1": {"DAT-001"}})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Change{}
	for _, change := range comparison.Changes {
		byID[change.StableID] = change
	}
	if byID["purpose-1"].Kind != "expression_only" || len(byID["purpose-1"].AffectedAreas) != 0 {
		t.Fatalf("expression=%#v", byID["purpose-1"])
	}
	if byID["data-1"].Kind != "meaning_changed" || !byID["data-1"].RequiresResearch || !contains(comparison.AffectedFindingIDs, "DAT-001") {
		t.Fatalf("meaning=%#v", byID["data-1"])
	}
	if byID["policy-1"].Kind != "unchanged" {
		t.Fatalf("unchanged=%#v", byID["policy-1"])
	}
}

func TestReevaluateClassifiesEveryFindingStateAndPreservesUnaffectedAreas(t *testing.T) {
	comparison := Comparison{DocumentID: "doc-1", AffectedAreas: []string{"data"}, ResearchFirstIDs: []string{"data-1"}}
	previous := []Finding{{ID: "resolved", Area: "data", Severity: "high"}, {ID: "withdrawn", Area: "data", Severity: "high", EvidenceIDs: []string{"old"}}, {ID: "partial", Area: "data", Severity: "high"}, {ID: "same", Area: "purpose", Severity: "medium"}, {ID: "worse", Area: "data", Severity: "medium"}}
	current := []Finding{{ID: "partial", Area: "data", Severity: "medium"}, {ID: "same", Area: "purpose", Severity: "medium"}, {ID: "worse", Area: "data", Severity: "blocking"}, {ID: "new", Area: "api", Severity: "low"}}
	result, err := Reevaluate("doc-1", "eval-1", "eval-2", comparison, previous, current, "not_executable", "criteria-2", "model-2", "objection-1", "withdrawn", []string{"new-evidence"})
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, change := range result.FindingChanges {
		states[change.FindingID] = change.Status
	}
	for id, want := range map[string]string{"resolved": "resolved", "withdrawn": "withdrawn_by_evidence", "partial": "partially_resolved", "same": "unresolved", "worse": "worsened", "new": "new"} {
		if states[id] != want {
			t.Fatalf("%s=%s want %s", id, states[id], want)
		}
	}
	if !contains(result.PreservedAreas, "purpose") || contains(result.PreservedAreas, "data") {
		t.Fatalf("preserved=%v", result.PreservedAreas)
	}
}

func TestObjectionRequiresEvidenceAndCannotSetVerdict(t *testing.T) {
	if _, err := NewObjection("eval", "finding", "동의하지 않음", nil); err == nil {
		t.Fatal("expected evidence validation")
	}
	objection, err := NewObjection("eval", "finding", "공식 명세가 변경됨", []Evidence{{ID: "E2", URLOrLocation: "https://official.test", ApplicableScope: "KR", ContentHash: "hash"}})
	if err != nil || objection.Status != "pending_reevaluation" {
		t.Fatalf("objection=%#v err=%v", objection, err)
	}
}
