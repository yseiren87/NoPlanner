package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	implementation "noplanner/backend/generation/domains/implementation"
	"noplanner/backend/generation/modules/connectors"
	"noplanner/backend/generation/modules/gitrepo"
	"noplanner/backend/generation/modules/rpcerror"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

func (s *Service) AnalyzeRepository(ctx context.Context, req *generationv1.AnalyzeRepositoryRequest) (*generationv1.RepositoryAnalysis, error) {
	if !gitrepo.Allowed(req.GetRepositoryPath(), s.repositoryRoots) {
		return nil, rpcFailure(ctx, "허용된 로컬 저장소 경로 밖은 분석할 수 없습니다.")
	}
	analysis, err := gitrepo.Analyze(ctx, req.GetRepositoryPath(), req.GetIncludePaths(), req.GetHistoryLimit())
	if err != nil {
		return nil, rpcFailure(ctx, "저장소를 읽기 전용으로 분석하지 못했습니다.")
	}
	out := &generationv1.RepositoryAnalysis{Id: newImplementationID("repository"), ProjectId: req.GetProjectId(), RepositoryRoot: analysis.Root, Revision: analysis.Revision, DetectedFeatures: analysis.Features, ReadOnly: true}
	for _, file := range analysis.Files {
		out.Files = append(out.Files, &generationv1.RepositoryFile{Path: file.Path, Language: file.Language, Size: file.Size, Symbols: file.Symbols, ContentHash: file.ContentHash})
	}
	for _, commit := range analysis.History {
		out.History = append(out.History, &generationv1.CommitSummary{Revision: commit.Revision, Author: commit.Author, CommittedAt: commit.CommittedAt, Subject: commit.Subject})
	}
	s.appendHistory(ctx, implementation.HistoryItem{ID: newImplementationID("history"), ProjectID: req.GetProjectId(), Kind: "repository_analysis", SourceID: req.GetRepositoryPath(), ResultID: out.Id, Revision: out.Revision, Status: "completed"})
	return out, nil
}

func (s *Service) CompareImplementation(ctx context.Context, req *generationv1.CompareImplementationRequest) (*generationv1.ImplementationReview, error) {
	repo := req.GetRepository()
	if repo == nil {
		return nil, invalidPlanning(ctx)
	}
	domainRepo := implementation.Repository{ID: repo.GetId(), ProjectID: repo.GetProjectId(), Root: repo.GetRepositoryRoot(), Revision: repo.GetRevision(), ReadOnly: repo.GetReadOnly(), Features: repo.GetDetectedFeatures()}
	for _, file := range repo.GetFiles() {
		domainRepo.Files = append(domainRepo.Files, implementation.File{Path: file.GetPath(), Language: file.GetLanguage(), Size: file.GetSize(), Symbols: file.GetSymbols(), ContentHash: file.GetContentHash()})
	}
	expectations := []implementation.Expectation{}
	for _, item := range req.GetExpectations() {
		expectations = append(expectations, implementation.Expectation{RequirementID: item.GetRequirementId(), Statement: item.GetStatement(), AcceptanceCriteria: item.GetAcceptanceCriteria(), ExpectedPaths: item.GetExpectedPaths(), ExpectedTestPaths: item.GetExpectedTestPaths()})
	}
	review, err := implementation.Compare(req.GetEvaluationId(), domainRepo, expectations)
	if err != nil {
		return nil, invalidPlanning(ctx)
	}
	out := reviewProto(review)
	s.appendHistory(ctx, implementation.HistoryItem{ID: newImplementationID("history"), ProjectID: domainRepo.ProjectID, Kind: "implementation_review", SourceID: req.GetEvaluationId(), ResultID: review.ID, Revision: repo.GetRevision(), Status: "completed"})
	return out, nil
}

func (s *Service) CreateImplementationProposal(ctx context.Context, req *generationv1.CreateImplementationProposalRequest) (*generationv1.ImplementationProposal, error) {
	review := reviewDomain(req.GetReview())
	patches := []implementation.Patch{}
	for _, item := range req.GetPatches() {
		patches = append(patches, implementation.Patch{Path: item.GetPath(), UnifiedDiff: item.GetUnifiedDiff(), MismatchIDs: item.GetMismatchIds(), RequirementIDs: item.GetRequirementIds()})
	}
	if len(patches) == 0 {
		if s.intelligence == nil {
			return nil, rpcFailure(ctx, "구현 변경안을 생성할 Intelligence 서비스가 설정되지 않았습니다.")
		}
		input, _ := json.Marshal(req.GetReview())
		prompt := "다음 구현 불일치 각각을 해결하는 검토용 unified diff를 생성하라. 현재 작업 트리를 수정하지 말고 JSON만 반환하라. 새 파일은 /dev/null에서 시작한다. 모든 patch는 path, unified_diff, mismatch_ids, requirement_ids를 포함하고 알려진 mismatch ID만 참조해야 한다. {\"patches\":[{\"path\":\"...\",\"unified_diff\":\"--- ...\\n+++ ...\\n@@ ...\",\"mismatch_ids\":[\"...\"],\"requirement_ids\":[\"...\"]}]} review=" + string(input)
		var result struct {
			Patches []struct {
				Path           string   `json:"path"`
				UnifiedDiff    string   `json:"unified_diff"`
				MismatchIDs    []string `json:"mismatch_ids"`
				RequirementIDs []string `json:"requirement_ids"`
			} `json:"patches"`
		}
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			requestPrompt := prompt
			if attempt > 0 {
				requestPrompt += " 이전 응답은 실제 저장소에 적용되지 않거나 검증을 수행하지 않는 placeholder였다. hunk 행 수가 정확하고 실제 인수 조건을 검증하는 완전한 diff만 반환하라."
			}
			generated, err := s.intelligence.Generate(ctx, &intelligencev1.GenerateRequest{Prompt: requestPrompt, JsonResponse: true, MaxTokens: 8192})
			if err != nil {
				lastErr = err
				continue
			}
			result.Patches = nil
			if err = decodeGeneratedJSON(generated.GetText(), &result); err != nil {
				lastErr = err
				continue
			}
			candidate := make([]implementation.Patch, 0, len(result.Patches))
			diffs := make([]string, 0, len(result.Patches))
			for _, item := range result.Patches {
				candidate = append(candidate, implementation.Patch{Path: item.Path, UnifiedDiff: item.UnifiedDiff, MismatchIDs: item.MismatchIDs, RequirementIDs: item.RequirementIDs})
				diffs = append(diffs, item.UnifiedDiff)
			}
			if _, err = implementation.Propose(review, req.GetBaseRevision(), candidate); err != nil {
				lastErr = err
				continue
			}
			if err = gitrepo.ValidatePatch(ctx, review.RepositoryRoot, req.GetBaseRevision(), diffs); err != nil {
				lastErr = err
				continue
			}
			patches = candidate
			break
		}
		if len(patches) == 0 {
			_ = lastErr
			return nil, rpcFailure(ctx, "실제 저장소에 적용 가능하고 인수 조건을 검증하는 변경안을 생성하지 못했습니다.")
		}
	}
	proposal, err := implementation.Propose(review, req.GetBaseRevision(), patches)
	if err != nil {
		return nil, invalidPlanning(ctx)
	}
	diffs := make([]string, 0, len(proposal.Patches))
	for _, patch := range proposal.Patches {
		diffs = append(diffs, patch.UnifiedDiff)
	}
	if strings.TrimSpace(review.RepositoryRoot) == "" || !gitrepo.Allowed(review.RepositoryRoot, s.repositoryRoots) || gitrepo.ValidatePatch(ctx, review.RepositoryRoot, req.GetBaseRevision(), diffs) != nil {
		return nil, rpcFailure(ctx, "변경안이 기준 리비전의 허용된 저장소에 안전하게 적용되지 않습니다.")
	}
	out := &generationv1.ImplementationProposal{Id: proposal.ID, ReviewId: proposal.ReviewID, BaseRevision: proposal.BaseRevision, ProposedBranch: proposal.ProposedBranch, WorkingTreeModified: false}
	for _, item := range proposal.Patches {
		out.Patches = append(out.Patches, &generationv1.ProposedFilePatch{Path: item.Path, UnifiedDiff: item.UnifiedDiff, MismatchIds: item.MismatchIDs, RequirementIds: item.RequirementIDs})
	}
	s.appendHistory(ctx, implementation.HistoryItem{ID: newImplementationID("history"), ProjectID: review.ProjectID, Kind: "implementation_proposal", SourceID: review.ID, ResultID: proposal.ID, Revision: proposal.BaseRevision, ActorID: req.GetActorId(), Status: "proposed"})
	return out, nil
}

func (s *Service) ListConnectors(context.Context, *generationv1.ListConnectorsRequest) (*generationv1.ConnectorCatalog, error) {
	out := &generationv1.ConnectorCatalog{}
	if s.connectors == nil {
		return out, nil
	}
	for _, item := range s.connectors.List() {
		status := &generationv1.ConnectorStatus{Name: item.Name, Enabled: item.Enabled, DisabledReason: item.DisabledReason}
		for _, capability := range item.Capabilities {
			status.Capabilities = append(status.Capabilities, capabilityProto(capability))
		}
		out.Connectors = append(out.Connectors, status)
	}
	return out, nil
}

func (s *Service) DeliverIntegration(ctx context.Context, req *generationv1.DeliverIntegrationRequest) (*generationv1.IntegrationDelivery, error) {
	if s.connectors == nil {
		return nil, rpcFailure(ctx, "외부 커넥터가 설정되지 않았습니다.")
	}
	if s.implementationStore != nil && req.GetTraceId() != "" {
		items, err := s.implementationStore.History(ctx, req.GetProjectId())
		if err == nil {
			target := req.GetConnector() + ":" + req.GetTarget()
			for _, item := range items {
				if item.Kind == "integration_delivery" && item.SourceID == req.GetTraceId() && item.Target == target && item.Status == "delivered" {
					return &generationv1.IntegrationDelivery{Id: item.ResultID, Connector: req.GetConnector(), Target: req.GetTarget(), Status: "already_delivered", AuditId: item.ID}, nil
				}
			}
		}
	}
	deliveryID := newImplementationID("delivery")
	result, err := s.connectors.Deliver(ctx, req.GetConnector(), req.GetTarget(), req.GetPayload(), req.GetApprovalId())
	status := "delivered"
	approvalRequired := false
	if errors.Is(err, connectors.ErrApprovalRequired) {
		status = "approval_required"
		approvalRequired = true
	} else if err != nil {
		status = "failed"
	}
	audit := s.appendHistory(ctx, implementation.HistoryItem{ID: newImplementationID("audit"), ProjectID: req.GetProjectId(), Kind: "integration_delivery", SourceID: req.GetTraceId(), ResultID: deliveryID, Revision: "", ActorID: req.GetActorId(), Target: req.GetConnector() + ":" + req.GetTarget(), Status: status, PayloadHash: implementation.PayloadHash(req.GetPayload())})
	out := &generationv1.IntegrationDelivery{Id: deliveryID, Connector: req.GetConnector(), Target: req.GetTarget(), Status: status, ApprovalRequired: approvalRequired, AuditId: audit.ID, ExternalReference: result.ExternalReference}
	if err != nil && !approvalRequired {
		return out, rpcFailure(ctx, "외부 연동 전송에 실패했습니다.")
	}
	return out, nil
}

func (s *Service) GetImplementationHistory(ctx context.Context, req *generationv1.GetImplementationHistoryRequest) (*generationv1.ImplementationHistory, error) {
	items, err := s.implementationStore.History(ctx, req.GetProjectId())
	if err != nil {
		return nil, rpcFailure(ctx, "구현 이력을 불러오지 못했습니다.")
	}
	out := &generationv1.ImplementationHistory{ProjectId: req.GetProjectId()}
	for _, item := range items {
		out.Items = append(out.Items, &generationv1.ImplementationHistoryItem{Id: item.ID, Kind: item.Kind, SourceId: item.SourceID, ResultId: item.ResultID, Revision: item.Revision, ActorId: item.ActorID, Target: item.Target, Status: item.Status, PayloadHash: item.PayloadHash, CreatedAt: timestamppb.New(item.CreatedAt)})
	}
	return out, nil
}

func (s *Service) appendHistory(ctx context.Context, item implementation.HistoryItem) implementation.HistoryItem {
	if s.implementationStore == nil {
		return item
	}
	saved, err := s.implementationStore.Append(ctx, item)
	if err != nil {
		return item
	}
	return saved
}
func rpcFailure(ctx context.Context, message string) error {
	return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_DEPENDENCY_UNAVAILABLE, message, false)
}
func newImplementationID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
func capabilityProto(value connectors.Capability) generationv1.ConnectorCapability {
	return map[connectors.Capability]generationv1.ConnectorCapability{connectors.ReadDocuments: generationv1.ConnectorCapability_CONNECTOR_CAPABILITY_READ_INTERNAL_DOCUMENTS, connectors.WriteWorkItem: generationv1.ConnectorCapability_CONNECTOR_CAPABILITY_WRITE_WORK_ITEM, connectors.SendMessage: generationv1.ConnectorCapability_CONNECTOR_CAPABILITY_SEND_MESSAGE}[value]
}
func reviewDomain(value *generationv1.ImplementationReview) implementation.Review {
	if value == nil {
		return implementation.Review{}
	}
	out := implementation.Review{ID: value.GetId(), ProjectID: value.GetProjectId(), EvaluationID: value.GetEvaluationId(), RepositoryRevision: value.GetRepositoryRevision(), RepositoryRoot: value.GetRepositoryRoot(), SatisfiedRequirementIDs: value.GetSatisfiedRequirementIds()}
	for _, item := range value.GetMismatches() {
		out.Mismatches = append(out.Mismatches, implementation.Mismatch{ID: item.GetId(), RequirementID: item.GetRequirementId(), Kind: item.GetKind(), PlanLocation: item.GetPlanLocation(), CodeLocations: item.GetCodeLocations(), Finding: item.GetFinding(), RequiredChange: item.GetRequiredChange(), Status: item.GetStatus()})
	}
	return out
}
func reviewProto(value implementation.Review) *generationv1.ImplementationReview {
	out := &generationv1.ImplementationReview{Id: value.ID, ProjectId: value.ProjectID, EvaluationId: value.EvaluationID, RepositoryRevision: value.RepositoryRevision, RepositoryRoot: value.RepositoryRoot, SatisfiedRequirementIds: value.SatisfiedRequirementIDs}
	for _, item := range value.Mismatches {
		out.Mismatches = append(out.Mismatches, &generationv1.ImplementationMismatch{Id: item.ID, RequirementId: item.RequirementID, Kind: item.Kind, PlanLocation: item.PlanLocation, CodeLocations: item.CodeLocations, Finding: item.Finding, RequiredChange: item.RequiredChange, Status: item.Status})
	}
	return out
}
