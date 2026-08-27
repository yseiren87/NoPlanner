package assessment

import (
	"context"
	revision "noplanner/backend/evaluation/domains/revision"
	"noplanner/backend/evaluation/modules/llm"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	"testing"
)

func TestRevisionE2ECompareObjectionReevaluateAndHistory(t *testing.T) {
	store := &revision.MemoryStore{}
	service := NewWithRevisions(&memoryStore{}, store, nil, "evaluation", "1")
	comparison, err := service.CompareDocumentVersions(context.Background(), &evaluationv1.CompareDocumentVersionsRequest{DocumentId: "doc", PreviousVersion: 1, CurrentVersion: 2, PreviousItems: []*evaluationv1.VersionItem{{Id: "data", Kind: "data", Content: "개인별 데이터 사용", RelatedFindingIds: []string{"DAT-1"}}}, CurrentItems: []*evaluationv1.VersionItem{{Id: "data", Kind: "data", Content: "집계 데이터 사용", RelatedFindingIds: []string{"DAT-1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.GetAffectedValidationAreas()) == 0 || len(comparison.GetAffectedFindingIds()) == 0 {
		t.Fatalf("comparison=%#v", comparison)
	}
	objection, err := service.SubmitObjection(context.Background(), &evaluationv1.SubmitObjectionRequest{EvaluationId: "eval-1", FindingId: "DAT-1", Explanation: "공식 데이터가 추가됨", Evidence: []*evaluationv1.ObjectionEvidence{{Id: "E-new", UrlOrLocation: "https://official.test", ApplicableScope: "KR", ContentHash: "hash"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Reevaluate(context.Background(), &evaluationv1.ReevaluateRequest{PreviousEvaluationId: "eval-1", CurrentEvaluationId: "eval-2", DocumentComparison: comparison, PreviousFindings: []*evaluationv1.FindingSnapshot{{Id: "DAT-1", Area: "data", Severity: "blocking", Status: "open"}}, CurrentFindings: []*evaluationv1.FindingSnapshot{}, PreviousVerdict: "not_executable", CriteriaVersion: "2", Model: "model-2", NewEvidenceIds: []string{"E-new"}, ObjectionId: objection.GetId(), ObjectionFindingId: "DAT-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetCurrentVerdict() != "executable" || !result.GetVerdictChanged() || result.GetFindings()[0].GetStatus() != evaluationv1.FindingChangeStatus_FINDING_CHANGE_STATUS_WITHDRAWN_BY_EVIDENCE {
		t.Fatalf("result=%#v", result)
	}
	history, err := service.GetVerdictHistory(context.Background(), &evaluationv1.GetVerdictHistoryRequest{DocumentId: "doc"})
	if err != nil || len(history.GetChanges()) != 1 || history.GetChanges()[0].GetCriteriaVersion() != "2" || history.GetChanges()[0].GetModel() != "model-2" {
		t.Fatalf("history=%#v err=%v", history, err)
	}
}

func TestReviewObjectionEvidenceRequiresVerifiedSubmittedIDs(t *testing.T) {
	service := NewWithRevisions(&memoryStore{}, &revision.MemoryStore{}, analyzer{objection: llm.ObjectionEvidenceResult{Status: "contradicts_finding", Reason: "동일 범위의 공식 원문이 기존 판단을 반박함", EvidenceIDs: []string{"E-new"}, Confidence: .9}}, "evaluation", "test")
	result, err := service.ReviewObjectionEvidence(context.Background(), &evaluationv1.ReviewObjectionEvidenceRequest{Finding: &evaluationv1.FindingSnapshot{Id: "F-1", Area: "api", Finding: "API 없음", Severity: "blocking"}, Explanation: "공식 API 문서가 추가됨", Evidence: []*evaluationv1.ObjectionEvidence{{Id: "E-new", UrlOrLocation: "https://official.test", ApplicableScope: "KR", ContentHash: "hash"}}})
	if err != nil || result.GetStatus() != "contradicts_finding" || result.GetEvidenceIds()[0] != "E-new" {
		t.Fatalf("review = %#v, %v", result, err)
	}
}
