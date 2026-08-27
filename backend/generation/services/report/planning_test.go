package report

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	planning "noplanner/backend/generation/domains/planning"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

type generatedPlanEvaluationStub struct {
	evaluationv1.EvaluationServiceClient
	purposeFindings []*evaluationv1.Finding
}

type planningIntelligenceStub struct {
	intelligencev1.IntelligenceServiceClient
	calls int
}

func (stub *planningIntelligenceStub) Generate(context.Context, *intelligencev1.GenerateRequest, ...grpc.CallOption) (*intelligencev1.GenerateResponse, error) {
	stub.calls++
	if stub.calls == 1 {
		return &intelligencev1.GenerateResponse{Text: "not-json"}, nil
	}
	return &intelligencev1.GenerateResponse{Text: `{"problem":"문제","purpose":"목적","target_user":"개발자","scope":"범위","out_of_scope":"제외","user_flow":["사용"],"policies":["정책"],"success_criteria":["성공"],"assumptions":[]}`}, nil
}

func (stub generatedPlanEvaluationStub) EvaluatePurposeAlignment(context.Context, *evaluationv1.EvaluatePurposeAlignmentRequest, ...grpc.CallOption) (*evaluationv1.PurposeAlignmentEvaluation, error) {
	return &evaluationv1.PurposeAlignmentEvaluation{Findings: stub.purposeFindings}, nil
}

func TestAutonomousPlanMergesFindingsFromEvaluationService(t *testing.T) {
	svc := New(nil, "generation", "test")
	svc.evaluation = generatedPlanEvaluationStub{purposeFindings: []*evaluationv1.Finding{{Id: "evaluation-finding", Severity: evaluationv1.Severity_SEVERITY_BLOCKING, RequiredAction: "목적과 기능 연결을 수정한다."}}}
	result, err := svc.GenerateAutonomousPlan(context.Background(), &generationv1.GenerateAutonomousPlanRequest{Idea: "검증 가능한 서비스", Context: &generationv1.PlanningContext{Purpose: "재작업 감소", TargetUser: "개발자", Environment: "한국 기업", Constraints: []string{"공식 근거"}}, Research: []*generationv1.ResearchFact{{Id: "R-1", Verified: true, Evidence: []*generationv1.EvidenceLink{{Id: "E-1"}}}}, Alternatives: []*generationv1.SolutionAlternative{{Id: "A-1", Name: "자동 검증", Description: "검증", Selected: true, DecisionReason: "직접 해결"}}, CriteriaVersion: "criteria-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetSelfEvaluation().GetVerdict() != generationv1.Verdict_VERDICT_NOT_EXECUTABLE || len(result.GetSelfEvaluation().GetFindings()) != 1 || result.GetSelfEvaluation().GetFindings()[0].GetDetail().GetId() != "evaluation-finding" {
		t.Fatalf("self evaluation = %#v", result.GetSelfEvaluation())
	}
}

func (generatedPlanEvaluationStub) EvaluateRequirementCompleteness(context.Context, *evaluationv1.EvaluateRequirementCompletenessRequest, ...grpc.CallOption) (*evaluationv1.RequirementCompletenessEvaluation, error) {
	return &evaluationv1.RequirementCompletenessEvaluation{}, nil
}

func TestAutonomousPlanningE2E(t *testing.T) {
	svc := New(nil, "generation", "test")
	svc.planningStore = planning.NewMemoryStore()
	svc.evaluation = generatedPlanEvaluationStub{}
	questions, err := svc.DiscoverPlanningQuestions(context.Background(), &generationv1.DiscoverPlanningQuestionsRequest{Idea: "검증 가능한 서비스", KnownContext: &generationv1.PlanningContext{Purpose: "재작업 감소", TargetUser: "개발자", Environment: "한국 기업", Constraints: []string{"공식 근거"}}})
	if err != nil || !questions.GetReadyToPlan() {
		t.Fatalf("question discovery failed: %#v %v", questions, err)
	}
	result, err := svc.GenerateAutonomousPlan(context.Background(), &generationv1.GenerateAutonomousPlanRequest{Idea: questions.GetIdea(), Context: &generationv1.PlanningContext{Purpose: "재작업 감소", TargetUser: "개발자", Environment: "한국 기업", Constraints: []string{"공식 근거"}}, Research: []*generationv1.ResearchFact{{Id: "R-1", Question: "문제가 실재하는가", Conclusion: "공식 통계로 확인", Verified: true, Evidence: []*generationv1.EvidenceLink{{Id: "E-1", Url: "https://example.test/source"}}}}, Alternatives: []*generationv1.SolutionAlternative{{Id: "A-1", Name: "자동 검증", Description: "핵심 검증 범위", EvidenceIds: []string{"E-1"}, Selected: true, DecisionReason: "재작업을 가장 직접 줄임"}, {Id: "A-2", Name: "수동 체크리스트", Description: "수동 검토", Selected: false, DecisionReason: "검토 품질이 사람에 의존"}}, CriteriaVersion: "criteria-1", Model: "test-model"})
	if err != nil || result.GetSelfEvaluation().GetVerdict() != generationv1.Verdict_VERDICT_EXECUTABLE || len(result.GetTraceIds()) != 3 || len(result.GetSpecification().GetRequirements()) == 0 {
		t.Fatalf("autonomous planning trace failed: %#v %v", result, err)
	}
	loaded, err := svc.GetAutonomousPlan(context.Background(), &generationv1.GetAutonomousPlanRequest{PlanId: result.GetId()})
	if err != nil || loaded.GetId() != result.GetId() {
		t.Fatalf("stored plan unavailable: %#v %v", loaded, err)
	}
}

func TestDecodeGeneratedJSONAcceptsCodeFenceAndSnakeCase(t *testing.T) {
	var value struct {
		TargetUser string `json:"target_user"`
	}
	if err := decodeGeneratedJSON("```json\n{\"target_user\":\"개발자\"}\n```\n설명 {\"ignored\":true}", &value); err != nil || value.TargetUser != "개발자" {
		t.Fatalf("decoded = %#v, %v", value, err)
	}
}

func TestAutonomousPlanningRetriesInvalidProviderJSONOnce(t *testing.T) {
	provider := &planningIntelligenceStub{}
	svc := New(nil, "generation", "test")
	svc.intelligence = provider
	svc.evaluation = generatedPlanEvaluationStub{}
	result, err := svc.GenerateAutonomousPlan(context.Background(), &generationv1.GenerateAutonomousPlanRequest{Idea: "도구", Context: &generationv1.PlanningContext{Purpose: "목적", TargetUser: "개발자", Environment: "한국", Constraints: []string{"제약"}}, Research: []*generationv1.ResearchFact{{Id: "R", Verified: true, Evidence: []*generationv1.EvidenceLink{{Id: "E"}}}}, Alternatives: []*generationv1.SolutionAlternative{{Id: "A", Name: "안", Description: "범위", Selected: true, DecisionReason: "이유"}}, CriteriaVersion: "1"})
	if err != nil || result.GetSpecification().GetProblem() != "문제" || provider.calls != 2 {
		t.Fatalf("result=%#v calls=%d err=%v", result, provider.calls, err)
	}
}

func TestImprovementE2E(t *testing.T) {
	svc := New(nil, "generation", "test")
	ctx := context.Background()
	plan, err := svc.CreateImprovementPlan(ctx, &generationv1.CreateImprovementPlanRequest{EvaluationId: "EV-1", Findings: []*generationv1.Finding{{Detail: &generationv1.FindingInput{Id: "F-1", RequiredAction: "공식 데이터 조사", VerificationMethod: "출처 원문 확인"}, Severity: generationv1.Severity_SEVERITY_BLOCKING}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ImproveExistingPlan(ctx, &generationv1.ImproveExistingPlanRequest{OriginalIntent: "목적 보존", OriginalSections: []*generationv1.PlanSection{{Key: "purpose", Title: "목적", Content: "원문"}}, ImprovementPlan: plan, Claims: []*generationv1.GroundedClaim{{Statement: "확인된 사실", EvidenceIds: []string{"E-1"}, FindingIds: []string{"F-1"}, Verified: true}}})
	if err != nil || result.GetPreservedIntent() != "목적 보존" || len(result.GetChanges()) != 1 || result.GetChanges()[0].GetFindingIds()[0] != "F-1" {
		t.Fatalf("improvement trace failed: %#v %v", result, err)
	}
}

func TestImprovementPlanSuppliesObjectiveVerificationFallback(t *testing.T) {
	svc := New(nil, "generation", "test")
	plan, err := svc.CreateImprovementPlan(context.Background(), &generationv1.CreateImprovementPlanRequest{EvaluationId: "EV-1", Findings: []*generationv1.Finding{{Detail: &generationv1.FindingInput{Id: "F-1", RequiredAction: "목적을 명시한다"}, Severity: generationv1.Severity_SEVERITY_HIGH}}})
	if err != nil || len(plan.GetTasks()) != 1 || plan.GetTasks()[0].GetVerificationMethod() == "" {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
}
