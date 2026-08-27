package assessment

import (
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	revision "noplanner/backend/evaluation/domains/revision"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	"strings"
)

func (s *Service) ReviewObjectionEvidence(ctx context.Context, req *evaluationv1.ReviewObjectionEvidenceRequest) (*evaluationv1.ObjectionEvidenceReview, error) {
	if req.GetFinding() == nil || strings.TrimSpace(req.GetFinding().GetId()) == "" || strings.TrimSpace(req.GetExplanation()) == "" || len(req.GetEvidence()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "검토할 Finding, 반박 설명과 검증된 근거가 필요합니다.")
	}
	allowed := map[string]bool{}
	var input strings.Builder
	finding := req.GetFinding()
	fmt.Fprintf(&input, "Finding ID: %s\nArea: %s\nStatement: %s\nFinding: %s\nSeverity: %s\nSource: %s\nObjection: %s\n", finding.GetId(), finding.GetArea(), finding.GetStatement(), finding.GetFinding(), finding.GetSeverity(), finding.GetSourceLocation(), req.GetExplanation())
	for _, evidence := range req.GetEvidence() {
		if evidence.GetId() == "" || evidence.GetUrlOrLocation() == "" || evidence.GetContentHash() == "" {
			return nil, status.Error(codes.InvalidArgument, "원문 접근과 해시가 확인된 근거만 검토할 수 있습니다.")
		}
		allowed[evidence.GetId()] = true
		fmt.Fprintf(&input, "[evidence:%s scope:%s hash:%s] %s %s\n", evidence.GetId(), evidence.GetApplicableScope(), evidence.GetContentHash(), evidence.GetTitle(), evidence.GetUrlOrLocation())
	}
	result, err := s.analyzer.ReviewObjectionEvidence(ctx, input.String())
	if err != nil {
		return nil, analyzerError(err)
	}
	if result.Status != "contradicts_finding" && result.Status != "supports_finding" && result.Status != "insufficient" || strings.TrimSpace(result.Reason) == "" || !score(result.Confidence) {
		return nil, status.Error(codes.Internal, "LLM이 유효하지 않은 이의 제기 근거 검토 결과를 반환했습니다.")
	}
	for _, id := range result.EvidenceIDs {
		if !allowed[id] {
			return nil, status.Error(codes.Internal, "이의 제기 검토가 제출되지 않은 근거를 참조했습니다.")
		}
	}
	return &evaluationv1.ObjectionEvidenceReview{Status: result.Status, Reason: result.Reason, EvidenceIds: result.EvidenceIDs, Confidence: result.Confidence}, nil
}

func (s *Service) CompareDocumentVersions(ctx context.Context, req *evaluationv1.CompareDocumentVersionsRequest) (*evaluationv1.DocumentVersionComparison, error) {
	if s.revisions == nil {
		return nil, status.Error(codes.FailedPrecondition, "재평가 저장소가 설정되지 않았습니다.")
	}
	previous := mapItems(req.GetPreviousItems())
	current := mapItems(req.GetCurrentItems())
	links := map[string][]string{}
	for _, item := range req.GetPreviousItems() {
		links[item.GetId()] = append(links[item.GetId()], item.GetRelatedFindingIds()...)
	}
	for _, item := range req.GetCurrentItems() {
		links[item.GetId()] = append(links[item.GetId()], item.GetRelatedFindingIds()...)
	}
	comparison, err := revision.Compare(req.GetDocumentId(), req.GetPreviousVersion(), req.GetCurrentVersion(), previous, current, links)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "비교할 문서 버전 입력이 올바르지 않습니다.")
	}
	comparison, err = s.revisions.SaveComparison(ctx, comparison)
	if err != nil {
		return nil, status.Error(codes.Internal, "문서 비교 결과를 저장하지 못했습니다.")
	}
	return comparisonMessage(comparison), nil
}
func (s *Service) Reevaluate(ctx context.Context, req *evaluationv1.ReevaluateRequest) (*evaluationv1.ReevaluationResult, error) {
	if s.revisions == nil {
		return nil, status.Error(codes.FailedPrecondition, "재평가 저장소가 설정되지 않았습니다.")
	}
	comparison := mapComparison(req.GetDocumentComparison())
	if comparison.DocumentID == "" {
		return nil, status.Error(codes.InvalidArgument, "문서 비교 결과가 필요합니다.")
	}
	result, err := revision.Reevaluate(comparison.DocumentID, req.GetPreviousEvaluationId(), req.GetCurrentEvaluationId(), comparison, mapFindings(req.GetPreviousFindings()), mapFindings(req.GetCurrentFindings()), req.GetPreviousVerdict(), req.GetCriteriaVersion(), req.GetModel(), req.GetObjectionId(), req.GetObjectionFindingId(), req.GetNewEvidenceIds())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "재평가 입력이 올바르지 않습니다.")
	}
	result, err = s.revisions.SaveReevaluation(ctx, result)
	if err != nil {
		return nil, status.Error(codes.Internal, "재평가 결과를 저장하지 못했습니다.")
	}
	return reevaluationMessage(result), nil
}
func (s *Service) SubmitObjection(ctx context.Context, req *evaluationv1.SubmitObjectionRequest) (*evaluationv1.Objection, error) {
	if s.revisions == nil {
		return nil, status.Error(codes.FailedPrecondition, "재평가 저장소가 설정되지 않았습니다.")
	}
	items := make([]revision.Evidence, 0, len(req.GetEvidence()))
	for _, item := range req.GetEvidence() {
		items = append(items, revision.Evidence{ID: item.GetId(), Title: item.GetTitle(), URLOrLocation: item.GetUrlOrLocation(), ApplicableScope: item.GetApplicableScope(), ContentHash: item.GetContentHash()})
	}
	objection, err := revision.NewObjection(req.GetEvaluationId(), req.GetFindingId(), req.GetExplanation(), items)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "반박 설명과 검증 가능한 추가 근거가 필요합니다.")
	}
	objection.ResearchID = req.GetResearchId()
	objection.VerificationStatus = req.GetVerificationStatus()
	objection.RemainingUncertainties = append([]string(nil), req.GetRemainingUncertainties()...)
	objection, err = s.revisions.SaveObjection(ctx, objection)
	if err != nil {
		return nil, status.Error(codes.Internal, "이의 제기를 저장하지 못했습니다.")
	}
	return objectionMessage(objection), nil
}
func (s *Service) GetVerdictHistory(ctx context.Context, req *evaluationv1.GetVerdictHistoryRequest) (*evaluationv1.VerdictHistory, error) {
	if s.revisions == nil {
		return nil, status.Error(codes.FailedPrecondition, "재평가 저장소가 설정되지 않았습니다.")
	}
	if strings.TrimSpace(req.GetDocumentId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "문서 ID가 필요합니다.")
	}
	values, err := s.revisions.ListVerdictChanges(ctx, req.GetDocumentId())
	if err != nil {
		return nil, status.Error(codes.Internal, "판정 변경 이력을 조회하지 못했습니다.")
	}
	out := &evaluationv1.VerdictHistory{DocumentId: req.GetDocumentId()}
	for _, value := range values {
		out.Changes = append(out.Changes, &evaluationv1.VerdictChange{ReevaluationId: value.ID, PreviousEvaluationId: value.PreviousEvaluationID, CurrentEvaluationId: value.CurrentEvaluationID, PreviousVerdict: value.PreviousVerdict, CurrentVerdict: value.CurrentVerdict, Reasons: value.VerdictChangeReasons, CriteriaVersion: value.CriteriaVersion, Model: value.Model, ChangedAt: timestamppb.New(value.CreatedAt)})
	}
	return out, nil
}
func mapItems(values []*evaluationv1.VersionItem) []revision.Item {
	out := make([]revision.Item, 0, len(values))
	for _, value := range values {
		out = append(out, revision.Item{ID: value.GetId(), Kind: value.GetKind(), Content: value.GetContent(), Ordinal: value.GetSourceBlockOrdinal(), Location: revisionLocation(value.GetSourceLocation())})
	}
	return out
}
func revisionLocation(value *documentv1.SourceLocation) revision.Location {
	if value == nil {
		return revision.Location{}
	}
	return revision.Location{Page: value.GetPageNumber(), Slide: value.GetSlideNumber(), Sheet: value.GetSheetName(), Sections: value.GetSectionPath(), Paragraph: value.GetParagraphNumber()}
}
func revisionProtoLocation(value revision.Location) *documentv1.SourceLocation {
	return &documentv1.SourceLocation{PageNumber: value.Page, SlideNumber: value.Slide, SheetName: value.Sheet, SectionPath: value.Sections, ParagraphNumber: value.Paragraph}
}
func changeKind(value string) evaluationv1.ChangeKind {
	return map[string]evaluationv1.ChangeKind{"added": evaluationv1.ChangeKind_CHANGE_KIND_ADDED, "removed": evaluationv1.ChangeKind_CHANGE_KIND_REMOVED, "meaning_changed": evaluationv1.ChangeKind_CHANGE_KIND_MEANING_CHANGED, "expression_only": evaluationv1.ChangeKind_CHANGE_KIND_EXPRESSION_ONLY, "unchanged": evaluationv1.ChangeKind_CHANGE_KIND_UNCHANGED}[value]
}
func itemMessage(value *revision.Item) *evaluationv1.VersionItem {
	if value == nil {
		return nil
	}
	return &evaluationv1.VersionItem{Id: value.ID, Kind: value.Kind, Content: value.Content, SourceBlockOrdinal: value.Ordinal, SourceLocation: revisionProtoLocation(value.Location)}
}
func comparisonMessage(value revision.Comparison) *evaluationv1.DocumentVersionComparison {
	out := &evaluationv1.DocumentVersionComparison{Id: value.ID, DocumentId: value.DocumentID, PreviousVersion: value.PreviousVersion, CurrentVersion: value.CurrentVersion, AffectedValidationAreas: value.AffectedAreas, AffectedFindingIds: value.AffectedFindingIDs, ResearchFirstIds: value.ResearchFirstIDs, CreatedAt: timestamppb.New(value.CreatedAt)}
	for _, change := range value.Changes {
		out.Changes = append(out.Changes, &evaluationv1.ItemChange{StableId: change.StableID, ItemKind: change.ItemKind, ChangeKind: changeKind(change.Kind), Previous: itemMessage(change.Previous), Current: itemMessage(change.Current), AffectedValidationAreas: change.AffectedAreas, AffectedFindingIds: change.AffectedFindingIDs, RequiresResearch: change.RequiresResearch, Reason: change.Reason})
	}
	return out
}
func mapComparison(value *evaluationv1.DocumentVersionComparison) revision.Comparison {
	if value == nil {
		return revision.Comparison{}
	}
	out := revision.Comparison{ID: value.GetId(), DocumentID: value.GetDocumentId(), PreviousVersion: value.GetPreviousVersion(), CurrentVersion: value.GetCurrentVersion(), AffectedAreas: value.GetAffectedValidationAreas(), AffectedFindingIDs: value.GetAffectedFindingIds(), ResearchFirstIDs: value.GetResearchFirstIds()}
	for _, change := range value.GetChanges() {
		out.Changes = append(out.Changes, revision.Change{StableID: change.GetStableId(), ItemKind: change.GetItemKind(), Kind: strings.ToLower(strings.TrimPrefix(change.GetChangeKind().String(), "CHANGE_KIND_")), AffectedAreas: change.GetAffectedValidationAreas(), AffectedFindingIDs: change.GetAffectedFindingIds(), RequiresResearch: change.GetRequiresResearch(), Reason: change.GetReason()})
	}
	return out
}
func mapFindings(values []*evaluationv1.FindingSnapshot) []revision.Finding {
	out := make([]revision.Finding, 0, len(values))
	for _, value := range values {
		out = append(out, revision.Finding{ID: value.GetId(), Area: value.GetArea(), Statement: value.GetStatement(), Finding: value.GetFinding(), Severity: value.GetSeverity(), Status: value.GetStatus(), EvidenceIDs: value.GetEvidenceIds(), SourceLocation: value.GetSourceLocation()})
	}
	return out
}
func findingSnapshot(value *revision.Finding) *evaluationv1.FindingSnapshot {
	if value == nil {
		return nil
	}
	return &evaluationv1.FindingSnapshot{Id: value.ID, Area: value.Area, Statement: value.Statement, Finding: value.Finding, Severity: value.Severity, Status: value.Status, EvidenceIds: value.EvidenceIDs, SourceLocation: value.SourceLocation}
}
func findingStatus(value string) evaluationv1.FindingChangeStatus {
	return map[string]evaluationv1.FindingChangeStatus{"resolved": evaluationv1.FindingChangeStatus_FINDING_CHANGE_STATUS_RESOLVED, "partially_resolved": evaluationv1.FindingChangeStatus_FINDING_CHANGE_STATUS_PARTIALLY_RESOLVED, "unresolved": evaluationv1.FindingChangeStatus_FINDING_CHANGE_STATUS_UNRESOLVED, "worsened": evaluationv1.FindingChangeStatus_FINDING_CHANGE_STATUS_WORSENED, "new": evaluationv1.FindingChangeStatus_FINDING_CHANGE_STATUS_NEW, "withdrawn_by_evidence": evaluationv1.FindingChangeStatus_FINDING_CHANGE_STATUS_WITHDRAWN_BY_EVIDENCE}[value]
}
func reevaluationMessage(value revision.Reevaluation) *evaluationv1.ReevaluationResult {
	out := &evaluationv1.ReevaluationResult{Id: value.ID, PreviousEvaluationId: value.PreviousEvaluationID, CurrentEvaluationId: value.CurrentEvaluationID, RerunValidationAreas: value.RerunAreas, RerunResearchItemIds: value.RerunResearchIDs, PreservedValidationAreas: value.PreservedAreas, PreviousVerdict: value.PreviousVerdict, CurrentVerdict: value.CurrentVerdict, VerdictChanged: value.VerdictChanged, VerdictChangeReasons: value.VerdictChangeReasons, CriteriaVersion: value.CriteriaVersion, Model: value.Model, CreatedAt: timestamppb.New(value.CreatedAt)}
	for _, change := range value.FindingChanges {
		out.Findings = append(out.Findings, &evaluationv1.FindingComparison{FindingId: change.FindingID, Status: findingStatus(change.Status), Previous: findingSnapshot(change.Previous), Current: findingSnapshot(change.Current), Reason: change.Reason, PreservedEvidenceIds: change.PreservedEvidenceIDs})
	}
	return out
}
func objectionMessage(value revision.Objection) *evaluationv1.Objection {
	out := &evaluationv1.Objection{Id: value.ID, EvaluationId: value.EvaluationID, FindingId: value.FindingID, Explanation: value.Explanation, Status: value.Status, ResultingReevaluationId: value.ResultingReevaluationID, CreatedAt: timestamppb.New(value.CreatedAt), ResearchId: value.ResearchID, VerificationStatus: value.VerificationStatus, RemainingUncertainties: value.RemainingUncertainties}
	for _, item := range value.Evidence {
		out.Evidence = append(out.Evidence, &evaluationv1.ObjectionEvidence{Id: item.ID, Title: item.Title, UrlOrLocation: item.URLOrLocation, ApplicableScope: item.ApplicableScope, ContentHash: item.ContentHash})
	}
	return out
}
