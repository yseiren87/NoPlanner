package assessment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	domain "noplanner/backend/evaluation/domains/assessment"
	qualitydomain "noplanner/backend/evaluation/domains/quality"
	revisiondomain "noplanner/backend/evaluation/domains/revision"
	"noplanner/backend/evaluation/modules/llm"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
)

type Service struct {
	evaluationv1.UnimplementedEvaluationServiceServer
	store         domain.Store
	revisions     revisiondomain.Store
	quality       qualitydomain.Store
	analyzer      llm.Analyzer
	name, version string
}

func NewWithQuality(store domain.Store, revisions revisiondomain.Store, qualityStore qualitydomain.Store, analyzer llm.Analyzer, name, version string) *Service {
	return &Service{store: store, revisions: revisions, quality: qualityStore, analyzer: analyzer, name: name, version: version}
}

func New(store domain.Store, analyzer llm.Analyzer, name, version string) *Service {
	return &Service{store: store, analyzer: analyzer, name: name, version: version}
}
func NewWithRevisions(store domain.Store, revisions revisiondomain.Store, analyzer llm.Analyzer, name, version string) *Service {
	return &Service{store: store, revisions: revisions, analyzer: analyzer, name: name, version: version}
}

func analyzerError(err error) error {
	if errors.Is(err, llm.ErrConfiguration) {
		return status.Error(codes.FailedPrecondition, "LLM 설정이 필요합니다.")
	}
	return status.Error(codes.Unavailable, "LLM 분석 결과를 검증하지 못했습니다: "+err.Error())
}

func (s *Service) GetStatus(context.Context, *commonv1.StatusRequest) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{Service: s.name, Version: s.version, State: "ready"}, nil
}

func (s *Service) IdentifyAssumptionsAndRisks(ctx context.Context, request *evaluationv1.IdentifyAssumptionsAndRisksRequest) (*evaluationv1.AssumptionRiskAnalysis, error) {
	if strings.TrimSpace(request.GetDocumentId()) == "" || request.GetDocumentVersion() == 0 || len(request.GetElements()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "분석할 문서 요소가 필요합니다.")
	}
	locations := map[uint32]*documentv1.SourceLocation{}
	var input strings.Builder
	for _, element := range request.GetElements() {
		if element.GetSourceBlockOrdinal() == 0 || strings.TrimSpace(element.GetContent()) == "" {
			return nil, status.Error(codes.InvalidArgument, "문서 요소의 원문 위치가 필요합니다.")
		}
		locations[element.GetSourceBlockOrdinal()] = element.GetSourceLocation()
		fmt.Fprintf(&input, "[block:%d element:%s]\n%s\n", element.GetSourceBlockOrdinal(), element.GetType().String(), element.GetContent())
	}
	for _, dependency := range request.GetDependencies() {
		fmt.Fprintf(&input, "[block:%d dependency:%s]\n%s\n", dependency.GetSourceBlockOrdinal(), dependency.GetType().String(), dependency.GetContent())
	}
	var analysis domain.Analysis
	var validationErr error
	for attempt := 0; attempt < 2; attempt++ {
		result, err := s.analyzer.Analyze(ctx, input.String())
		if err != nil {
			return nil, analyzerError(err)
		}
		analysis, validationErr = buildAssumptionRiskAnalysis(request, result, locations, s.analyzer.Model())
		if validationErr == nil {
			break
		}
	}
	if validationErr != nil {
		return nil, status.Error(codes.Internal, validationErr.Error())
	}
	var err error
	analysis, err = s.store.Create(ctx, analysis)
	if err != nil {
		if errors.Is(err, domain.ErrInvalid) {
			return nil, status.Error(codes.InvalidArgument, "평가 입력값을 확인해 주세요.")
		}
		return nil, status.Error(codes.Internal, "평가 결과를 저장하지 못했습니다.")
	}
	return message(analysis), nil
}

func buildAssumptionRiskAnalysis(request *evaluationv1.IdentifyAssumptionsAndRisksRequest, result llm.Result, locations map[uint32]*documentv1.SourceLocation, model string) (domain.Analysis, error) {
	analysis := domain.Analysis{DocumentID: request.GetDocumentId(), DocumentVersion: request.GetDocumentVersion(), Model: model}
	keys := map[string]string{}
	for index, value := range result.Assumptions {
		if !validText(value.Statement, value.FailureImpact, value.Confidence, value.SourceBlockOrdinal, locations) || (value.Criticality != "critical" && value.Criticality != "general") || strings.TrimSpace(value.CriticalityReason) == "" || strings.TrimSpace(value.Key) == "" {
			return domain.Analysis{}, errors.New("LLM이 유효하지 않은 전제를 반환했습니다.")
		}
		id := identifier(request.GetDocumentId(), request.GetDocumentVersion(), "assumption", index, value.Statement)
		if _, exists := keys[value.Key]; exists {
			return domain.Analysis{}, errors.New("LLM이 중복 전제 키를 반환했습니다.")
		}
		keys[value.Key] = id
		analysis.Assumptions = append(analysis.Assumptions, domain.Assumption{ID: id, Statement: value.Statement, Criticality: value.Criticality, CriticalityReason: value.CriticalityReason, FailureImpact: value.FailureImpact, Confidence: value.Confidence, SourceBlockOrdinal: value.SourceBlockOrdinal, Location: location(locations[value.SourceBlockOrdinal])})
	}
	for index, value := range result.Risks {
		if !validText(value.Statement, value.FailureImpact, value.Confidence, value.SourceBlockOrdinal, locations) || !score(value.Likelihood) || !score(value.Impact) || !score(value.Reversibility) {
			return domain.Analysis{}, errors.New("LLM이 유효하지 않은 위험을 반환했습니다.")
		}
		links := make([]string, 0, len(value.AssumptionKeys))
		for _, key := range value.AssumptionKeys {
			id, ok := keys[key]
			if !ok {
				return domain.Analysis{}, errors.New("위험이 존재하지 않는 전제를 참조합니다.")
			}
			links = append(links, id)
		}
		analysis.Risks = append(analysis.Risks, domain.Risk{ID: identifier(request.GetDocumentId(), request.GetDocumentVersion(), "risk", index, value.Statement), Statement: value.Statement, FailureImpact: value.FailureImpact, Likelihood: value.Likelihood, Impact: value.Impact, Reversibility: value.Reversibility, Confidence: value.Confidence, AssumptionIDs: links, SourceBlockOrdinal: value.SourceBlockOrdinal, Location: location(locations[value.SourceBlockOrdinal])})
	}
	return analysis, nil
}

var validationAreas = []string{"purpose", "problem", "context", "evidence", "data", "api", "technology", "resource", "consistency", "completeness", "verifiability", "operations"}

func (s *Service) CreateValidationPlan(ctx context.Context, request *evaluationv1.CreateValidationPlanRequest) (*evaluationv1.ValidationPlan, error) {
	if strings.TrimSpace(request.GetDocumentId()) == "" || request.GetDocumentVersion() == 0 || strings.TrimSpace(request.GetPurpose()) == "" {
		return nil, status.Error(codes.InvalidArgument, "문서와 기획 목적이 필요합니다.")
	}
	drivers := map[string]struct{}{}
	requiredDrivers := map[string]struct{}{}
	requiredAreas := map[string]struct{}{}
	if strings.TrimSpace(request.GetCountry()) == "" || strings.TrimSpace(request.GetDomain()) == "" {
		requiredAreas["context"] = struct{}{}
	}
	var input strings.Builder
	fmt.Fprintf(&input, "Purpose: %s\nCountry: %s\nDomain: %s\n", request.GetPurpose(), knownContext(request.GetCountry()), knownContext(request.GetDomain()))
	for _, value := range request.GetAssumptions() {
		if value.GetId() == "" {
			return nil, status.Error(codes.InvalidArgument, "전제 식별자가 필요합니다.")
		}
		drivers[value.GetId()] = struct{}{}
		if value.GetCriticality() == evaluationv1.AssumptionCriticality_ASSUMPTION_CRITICALITY_CRITICAL {
			requiredDrivers[value.GetId()] = struct{}{}
		}
		fmt.Fprintf(&input, "[assumption:%s criticality:%s] %s; failure impact: %s\n", value.GetId(), value.GetCriticality().String(), value.GetStatement(), value.GetFailureImpact())
	}
	for _, value := range request.GetRisks() {
		if value.GetId() == "" {
			return nil, status.Error(codes.InvalidArgument, "위험 식별자가 필요합니다.")
		}
		drivers[value.GetId()] = struct{}{}
		if value.GetImpact() >= .7 || value.GetLikelihood() >= .7 {
			requiredDrivers[value.GetId()] = struct{}{}
		}
		fmt.Fprintf(&input, "[risk:%s likelihood:%.2f impact:%.2f reversibility:%.2f] %s\n", value.GetId(), value.GetLikelihood(), value.GetImpact(), value.GetReversibility(), value.GetStatement())
	}
	for _, value := range request.GetDependencies() {
		if value.GetId() == "" {
			return nil, status.Error(codes.InvalidArgument, "의존성 식별자가 필요합니다.")
		}
		drivers[value.GetId()] = struct{}{}
		area := map[documentv1.DependencyType]string{documentv1.DependencyType_DEPENDENCY_TYPE_DATA: "data", documentv1.DependencyType_DEPENDENCY_TYPE_API: "api", documentv1.DependencyType_DEPENDENCY_TYPE_TECHNOLOGY: "technology", documentv1.DependencyType_DEPENDENCY_TYPE_PERMISSION: "operations", documentv1.DependencyType_DEPENDENCY_TYPE_RESOURCE: "resource"}[value.GetType()]
		if area != "" {
			requiredAreas[area] = struct{}{}
		}
		fmt.Fprintf(&input, "[dependency:%s type:%s] %s\n", value.GetId(), value.GetType().String(), value.GetContent())
	}
	result, err := s.analyzer.Plan(ctx, input.String())
	if err != nil {
		return nil, analyzerError(err)
	}
	result.Items = normalizePlanDrivers(result.Items, drivers)
	result.Items = applyRequiredPlanPolicy(result.Items, requiredDrivers, requiredAreas)
	if err := validatePlan(result.Items, drivers, requiredDrivers, requiredAreas); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	plan := domain.ValidationPlan{DocumentID: request.GetDocumentId(), DocumentVersion: request.GetDocumentVersion(), Purpose: strings.TrimSpace(request.GetPurpose()), Country: strings.TrimSpace(request.GetCountry()), Domain: strings.TrimSpace(request.GetDomain()), Model: s.analyzer.Model()}
	for _, item := range result.Items {
		plan.Items = append(plan.Items, domain.ValidationPlanItem{Area: item.Area, Selected: item.Selected, Depth: item.Depth, Reason: item.Reason, DriverIDs: append([]string(nil), item.DriverIDs...)})
	}
	plan, err = s.store.CreateValidationPlan(ctx, plan)
	if err != nil {
		return nil, status.Error(codes.Internal, "검증 계획을 저장하지 못했습니다.")
	}
	return planMessage(plan), nil
}

func (s *Service) RecordValidationResults(ctx context.Context, request *evaluationv1.RecordValidationResultsRequest) (*evaluationv1.ValidationResultSummary, error) {
	statuses := map[evaluationv1.ValidationStatus]string{
		evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED: "satisfied", evaluationv1.ValidationStatus_VALIDATION_STATUS_PARTIALLY_SATISFIED: "partially_satisfied",
		evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED: "not_satisfied", evaluationv1.ValidationStatus_VALIDATION_STATUS_UNVERIFIED: "unverified",
		evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_APPLICABLE: "not_applicable", evaluationv1.ValidationStatus_VALIDATION_STATUS_SKIPPED: "skipped",
	}
	areas := validationAreaNames()
	values := make([]domain.ValidationResult, 0, len(request.GetResults()))
	for _, result := range request.GetResults() {
		area, areaOK := areas[result.GetArea()]
		state, statusOK := statuses[result.GetStatus()]
		if !areaOK || !statusOK || strings.TrimSpace(result.GetReason()) == "" {
			return nil, status.Error(codes.InvalidArgument, "적용 상태, 평가 영역과 상태 근거가 필요합니다.")
		}
		values = append(values, domain.ValidationResult{Area: area, Status: state, Reason: strings.TrimSpace(result.GetReason())})
	}
	summary, err := domain.Summarize(request.GetValidationPlanId(), values)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "적용 상태 입력값을 확인해 주세요.")
	}
	summary, err = s.store.CreateValidationResultSummary(ctx, summary)
	if err != nil {
		return nil, status.Error(codes.Internal, "적용 상태를 저장하지 못했습니다.")
	}
	return resultSummaryMessage(summary), nil
}

func (s *Service) EvaluatePurposeAlignment(ctx context.Context, request *evaluationv1.EvaluatePurposeAlignmentRequest) (*evaluationv1.PurposeAlignmentEvaluation, error) {
	if strings.TrimSpace(request.GetDocumentId()) == "" || request.GetDocumentVersion() == 0 || len(request.GetElements()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "정합성을 검증할 문서 요소가 필요합니다.")
	}
	locations := map[uint32]*documentv1.SourceLocation{}
	var input strings.Builder
	for _, element := range request.GetElements() {
		if element.GetSourceBlockOrdinal() == 0 || strings.TrimSpace(element.GetContent()) == "" {
			return nil, status.Error(codes.InvalidArgument, "문서 요소의 원문 위치가 필요합니다.")
		}
		locations[element.GetSourceBlockOrdinal()] = element.GetSourceLocation()
		fmt.Fprintf(&input, "[block:%d element:%s]\n%s\n", element.GetSourceBlockOrdinal(), element.GetType().String(), element.GetContent())
	}
	result, err := s.analyzer.EvaluatePurposeAlignment(ctx, input.String())
	if err != nil {
		return nil, analyzerError(err)
	}
	evaluation := domain.PurposeAlignmentEvaluation{DocumentID: request.GetDocumentId(), DocumentVersion: request.GetDocumentVersion(), Status: "satisfied", Model: s.analyzer.Model()}
	for index, value := range result.Findings {
		if !validAlignmentFinding(value, locations) {
			value = unverifiableAlignmentFinding(value)
		}
		finding := domain.Finding{ID: identifier(request.GetDocumentId(), request.GetDocumentVersion(), "purpose-finding", index, value.Finding), Type: value.Type, Statement: value.Statement, Finding: value.Finding, ReasoningSummary: value.ReasoningSummary, Impact: value.Impact, Severity: value.Severity, Confidence: value.Confidence, RequiredAction: value.RequiredAction, SourceBlockOrdinals: append([]uint32(nil), value.SourceBlockOrdinals...), DocumentAbsence: value.DocumentAbsence}
		for _, ordinal := range value.SourceBlockOrdinals {
			finding.SourceLocations = append(finding.SourceLocations, location(locations[ordinal]))
		}
		evaluation.Findings = append(evaluation.Findings, finding)
		if value.Severity == "high" || value.Severity == "blocking" {
			evaluation.Status = "not_satisfied"
		} else if evaluation.Status == "satisfied" {
			evaluation.Status = "partially_satisfied"
		}
	}
	evaluation, err = s.store.CreatePurposeAlignmentEvaluation(ctx, evaluation)
	if err != nil {
		return nil, status.Error(codes.Internal, "목적 정합성 결과를 저장하지 못했습니다.")
	}
	return alignmentMessage(evaluation), nil
}

func (s *Service) DetectContradictions(ctx context.Context, request *evaluationv1.DetectContradictionsRequest) (*evaluationv1.ContradictionEvaluation, error) {
	if strings.TrimSpace(request.GetDocumentId()) == "" || request.GetDocumentVersion() == 0 || (len(request.GetElements())+len(request.GetRequirements()) < 2) {
		return nil, status.Error(codes.InvalidArgument, "모순을 비교할 문서 요소가 두 개 이상 필요합니다.")
	}
	locations := map[uint32]*documentv1.SourceLocation{}
	var input strings.Builder
	for _, value := range request.GetElements() {
		if value.GetSourceBlockOrdinal() == 0 || strings.TrimSpace(value.GetContent()) == "" {
			return nil, status.Error(codes.InvalidArgument, "문서 요소의 원문 위치가 필요합니다.")
		}
		locations[value.GetSourceBlockOrdinal()] = value.GetSourceLocation()
		fmt.Fprintf(&input, "[block:%d element:%s]\n%s\n", value.GetSourceBlockOrdinal(), value.GetType().String(), value.GetContent())
	}
	for _, value := range request.GetRequirements() {
		if value.GetSourceBlockOrdinal() == 0 || strings.TrimSpace(value.GetContent()) == "" {
			return nil, status.Error(codes.InvalidArgument, "요구사항의 원문 위치가 필요합니다.")
		}
		locations[value.GetSourceBlockOrdinal()] = value.GetSourceLocation()
		fmt.Fprintf(&input, "[block:%d requirement:%s]\n%s\n", value.GetSourceBlockOrdinal(), value.GetType().String(), value.GetContent())
	}
	result, err := s.analyzer.DetectContradictions(ctx, input.String())
	if err != nil {
		return nil, analyzerError(err)
	}
	evaluation := domain.ContradictionEvaluation{DocumentID: request.GetDocumentId(), DocumentVersion: request.GetDocumentVersion(), Status: "satisfied", Model: s.analyzer.Model()}
	for index, value := range result.Contradictions {
		if !validContradiction(value, locations) {
			return nil, status.Error(codes.Internal, "LLM이 유효하지 않은 모순을 반환했습니다.")
		}
		finding := domain.Finding{ID: identifier(request.GetDocumentId(), request.GetDocumentVersion(), "contradiction", index, value.Finding), Type: "contradiction", Statement: value.FirstStatement + " ↔ " + value.SecondStatement, Finding: value.Finding, ReasoningSummary: value.ReasoningSummary, Impact: value.Impact, Severity: value.Severity, Confidence: value.Confidence, RequiredAction: value.RequiredAction, SourceBlockOrdinals: append([]uint32(nil), value.SourceBlockOrdinals...)}
		for _, ordinal := range value.SourceBlockOrdinals {
			finding.SourceLocations = append(finding.SourceLocations, location(locations[ordinal]))
		}
		evaluation.Contradictions = append(evaluation.Contradictions, domain.Contradiction{Finding: finding, Category: value.Category, FirstStatement: value.FirstStatement, SecondStatement: value.SecondStatement})
		if value.Severity == "high" || value.Severity == "blocking" {
			evaluation.Status = "not_satisfied"
		} else if evaluation.Status == "satisfied" {
			evaluation.Status = "partially_satisfied"
		}
	}
	evaluation, err = s.store.CreateContradictionEvaluation(ctx, evaluation)
	if err != nil {
		return nil, status.Error(codes.Internal, "문서 모순 결과를 저장하지 못했습니다.")
	}
	return contradictionMessage(evaluation), nil
}

func (s *Service) EvaluateRequirementCompleteness(ctx context.Context, request *evaluationv1.EvaluateRequirementCompletenessRequest) (*evaluationv1.RequirementCompletenessEvaluation, error) {
	if strings.TrimSpace(request.GetDocumentId()) == "" || request.GetDocumentVersion() == 0 || len(request.GetRequirements()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "완전성을 검증할 요구사항이 필요합니다.")
	}
	locations := map[uint32]*documentv1.SourceLocation{}
	var input strings.Builder
	for _, value := range request.GetRequirements() {
		if value.GetSourceBlockOrdinal() == 0 || strings.TrimSpace(value.GetContent()) == "" {
			return nil, status.Error(codes.InvalidArgument, "요구사항의 원문 위치가 필요합니다.")
		}
		locations[value.GetSourceBlockOrdinal()] = value.GetSourceLocation()
		fmt.Fprintf(&input, "[block:%d requirement:%s]\n%s\n", value.GetSourceBlockOrdinal(), value.GetType().String(), value.GetContent())
	}
	for _, value := range request.GetDependencies() {
		if value.GetSourceBlockOrdinal() == 0 || strings.TrimSpace(value.GetContent()) == "" {
			return nil, status.Error(codes.InvalidArgument, "의존성의 원문 위치가 필요합니다.")
		}
		locations[value.GetSourceBlockOrdinal()] = value.GetSourceLocation()
		fmt.Fprintf(&input, "[block:%d dependency:%s]\n%s\n", value.GetSourceBlockOrdinal(), value.GetType().String(), value.GetContent())
	}
	result, err := s.analyzer.EvaluateRequirementCompleteness(ctx, input.String())
	if err != nil {
		return nil, analyzerError(err)
	}
	evaluation := domain.RequirementCompletenessEvaluation{DocumentID: request.GetDocumentId(), DocumentVersion: request.GetDocumentVersion(), Status: "satisfied", Model: s.analyzer.Model()}
	for index, value := range result.Gaps {
		if !validRequirementGap(value, locations) {
			return nil, status.Error(codes.Internal, "LLM이 유효하지 않은 요구사항 누락을 반환했습니다.")
		}
		finding := domain.Finding{ID: identifier(request.GetDocumentId(), request.GetDocumentVersion(), "requirement-gap", index, value.Finding), Type: "incomplete_requirement", Statement: value.Statement, Finding: value.Finding, ReasoningSummary: value.ReasoningSummary, Impact: value.Impact, Severity: value.Severity, Confidence: value.Confidence, RequiredAction: value.MissingDecision, SourceBlockOrdinals: append([]uint32(nil), value.SourceBlockOrdinals...), DocumentAbsence: true}
		for _, ordinal := range value.SourceBlockOrdinals {
			finding.SourceLocations = append(finding.SourceLocations, location(locations[ordinal]))
		}
		evaluation.Gaps = append(evaluation.Gaps, domain.RequirementGap{Finding: finding, Category: value.Category, MissingDecision: value.MissingDecision})
		if value.Severity == "high" || value.Severity == "blocking" {
			evaluation.Status = "not_satisfied"
		} else if evaluation.Status == "satisfied" {
			evaluation.Status = "partially_satisfied"
		}
	}
	evaluation, err = s.store.CreateRequirementCompletenessEvaluation(ctx, evaluation)
	if err != nil {
		return nil, status.Error(codes.Internal, "요구사항 완전성 결과를 저장하지 못했습니다.")
	}
	return completenessMessage(evaluation), nil
}

func (s *Service) EvaluateDocumentWorkQuality(ctx context.Context, request *evaluationv1.EvaluateDocumentWorkQualityRequest) (*evaluationv1.DocumentWorkQualityEvaluation, error) {
	if strings.TrimSpace(request.GetDocumentId()) == "" || request.GetDocumentVersion() == 0 || len(request.GetElements())+len(request.GetRequirements()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "수행 품질을 평가할 문서 내용이 필요합니다.")
	}
	locations := map[uint32]*documentv1.SourceLocation{}
	var input strings.Builder
	for _, value := range request.GetElements() {
		if value.GetSourceBlockOrdinal() == 0 || strings.TrimSpace(value.GetContent()) == "" {
			return nil, status.Error(codes.InvalidArgument, "문서 요소의 원문 위치가 필요합니다.")
		}
		locations[value.GetSourceBlockOrdinal()] = value.GetSourceLocation()
		fmt.Fprintf(&input, "[block:%d element:%s]\n%s\n", value.GetSourceBlockOrdinal(), value.GetType().String(), value.GetContent())
	}
	for _, value := range request.GetRequirements() {
		if value.GetSourceBlockOrdinal() == 0 || strings.TrimSpace(value.GetContent()) == "" {
			return nil, status.Error(codes.InvalidArgument, "요구사항의 원문 위치가 필요합니다.")
		}
		locations[value.GetSourceBlockOrdinal()] = value.GetSourceLocation()
		fmt.Fprintf(&input, "[block:%d requirement:%s]\n%s\n", value.GetSourceBlockOrdinal(), value.GetType().String(), value.GetContent())
	}
	result, err := s.analyzer.EvaluateDocumentWorkQuality(ctx, input.String())
	if err != nil {
		return nil, analyzerError(err)
	}
	if !validWorkQuality(result.Results, locations) {
		return nil, status.Error(codes.Internal, "LLM이 유효하지 않은 수행 품질 결과를 반환했습니다.")
	}
	evaluation := domain.DocumentWorkQualityEvaluation{DocumentID: request.GetDocumentId(), DocumentVersion: request.GetDocumentVersion(), Model: s.analyzer.Model()}
	for _, value := range result.Results {
		item := domain.WorkQualityResult{Dimension: value.Dimension, Status: value.Status, Finding: value.Finding, ReasoningSummary: value.ReasoningSummary, Confidence: value.Confidence, SourceBlockOrdinals: append([]uint32(nil), value.SourceBlockOrdinals...)}
		for _, ordinal := range value.SourceBlockOrdinals {
			item.SourceLocations = append(item.SourceLocations, location(locations[ordinal]))
		}
		evaluation.Results = append(evaluation.Results, item)
	}
	evaluation, err = s.store.CreateDocumentWorkQualityEvaluation(ctx, evaluation)
	if err != nil {
		return nil, status.Error(codes.Internal, "문서 수행 품질 결과를 저장하지 못했습니다.")
	}
	return workQualityMessage(evaluation), nil
}

func validWorkQuality(items []llm.WorkQualityItem, locations map[uint32]*documentv1.SourceLocation) bool {
	if len(items) != 3 {
		return false
	}
	dimensions := map[string]bool{"research": true, "validation": true, "logic": true}
	statuses := map[string]bool{"satisfied": true, "partially_satisfied": true, "not_satisfied": true, "unverified": true}
	for _, item := range items {
		if !dimensions[item.Dimension] || !statuses[item.Status] || !score(item.Confidence) || strings.TrimSpace(item.Finding) == "" || strings.TrimSpace(item.ReasoningSummary) == "" || len(item.SourceBlockOrdinals) == 0 {
			return false
		}
		delete(dimensions, item.Dimension)
		seen := map[uint32]bool{}
		for _, ordinal := range item.SourceBlockOrdinals {
			if seen[ordinal] {
				return false
			}
			seen[ordinal] = true
			if _, ok := locations[ordinal]; !ok {
				return false
			}
		}
	}
	return len(dimensions) == 0
}

func workQualityMessage(value domain.DocumentWorkQualityEvaluation) *evaluationv1.DocumentWorkQualityEvaluation {
	dimensions := map[string]evaluationv1.WorkQualityDimension{"research": evaluationv1.WorkQualityDimension_WORK_QUALITY_DIMENSION_RESEARCH, "validation": evaluationv1.WorkQualityDimension_WORK_QUALITY_DIMENSION_VALIDATION, "logic": evaluationv1.WorkQualityDimension_WORK_QUALITY_DIMENSION_LOGIC}
	statuses := map[string]evaluationv1.ValidationStatus{"satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED, "partially_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_PARTIALLY_SATISFIED, "not_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED, "unverified": evaluationv1.ValidationStatus_VALIDATION_STATUS_UNVERIFIED}
	result := &evaluationv1.DocumentWorkQualityEvaluation{Id: value.ID, DocumentId: value.DocumentID, DocumentVersion: value.DocumentVersion, Model: value.Model, CreatedAt: timestamppb.New(value.CreatedAt)}
	for _, item := range value.Results {
		entry := &evaluationv1.WorkQualityResult{Dimension: dimensions[item.Dimension], Status: statuses[item.Status], Finding: item.Finding, ReasoningSummary: item.ReasoningSummary, Confidence: item.Confidence, SourceBlockOrdinals: append([]uint32(nil), item.SourceBlockOrdinals...)}
		for _, source := range item.SourceLocations {
			entry.SourceLocations = append(entry.SourceLocations, protoLocation(source))
		}
		result.Results = append(result.Results, entry)
	}
	return result
}

func validRequirementGap(value llm.RequirementGap, locations map[uint32]*documentv1.SourceLocation) bool {
	categories := map[string]bool{"normal_flow": true, "exception": true, "failure": true, "permission": true, "state": true, "boundary": true}
	severities := map[string]bool{"low": true, "medium": true, "high": true, "blocking": true}
	if !categories[value.Category] || !severities[value.Severity] || !score(value.Confidence) || strings.TrimSpace(value.Statement) == "" || strings.TrimSpace(value.Finding) == "" || strings.TrimSpace(value.ReasoningSummary) == "" || strings.TrimSpace(value.Impact) == "" || strings.TrimSpace(value.MissingDecision) == "" || len(value.SourceBlockOrdinals) == 0 {
		return false
	}
	seen := map[uint32]bool{}
	for _, ordinal := range value.SourceBlockOrdinals {
		if seen[ordinal] {
			return false
		}
		seen[ordinal] = true
		if _, ok := locations[ordinal]; !ok {
			return false
		}
	}
	return true
}

func completenessMessage(value domain.RequirementCompletenessEvaluation) *evaluationv1.RequirementCompletenessEvaluation {
	statuses := map[string]evaluationv1.ValidationStatus{"satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED, "partially_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_PARTIALLY_SATISFIED, "not_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED}
	categories := map[string]evaluationv1.RequirementGapCategory{"normal_flow": evaluationv1.RequirementGapCategory_REQUIREMENT_GAP_CATEGORY_NORMAL_FLOW, "exception": evaluationv1.RequirementGapCategory_REQUIREMENT_GAP_CATEGORY_EXCEPTION, "failure": evaluationv1.RequirementGapCategory_REQUIREMENT_GAP_CATEGORY_FAILURE, "permission": evaluationv1.RequirementGapCategory_REQUIREMENT_GAP_CATEGORY_PERMISSION, "state": evaluationv1.RequirementGapCategory_REQUIREMENT_GAP_CATEGORY_STATE, "boundary": evaluationv1.RequirementGapCategory_REQUIREMENT_GAP_CATEGORY_BOUNDARY}
	severities := map[string]evaluationv1.Severity{"low": evaluationv1.Severity_SEVERITY_LOW, "medium": evaluationv1.Severity_SEVERITY_MEDIUM, "high": evaluationv1.Severity_SEVERITY_HIGH, "blocking": evaluationv1.Severity_SEVERITY_BLOCKING}
	result := &evaluationv1.RequirementCompletenessEvaluation{Id: value.ID, DocumentId: value.DocumentID, DocumentVersion: value.DocumentVersion, Status: statuses[value.Status], Model: value.Model, CreatedAt: timestamppb.New(value.CreatedAt)}
	for _, item := range value.Gaps {
		finding := &evaluationv1.Finding{Id: item.ID, Area: evaluationv1.ValidationArea_VALIDATION_AREA_COMPLETENESS, Type: evaluationv1.FindingType_FINDING_TYPE_INCOMPLETE_REQUIREMENT, Statement: item.Statement, Finding: item.Finding.Finding, ReasoningSummary: item.ReasoningSummary, Impact: item.Impact, Severity: severities[item.Severity], Confidence: item.Confidence, RequiredAction: item.RequiredAction, SourceBlockOrdinals: append([]uint32(nil), item.SourceBlockOrdinals...), DocumentAbsence: true}
		for _, source := range item.SourceLocations {
			finding.SourceLocations = append(finding.SourceLocations, protoLocation(source))
		}
		result.Gaps = append(result.Gaps, &evaluationv1.RequirementGap{Finding: finding, Category: categories[item.Category], MissingDecision: item.MissingDecision})
	}
	return result
}

func validContradiction(value llm.Contradiction, locations map[uint32]*documentv1.SourceLocation) bool {
	categories := map[string]bool{"claim": true, "figure": true, "policy": true, "schedule": true, "requirement": true}
	severities := map[string]bool{"low": true, "medium": true, "high": true, "blocking": true}
	if !categories[value.Category] || !severities[value.Severity] || !score(value.Confidence) || strings.TrimSpace(value.FirstStatement) == "" || strings.TrimSpace(value.SecondStatement) == "" || strings.TrimSpace(value.Finding) == "" || strings.TrimSpace(value.ReasoningSummary) == "" || strings.TrimSpace(value.Impact) == "" || strings.TrimSpace(value.RequiredAction) == "" || len(value.SourceBlockOrdinals) < 2 {
		return false
	}
	seen := map[uint32]bool{}
	for _, ordinal := range value.SourceBlockOrdinals {
		if seen[ordinal] {
			return false
		}
		seen[ordinal] = true
		if _, ok := locations[ordinal]; !ok {
			return false
		}
	}
	return true
}

func contradictionMessage(value domain.ContradictionEvaluation) *evaluationv1.ContradictionEvaluation {
	statuses := map[string]evaluationv1.ValidationStatus{"satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED, "partially_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_PARTIALLY_SATISFIED, "not_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED}
	categories := map[string]evaluationv1.ContradictionCategory{"claim": evaluationv1.ContradictionCategory_CONTRADICTION_CATEGORY_CLAIM, "figure": evaluationv1.ContradictionCategory_CONTRADICTION_CATEGORY_FIGURE, "policy": evaluationv1.ContradictionCategory_CONTRADICTION_CATEGORY_POLICY, "schedule": evaluationv1.ContradictionCategory_CONTRADICTION_CATEGORY_SCHEDULE, "requirement": evaluationv1.ContradictionCategory_CONTRADICTION_CATEGORY_REQUIREMENT}
	severities := map[string]evaluationv1.Severity{"low": evaluationv1.Severity_SEVERITY_LOW, "medium": evaluationv1.Severity_SEVERITY_MEDIUM, "high": evaluationv1.Severity_SEVERITY_HIGH, "blocking": evaluationv1.Severity_SEVERITY_BLOCKING}
	result := &evaluationv1.ContradictionEvaluation{Id: value.ID, DocumentId: value.DocumentID, DocumentVersion: value.DocumentVersion, Status: statuses[value.Status], Model: value.Model, CreatedAt: timestamppb.New(value.CreatedAt)}
	for _, item := range value.Contradictions {
		finding := &evaluationv1.Finding{Id: item.ID, Area: evaluationv1.ValidationArea_VALIDATION_AREA_CONSISTENCY, Type: evaluationv1.FindingType_FINDING_TYPE_CONTRADICTION, Statement: item.Statement, Finding: item.Finding.Finding, ReasoningSummary: item.ReasoningSummary, Impact: item.Impact, Severity: severities[item.Severity], Confidence: item.Confidence, RequiredAction: item.RequiredAction, SourceBlockOrdinals: append([]uint32(nil), item.SourceBlockOrdinals...)}
		for _, source := range item.SourceLocations {
			finding.SourceLocations = append(finding.SourceLocations, protoLocation(source))
		}
		result.Contradictions = append(result.Contradictions, &evaluationv1.Contradiction{Finding: finding, Category: categories[item.Category], FirstStatement: item.FirstStatement, SecondStatement: item.SecondStatement})
	}
	return result
}

func validAlignmentFinding(value llm.AlignmentFinding, locations map[uint32]*documentv1.SourceLocation) bool {
	types := map[string]bool{"missing": true, "purposeless_feature": true, "logical_gap": true, "unmeasurable_goal": true}
	severities := map[string]bool{"low": true, "medium": true, "high": true, "blocking": true}
	if !types[value.Type] || !severities[value.Severity] || !score(value.Confidence) || strings.TrimSpace(value.Statement) == "" || strings.TrimSpace(value.Finding) == "" || strings.TrimSpace(value.ReasoningSummary) == "" || strings.TrimSpace(value.Impact) == "" || strings.TrimSpace(value.RequiredAction) == "" {
		return false
	}
	if value.DocumentAbsence {
		return len(value.SourceBlockOrdinals) == 0
	}
	if len(value.SourceBlockOrdinals) == 0 {
		return false
	}
	for _, ordinal := range value.SourceBlockOrdinals {
		if _, ok := locations[ordinal]; !ok {
			return false
		}
	}
	return true
}

func unverifiableAlignmentFinding(value llm.AlignmentFinding) llm.AlignmentFinding {
	statement := strings.TrimSpace(value.Statement)
	if statement == "" {
		statement = "자동 정합성 분석 결과"
	}
	return llm.AlignmentFinding{
		Type: "missing", Statement: statement,
		Finding:          "자동 분석 결과가 유효한 원문 위치와 연결되지 않아 해당 판단을 확정할 수 없습니다.",
		ReasoningSummary: "모델 출력이 원문 추적 규약을 충족하지 않아 원래 판단을 문제로 확정하지 않고 검증 불가로 처리했습니다.",
		Impact:           "이 항목은 재검증 전까지 기획의 정합성 판정 근거로 사용할 수 없습니다.", Severity: "medium", Confidence: 0,
		RequiredAction: "원문 위치를 보존해 정합성 검사를 다시 수행합니다.", DocumentAbsence: true,
	}
}

func alignmentMessage(value domain.PurposeAlignmentEvaluation) *evaluationv1.PurposeAlignmentEvaluation {
	statuses := map[string]evaluationv1.ValidationStatus{"satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED, "partially_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_PARTIALLY_SATISFIED, "not_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED}
	types := map[string]evaluationv1.FindingType{"missing": evaluationv1.FindingType_FINDING_TYPE_MISSING, "purposeless_feature": evaluationv1.FindingType_FINDING_TYPE_PURPOSELESS_FEATURE, "logical_gap": evaluationv1.FindingType_FINDING_TYPE_LOGICAL_GAP, "unmeasurable_goal": evaluationv1.FindingType_FINDING_TYPE_UNMEASURABLE_GOAL}
	severities := map[string]evaluationv1.Severity{"low": evaluationv1.Severity_SEVERITY_LOW, "medium": evaluationv1.Severity_SEVERITY_MEDIUM, "high": evaluationv1.Severity_SEVERITY_HIGH, "blocking": evaluationv1.Severity_SEVERITY_BLOCKING}
	result := &evaluationv1.PurposeAlignmentEvaluation{Id: value.ID, DocumentId: value.DocumentID, DocumentVersion: value.DocumentVersion, Status: statuses[value.Status], Model: value.Model, CreatedAt: timestamppb.New(value.CreatedAt)}
	for _, item := range value.Findings {
		finding := &evaluationv1.Finding{Id: item.ID, Area: evaluationv1.ValidationArea_VALIDATION_AREA_PURPOSE, Type: types[item.Type], Statement: item.Statement, Finding: item.Finding, ReasoningSummary: item.ReasoningSummary, Impact: item.Impact, Severity: severities[item.Severity], Confidence: item.Confidence, RequiredAction: item.RequiredAction, SourceBlockOrdinals: append([]uint32(nil), item.SourceBlockOrdinals...), DocumentAbsence: item.DocumentAbsence}
		for _, source := range item.SourceLocations {
			finding.SourceLocations = append(finding.SourceLocations, protoLocation(source))
		}
		result.Findings = append(result.Findings, finding)
	}
	return result
}

func validationAreaNames() map[evaluationv1.ValidationArea]string {
	return map[evaluationv1.ValidationArea]string{evaluationv1.ValidationArea_VALIDATION_AREA_PURPOSE: "purpose", evaluationv1.ValidationArea_VALIDATION_AREA_PROBLEM: "problem", evaluationv1.ValidationArea_VALIDATION_AREA_CONTEXT: "context", evaluationv1.ValidationArea_VALIDATION_AREA_EVIDENCE: "evidence", evaluationv1.ValidationArea_VALIDATION_AREA_DATA: "data", evaluationv1.ValidationArea_VALIDATION_AREA_API: "api", evaluationv1.ValidationArea_VALIDATION_AREA_TECHNOLOGY: "technology", evaluationv1.ValidationArea_VALIDATION_AREA_RESOURCE: "resource", evaluationv1.ValidationArea_VALIDATION_AREA_CONSISTENCY: "consistency", evaluationv1.ValidationArea_VALIDATION_AREA_COMPLETENESS: "completeness", evaluationv1.ValidationArea_VALIDATION_AREA_VERIFIABILITY: "verifiability", evaluationv1.ValidationArea_VALIDATION_AREA_OPERATIONS: "operations"}
}

func resultSummaryMessage(value domain.ValidationResultSummary) *evaluationv1.ValidationResultSummary {
	result := &evaluationv1.ValidationResultSummary{ValidationPlanId: value.ValidationPlanID, DefectCount: value.DefectCount, ScoredCount: value.ScoredCount, ExcludedCount: value.ExcludedCount, SatisfactionRate: value.SatisfactionRate, CreatedAt: timestamppb.New(value.CreatedAt)}
	areaValues := map[string]evaluationv1.ValidationArea{}
	for key, name := range validationAreaNames() {
		areaValues[name] = key
	}
	statusValues := map[string]evaluationv1.ValidationStatus{"satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED, "partially_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_PARTIALLY_SATISFIED, "not_satisfied": evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED, "unverified": evaluationv1.ValidationStatus_VALIDATION_STATUS_UNVERIFIED, "not_applicable": evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_APPLICABLE, "skipped": evaluationv1.ValidationStatus_VALIDATION_STATUS_SKIPPED}
	for _, item := range value.Results {
		result.Results = append(result.Results, &evaluationv1.ValidationResult{Area: areaValues[item.Area], Status: statusValues[item.Status], Reason: item.Reason, CountedAsDefect: item.CountedAsDefect, IncludedInScore: item.Scored})
	}
	return result
}

func knownContext(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown (validate before context-dependent conclusions)"
	}
	return strings.TrimSpace(value)
}

func normalizePlanDrivers(items []llm.PlanItem, drivers map[string]struct{}) []llm.PlanItem {
	result := append([]llm.PlanItem(nil), items...)
	for index := range result {
		seen := map[string]struct{}{}
		valid := make([]string, 0, len(result[index].DriverIDs))
		removed := false
		for _, id := range result[index].DriverIDs {
			if _, exists := drivers[id]; !exists {
				removed = true
				continue
			}
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			valid = append(valid, id)
		}
		result[index].DriverIDs = valid
		if removed {
			result[index].Reason = strings.TrimSpace(result[index].Reason + " 모델이 생성한 미등록 판단 요소 참조는 제외했습니다.")
		}
	}
	return result
}

func applyRequiredPlanPolicy(items []llm.PlanItem, requiredDrivers, requiredAreas map[string]struct{}) []llm.PlanItem {
	result := append([]llm.PlanItem(nil), items...)
	used := map[string]struct{}{}
	verifiability := -1
	for index := range result {
		item := &result[index]
		if _, required := requiredAreas[item.Area]; required && !item.Selected {
			item.Selected = true
			item.Depth = "standard"
			item.Reason = strings.TrimSpace(item.Reason + " 필수 의존성 또는 실행 맥락이 있어 정책상 검증합니다.")
		}
		if item.Area == "verifiability" {
			verifiability = index
		}
		for _, id := range item.DriverIDs {
			used[id] = struct{}{}
		}
	}
	if verifiability >= 0 {
		item := &result[verifiability]
		for id := range requiredDrivers {
			if _, exists := used[id]; exists {
				continue
			}
			item.DriverIDs = append(item.DriverIDs, id)
		}
		if len(item.DriverIDs) > 0 {
			item.Selected = true
			item.Depth = "deep"
			item.Reason = strings.TrimSpace(item.Reason + " 핵심 전제와 고위험 요소는 정책상 검증 가능성을 심층 확인합니다.")
		}
	}
	return result
}

func validatePlan(items []llm.PlanItem, drivers, requiredDrivers, requiredAreas map[string]struct{}) error {
	if len(items) != len(validationAreas) {
		return errors.New("검증 계획이 모든 평가 영역을 포함하지 않습니다.")
	}
	expected := map[string]struct{}{}
	usedDrivers := map[string]struct{}{}
	for _, area := range validationAreas {
		expected[area] = struct{}{}
	}
	for _, item := range items {
		if _, ok := expected[item.Area]; !ok {
			return errors.New("검증 계획에 알 수 없거나 중복된 영역이 있습니다.")
		}
		delete(expected, item.Area)
		if strings.TrimSpace(item.Reason) == "" {
			return errors.New("검증 계획의 선택 근거가 없습니다.")
		}
		if item.Selected {
			if item.Depth != "basic" && item.Depth != "standard" && item.Depth != "deep" {
				return errors.New("선택된 검증의 깊이가 유효하지 않습니다.")
			}
		} else if item.Depth != "none" {
			return errors.New("미선택 검증의 깊이는 none이어야 합니다.")
		}
		if _, required := requiredAreas[item.Area]; required && !item.Selected {
			return errors.New("필수 의존성 또는 맥락 검증이 선택되지 않았습니다.")
		}
		for _, id := range item.DriverIDs {
			if _, ok := drivers[id]; !ok {
				return errors.New("검증 계획이 존재하지 않는 판단 요소를 참조합니다.")
			}
			usedDrivers[id] = struct{}{}
		}
	}
	for id := range requiredDrivers {
		if _, ok := usedDrivers[id]; !ok {
			return errors.New("핵심 전제 또는 고위험 요소가 검증 계획에 반영되지 않았습니다.")
		}
	}
	return nil
}
func score(value float64) bool { return value >= 0 && value <= 1 }
func validText(statement, impact string, confidence float64, ordinal uint32, locations map[uint32]*documentv1.SourceLocation) bool {
	_, ok := locations[ordinal]
	return strings.TrimSpace(statement) != "" && strings.TrimSpace(impact) != "" && score(confidence) && ok
}
func identifier(documentID string, version uint32, kind string, index int, value string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s:%d:%s", documentID, version, kind, index, value)))
	return hex.EncodeToString(sum[:8])
}
func location(value *documentv1.SourceLocation) domain.Location {
	if value == nil {
		return domain.Location{}
	}
	return domain.Location{PageNumber: value.GetPageNumber(), SlideNumber: value.GetSlideNumber(), SheetName: value.GetSheetName(), SectionPath: append([]string(nil), value.GetSectionPath()...), ParagraphNumber: value.GetParagraphNumber()}
}
func protoLocation(value domain.Location) *documentv1.SourceLocation {
	return &documentv1.SourceLocation{PageNumber: value.PageNumber, SlideNumber: value.SlideNumber, SheetName: value.SheetName, SectionPath: append([]string(nil), value.SectionPath...), ParagraphNumber: value.ParagraphNumber}
}
func message(value domain.Analysis) *evaluationv1.AssumptionRiskAnalysis {
	result := &evaluationv1.AssumptionRiskAnalysis{Id: value.ID, DocumentId: value.DocumentID, DocumentVersion: value.DocumentVersion, Model: value.Model, CreatedAt: timestamppb.New(value.CreatedAt)}
	for _, item := range value.Assumptions {
		criticality := evaluationv1.AssumptionCriticality_ASSUMPTION_CRITICALITY_GENERAL
		if item.Criticality == "critical" {
			criticality = evaluationv1.AssumptionCriticality_ASSUMPTION_CRITICALITY_CRITICAL
		}
		result.Assumptions = append(result.Assumptions, &evaluationv1.Assumption{Id: item.ID, Statement: item.Statement, Criticality: criticality, CriticalityReason: item.CriticalityReason, FailureImpact: item.FailureImpact, Confidence: item.Confidence, SourceBlockOrdinal: item.SourceBlockOrdinal, SourceLocation: protoLocation(item.Location)})
	}
	for _, item := range value.Risks {
		result.Risks = append(result.Risks, &evaluationv1.Risk{Id: item.ID, Statement: item.Statement, FailureImpact: item.FailureImpact, Likelihood: item.Likelihood, Impact: item.Impact, Reversibility: item.Reversibility, Confidence: item.Confidence, AssumptionIds: append([]string(nil), item.AssumptionIDs...), SourceBlockOrdinal: item.SourceBlockOrdinal, SourceLocation: protoLocation(item.Location)})
	}
	return result
}

func planMessage(value domain.ValidationPlan) *evaluationv1.ValidationPlan {
	result := &evaluationv1.ValidationPlan{Id: value.ID, DocumentId: value.DocumentID, DocumentVersion: value.DocumentVersion, Purpose: value.Purpose, Country: value.Country, Domain: value.Domain, Model: value.Model, CreatedAt: timestamppb.New(value.CreatedAt)}
	areas := map[string]evaluationv1.ValidationArea{"purpose": evaluationv1.ValidationArea_VALIDATION_AREA_PURPOSE, "problem": evaluationv1.ValidationArea_VALIDATION_AREA_PROBLEM, "context": evaluationv1.ValidationArea_VALIDATION_AREA_CONTEXT, "evidence": evaluationv1.ValidationArea_VALIDATION_AREA_EVIDENCE, "data": evaluationv1.ValidationArea_VALIDATION_AREA_DATA, "api": evaluationv1.ValidationArea_VALIDATION_AREA_API, "technology": evaluationv1.ValidationArea_VALIDATION_AREA_TECHNOLOGY, "resource": evaluationv1.ValidationArea_VALIDATION_AREA_RESOURCE, "consistency": evaluationv1.ValidationArea_VALIDATION_AREA_CONSISTENCY, "completeness": evaluationv1.ValidationArea_VALIDATION_AREA_COMPLETENESS, "verifiability": evaluationv1.ValidationArea_VALIDATION_AREA_VERIFIABILITY, "operations": evaluationv1.ValidationArea_VALIDATION_AREA_OPERATIONS}
	depths := map[string]evaluationv1.ValidationDepth{"none": evaluationv1.ValidationDepth_VALIDATION_DEPTH_NONE, "basic": evaluationv1.ValidationDepth_VALIDATION_DEPTH_BASIC, "standard": evaluationv1.ValidationDepth_VALIDATION_DEPTH_STANDARD, "deep": evaluationv1.ValidationDepth_VALIDATION_DEPTH_DEEP}
	for _, item := range value.Items {
		result.Items = append(result.Items, &evaluationv1.ValidationPlanItem{Area: areas[item.Area], Selected: item.Selected, Depth: depths[item.Depth], Reason: item.Reason, DriverIds: append([]string(nil), item.DriverIDs...)})
	}
	return result
}
