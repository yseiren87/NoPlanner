package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	planning "noplanner/backend/generation/domains/planning"
	"noplanner/backend/generation/modules/rpcerror"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

func (s *Service) CreateImprovementPlan(ctx context.Context, req *generationv1.CreateImprovementPlanRequest) (*generationv1.ImprovementPlan, error) {
	findings := make([]planning.Finding, 0, len(req.GetFindings()))
	for _, finding := range req.GetFindings() {
		detail := finding.GetDetail()
		action := strings.TrimSpace(detail.GetRequiredAction())
		if action == "" {
			action = "Finding " + detail.GetId() + "의 원인을 해소하고 기획서에 반영합니다."
		}
		verification := strings.TrimSpace(detail.GetVerificationMethod())
		if verification == "" {
			verification = "수정된 문서를 동일 평가 기준으로 다시 검증합니다."
		}
		findings = append(findings, planning.Finding{ID: detail.GetId(), Severity: severityName(finding.GetSeverity()), RequiredAction: action, VerificationMethod: verification})
	}
	result, err := planning.BuildImprovementPlan(req.GetEvaluationId(), findings)
	if err != nil {
		return nil, invalidPlanning(ctx)
	}
	return improvementPlanProto(result), nil
}

func (s *Service) ImproveExistingPlan(ctx context.Context, req *generationv1.ImproveExistingPlanRequest) (*generationv1.ImprovedPlan, error) {
	result, err := planning.Improve(req.GetOriginalIntent(), sectionsDomain(req.GetOriginalSections()), improvementPlanDomain(req.GetImprovementPlan()), claimsDomain(req.GetClaims()))
	if err != nil {
		return nil, invalidPlanning(ctx)
	}
	out := &generationv1.ImprovedPlan{Id: result.ID, PreservedIntent: result.PreservedIntent, UnresolvedFindingIds: result.UnresolvedFindingIDs}
	for _, section := range result.Sections {
		out.Sections = append(out.Sections, sectionProto(section))
	}
	for _, change := range result.Changes {
		out.Changes = append(out.Changes, &generationv1.PlanChange{SectionKey: change.SectionKey, FindingIds: change.FindingIDs, Before: change.Before, After: change.After, Reason: change.Reason})
	}
	return out, nil
}

func (s *Service) DiscoverPlanningQuestions(ctx context.Context, req *generationv1.DiscoverPlanningQuestionsRequest) (*generationv1.PlanningQuestionSet, error) {
	questions, err := planning.MissingQuestions(req.GetIdea(), contextDomain(req.GetKnownContext()))
	if err != nil {
		return nil, invalidPlanning(ctx)
	}
	out := &generationv1.PlanningQuestionSet{Idea: req.GetIdea(), ReadyToPlan: len(questions) == 0}
	for _, q := range questions {
		out.Questions = append(out.Questions, &generationv1.PlanningQuestion{Id: q.ID, Field: q.Field, Question: q.Question, Reason: q.Reason})
	}
	return out, nil
}

func (s *Service) GenerateAutonomousPlan(ctx context.Context, req *generationv1.GenerateAutonomousPlanRequest) (*generationv1.AutonomousPlan, error) {
	providerOutputInvalid := false
	var aiSpec struct {
		Problem         string   `json:"problem"`
		Purpose         string   `json:"purpose"`
		TargetUser      string   `json:"target_user"`
		Scope           string   `json:"scope"`
		OutOfScope      string   `json:"out_of_scope"`
		UserFlow        []string `json:"user_flow"`
		Policies        []string `json:"policies"`
		SuccessCriteria []string `json:"success_criteria"`
		Assumptions     []string `json:"assumptions"`
	}
	if s.intelligence != nil {
		prompt := fmt.Sprintf("다음 아이디어를 실행 가능한 제품 기획으로 작성하라. 근거 없는 사실은 만들지 말고 JSON만 반환하라. 필드: problem,purpose,target_user,scope,out_of_scope,user_flow,policies,success_criteria,assumptions. idea=%q context=%q", req.GetIdea(), req.GetContext().String())
		var decodeErr error
		for attempt := 0; attempt < 2; attempt++ {
			requestPrompt := prompt
			if attempt > 0 {
				requestPrompt += " 이전 응답은 JSON 파싱에 실패했다. 마크다운과 설명 없이 하나의 완전한 JSON 객체만 반환하라."
			}
			generated, err := s.intelligence.Generate(ctx, &intelligencev1.GenerateRequest{Prompt: requestPrompt, JsonResponse: true, MaxTokens: 8192})
			if err != nil {
				return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_DEPENDENCY_UNAVAILABLE, "자율 기획 LLM 호출에 실패했습니다.", true)
			}
			decodeErr = decodeGeneratedJSON(generated.Text, &aiSpec)
			if decodeErr == nil {
				break
			}
		}
		if decodeErr != nil {
			providerOutputInvalid = true
		}
		if req.Context == nil {
			req.Context = &generationv1.PlanningContext{}
		}
		if req.Context.Purpose == "" {
			req.Context.Purpose = aiSpec.Purpose
		}
		if req.Context.TargetUser == "" {
			req.Context.TargetUser = aiSpec.TargetUser
		}
	}
	research := make([]planning.ResearchFact, 0, len(req.GetResearch()))
	for _, r := range req.GetResearch() {
		ids := []string{}
		for _, e := range r.GetEvidence() {
			ids = append(ids, e.GetId())
		}
		research = append(research, planning.ResearchFact{ID: r.GetId(), Question: r.GetQuestion(), Conclusion: r.GetConclusion(), EvidenceIDs: ids, Verified: r.GetVerified(), Uncertainties: r.GetUncertainties()})
	}
	alternatives := make([]planning.Alternative, 0, len(req.GetAlternatives()))
	for _, a := range req.GetAlternatives() {
		alternatives = append(alternatives, planning.Alternative{ID: a.GetId(), Name: a.GetName(), Description: a.GetDescription(), EvidenceIDs: a.GetEvidenceIds(), Advantages: a.GetAdvantages(), Disadvantages: a.GetDisadvantages(), Selected: a.GetSelected(), DecisionReason: a.GetDecisionReason()})
	}
	result, err := planning.Generate(req.GetIdea(), contextDomain(req.GetContext()), research, alternatives, req.GetCriteriaVersion(), req.GetModel())
	if err != nil {
		return nil, invalidPlanning(ctx)
	}
	result.OwnerSubject = req.GetOwnerSubject()
	if providerOutputInvalid {
		planning.MergeSelfEvaluation(&result, []planning.Finding{{ID: "generation-provider-output-invalid", Severity: "blocking", RequiredAction: "공급자 응답 형식을 교정한 뒤 생성 기획을 다시 작성합니다.", VerificationMethod: "동일 평가 엔진으로 재생성 결과를 검증합니다."}})
	}
	result.CreatedAt = time.Now().UTC()
	if aiSpec.Problem != "" {
		result.Specification.Problem = aiSpec.Problem
	}
	if aiSpec.Scope != "" {
		result.Specification.Scope = aiSpec.Scope
	}
	if aiSpec.OutOfScope != "" {
		result.Specification.OutOfScope = aiSpec.OutOfScope
	}
	if len(aiSpec.UserFlow) > 0 {
		result.Specification.UserFlow = aiSpec.UserFlow
	}
	if len(aiSpec.Policies) > 0 {
		result.Specification.Policies = aiSpec.Policies
	}
	if len(aiSpec.SuccessCriteria) > 0 {
		result.Specification.SuccessCriteria = aiSpec.SuccessCriteria
	}
	if len(aiSpec.Assumptions) > 0 {
		result.Specification.Assumptions = aiSpec.Assumptions
	}
	if err = s.evaluateGeneratedPlan(ctx, &result); err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_DEPENDENCY_UNAVAILABLE, "생성 기획을 동일 평가 엔진으로 검증하지 못했습니다.", true)
	}
	if s.planningStore != nil {
		if _, err = s.planningStore.Save(ctx, result); err != nil {
			return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "자율 기획을 저장하지 못했습니다.", true)
		}
	}
	return autonomousProto(result, req.GetResearch()), nil
}

func decodeGeneratedJSON(value string, target any) error {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(value), "```json"), "```"), "```"))
	start := strings.Index(value, "{")
	if start < 0 {
		return errors.New("JSON object not found")
	}
	depth, inString, escaped := 0, false, false
	for index := start; index < len(value); index++ {
		character := value[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
			continue
		}
		if character == '{' {
			depth++
		}
		if character == '}' {
			depth--
			if depth == 0 {
				return json.Unmarshal([]byte(value[start:index+1]), target)
			}
		}
	}
	return errors.New("complete JSON object not found")
}

func (s *Service) evaluateGeneratedPlan(ctx context.Context, plan *planning.AutonomousPlan) error {
	if s.evaluation == nil {
		return errors.New("evaluation service is not configured")
	}
	location := func(ordinal uint32) *documentv1.SourceLocation {
		return &documentv1.SourceLocation{SectionPath: []string{"generated-plan"}, ParagraphNumber: ordinal}
	}
	contents := []struct {
		kind    documentv1.DocumentElementType
		content string
	}{
		{documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_PROBLEM, plan.Specification.Problem},
		{documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_PURPOSE, plan.Specification.Purpose},
		{documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_TARGET, plan.Specification.TargetUser},
		{documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_SOLUTION, plan.Specification.Scope},
		{documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_GOAL, strings.Join(plan.Specification.SuccessCriteria, "\n")},
	}
	elements := make([]*documentv1.DocumentElement, 0, len(contents))
	for _, item := range contents {
		if strings.TrimSpace(item.content) == "" {
			continue
		}
		ordinal := uint32(len(elements) + 1)
		elements = append(elements, &documentv1.DocumentElement{Id: fmt.Sprintf("generated-%d", ordinal), Type: item.kind, Content: item.content, Confidence: 1, SourceBlockOrdinal: ordinal, SourceLocation: location(ordinal)})
	}
	purpose, err := s.evaluation.EvaluatePurposeAlignment(ctx, &evaluationv1.EvaluatePurposeAlignmentRequest{DocumentId: plan.ID, DocumentVersion: 1, Elements: elements})
	if err != nil {
		return err
	}
	requirements := make([]*documentv1.Requirement, 0, len(plan.Specification.Requirements))
	for index, requirement := range plan.Specification.Requirements {
		ordinal := uint32(len(elements) + index + 1)
		requirements = append(requirements, &documentv1.Requirement{Id: requirement.ID, Type: documentv1.RequirementType_REQUIREMENT_TYPE_REQUIREMENT, Content: requirement.Statement + "\n인수 조건: " + requirement.AcceptanceCriteria, Confidence: 1, SourceBlockOrdinal: ordinal, SourceLocation: location(ordinal)})
	}
	completeness, err := s.evaluation.EvaluateRequirementCompleteness(ctx, &evaluationv1.EvaluateRequirementCompletenessRequest{DocumentId: plan.ID, DocumentVersion: 1, Requirements: requirements})
	if err != nil {
		return err
	}
	findings := make([]planning.Finding, 0, len(purpose.GetFindings())+len(completeness.GetGaps()))
	appendFinding := func(finding *evaluationv1.Finding) {
		if finding == nil {
			return
		}
		findings = append(findings, planning.Finding{ID: finding.GetId(), Severity: strings.ToLower(strings.TrimPrefix(finding.GetSeverity().String(), "SEVERITY_")), RequiredAction: finding.GetRequiredAction(), VerificationMethod: "동일 평가 엔진으로 수정본을 재검증한다."})
	}
	for _, finding := range purpose.GetFindings() {
		appendFinding(finding)
	}
	for _, gap := range completeness.GetGaps() {
		appendFinding(gap.GetFinding())
	}
	planning.MergeSelfEvaluation(plan, findings)
	return nil
}

func (s *Service) GetAutonomousPlan(ctx context.Context, req *generationv1.GetAutonomousPlanRequest) (*generationv1.AutonomousPlan, error) {
	if s.planningStore == nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "자율 기획 저장소가 없습니다.", false)
	}
	value, err := s.planningStore.Get(ctx, req.GetPlanId())
	if errors.Is(err, planning.ErrNotFound) {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_NOT_FOUND, "자율 기획을 찾을 수 없습니다.", false)
	}
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "자율 기획을 불러오지 못했습니다.", true)
	}
	return autonomousProto(value, researchProto(value.Research)), nil
}

func researchProto(values []planning.ResearchFact) []*generationv1.ResearchFact {
	out := make([]*generationv1.ResearchFact, 0, len(values))
	for _, v := range values {
		out = append(out, &generationv1.ResearchFact{Id: v.ID, Question: v.Question, Conclusion: v.Conclusion, Verified: v.Verified, Uncertainties: v.Uncertainties})
	}
	return out
}

func invalidPlanning(ctx context.Context) error {
	return rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "기획 입력과 추적 근거가 올바르지 않습니다.", false)
}
func severityName(value generationv1.Severity) string {
	return map[generationv1.Severity]string{generationv1.Severity_SEVERITY_LOW: "low", generationv1.Severity_SEVERITY_MEDIUM: "medium", generationv1.Severity_SEVERITY_HIGH: "high", generationv1.Severity_SEVERITY_BLOCKING: "blocking"}[value]
}
func contextDomain(v *generationv1.PlanningContext) planning.Context {
	if v == nil {
		return planning.Context{}
	}
	return planning.Context{Purpose: v.GetPurpose(), TargetUser: v.GetTargetUser(), Environment: v.GetEnvironment(), Constraints: v.GetConstraints()}
}
func sectionsDomain(values []*generationv1.PlanSection) []planning.Section {
	out := make([]planning.Section, 0, len(values))
	for _, v := range values {
		out = append(out, planning.Section{Key: v.GetKey(), Title: v.GetTitle(), Content: v.GetContent(), SourceFindingIDs: v.GetSourceFindingIds(), EvidenceIDs: v.GetEvidenceIds()})
	}
	return out
}
func claimsDomain(values []*generationv1.GroundedClaim) []planning.Claim {
	out := make([]planning.Claim, 0, len(values))
	for _, v := range values {
		out = append(out, planning.Claim{Statement: v.GetStatement(), EvidenceIDs: v.GetEvidenceIds(), FindingIDs: v.GetFindingIds(), Verified: v.GetVerified()})
	}
	return out
}
func improvementPlanDomain(v *generationv1.ImprovementPlan) planning.ImprovementPlan {
	if v == nil {
		return planning.ImprovementPlan{}
	}
	out := planning.ImprovementPlan{ID: v.GetId(), EvaluationID: v.GetEvaluationId()}
	for _, t := range v.GetTasks() {
		out.Tasks = append(out.Tasks, planning.ImprovementTask{ID: t.GetId(), FindingIDs: t.GetFindingIds(), Order: t.GetOrder(), Action: t.GetAction(), RequiredResearch: t.GetRequiredResearch(), VerificationMethod: t.GetVerificationMethod(), Blocking: t.GetBlocking()})
	}
	return out
}
func improvementPlanProto(v planning.ImprovementPlan) *generationv1.ImprovementPlan {
	out := &generationv1.ImprovementPlan{Id: v.ID, EvaluationId: v.EvaluationID}
	for _, t := range v.Tasks {
		out.Tasks = append(out.Tasks, &generationv1.ImprovementTask{Id: t.ID, FindingIds: t.FindingIDs, Order: t.Order, Action: t.Action, RequiredResearch: t.RequiredResearch, VerificationMethod: t.VerificationMethod, Blocking: t.Blocking})
	}
	return out
}
func sectionProto(v planning.Section) *generationv1.PlanSection {
	return &generationv1.PlanSection{Key: v.Key, Title: v.Title, Content: v.Content, SourceFindingIds: v.SourceFindingIDs, EvidenceIds: v.EvidenceIDs}
}

func autonomousProto(v planning.AutonomousPlan, evidenceSource []*generationv1.ResearchFact) *generationv1.AutonomousPlan {
	ctx := &generationv1.PlanningContext{Purpose: v.Context.Purpose, TargetUser: v.Context.TargetUser, Environment: v.Context.Environment, Constraints: v.Context.Constraints}
	out := &generationv1.AutonomousPlan{Id: v.ID, Idea: v.Idea, Context: ctx, TraceIds: v.TraceIDs, CreatedAt: timestamppb.New(v.CreatedAt), OwnerSubject: v.OwnerSubject}
	out.Research = evidenceSource
	for _, a := range v.Alternatives {
		out.Alternatives = append(out.Alternatives, &generationv1.SolutionAlternative{Id: a.ID, Name: a.Name, Description: a.Description, EvidenceIds: a.EvidenceIDs, Advantages: a.Advantages, Disadvantages: a.Disadvantages, Selected: a.Selected, DecisionReason: a.DecisionReason})
	}
	spec := v.Specification
	out.Specification = &generationv1.GeneratedSpecification{Problem: spec.Problem, Purpose: spec.Purpose, TargetUser: spec.TargetUser, Scope: spec.Scope, OutOfScope: spec.OutOfScope, UserFlow: spec.UserFlow, Policies: spec.Policies, SuccessCriteria: spec.SuccessCriteria, Assumptions: spec.Assumptions}
	for _, r := range spec.Requirements {
		out.Specification.Requirements = append(out.Specification.Requirements, &generationv1.ExecutionRequirement{Id: r.ID, Statement: r.Statement, AcceptanceCriteria: r.AcceptanceCriteria, EvidenceIds: r.EvidenceIDs})
	}
	evaluation := &generationv1.SelfEvaluation{Verdict: verdictProto(v.SelfEvaluation.Verdict), UnresolvedBlockers: v.SelfEvaluation.UnresolvedBlockers}
	for _, f := range v.SelfEvaluation.Findings {
		evaluation.Findings = append(evaluation.Findings, &generationv1.Finding{Detail: &generationv1.FindingInput{Id: f.ID, RequiredAction: f.RequiredAction, VerificationMethod: f.VerificationMethod}, Severity: severityProto(f.Severity), Confidence: confidenceProto(v.SelfEvaluation.Confidence)})
	}
	out.SelfEvaluation = evaluation
	return out
}
