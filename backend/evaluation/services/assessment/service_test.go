package assessment

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	domain "noplanner/backend/evaluation/domains/assessment"
	"noplanner/backend/evaluation/modules/llm"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
)

type memoryStore struct{ value domain.Analysis }

func (s *memoryStore) Create(_ context.Context, value domain.Analysis) (domain.Analysis, error) {
	value.ID = "analysis-1"
	value.CreatedAt = time.Unix(1, 0)
	s.value = value
	return value, nil
}

func (s *memoryStore) CreateValidationPlan(_ context.Context, value domain.ValidationPlan) (domain.ValidationPlan, error) {
	value.ID = "plan-1"
	value.CreatedAt = time.Unix(1, 0)
	return value, nil
}

func (s *memoryStore) CreateValidationResultSummary(_ context.Context, value domain.ValidationResultSummary) (domain.ValidationResultSummary, error) {
	value.CreatedAt = time.Unix(1, 0)
	return value, nil
}

func (s *memoryStore) CreatePurposeAlignmentEvaluation(_ context.Context, value domain.PurposeAlignmentEvaluation) (domain.PurposeAlignmentEvaluation, error) {
	value.ID = "alignment-1"
	value.CreatedAt = time.Unix(1, 0)
	return value, nil
}

func (s *memoryStore) CreateContradictionEvaluation(_ context.Context, value domain.ContradictionEvaluation) (domain.ContradictionEvaluation, error) {
	value.ID = "contradiction-1"
	value.CreatedAt = time.Unix(1, 0)
	return value, nil
}

func (s *memoryStore) CreateRequirementCompletenessEvaluation(_ context.Context, value domain.RequirementCompletenessEvaluation) (domain.RequirementCompletenessEvaluation, error) {
	value.ID = "completeness-1"
	value.CreatedAt = time.Unix(1, 0)
	return value, nil
}

func (s *memoryStore) CreateDocumentWorkQualityEvaluation(_ context.Context, value domain.DocumentWorkQualityEvaluation) (domain.DocumentWorkQualityEvaluation, error) {
	value.ID = "quality-1"
	value.CreatedAt = time.Unix(1, 0)
	return value, nil
}

type analyzer struct {
	result         llm.Result
	plan           llm.PlanResult
	alignment      llm.AlignmentResult
	contradictions llm.ContradictionResult
	completeness   llm.CompletenessResult
	quality        llm.WorkQualityResult
	objection      llm.ObjectionEvidenceResult
}

func (a analyzer) Model() string                                        { return "test-model" }
func (a analyzer) Analyze(context.Context, string) (llm.Result, error)  { return a.result, nil }
func (a analyzer) Plan(context.Context, string) (llm.PlanResult, error) { return a.plan, nil }
func (a analyzer) EvaluatePurposeAlignment(context.Context, string) (llm.AlignmentResult, error) {
	return a.alignment, nil
}
func (a analyzer) DetectContradictions(context.Context, string) (llm.ContradictionResult, error) {
	return a.contradictions, nil
}
func (a analyzer) EvaluateRequirementCompleteness(context.Context, string) (llm.CompletenessResult, error) {
	return a.completeness, nil
}
func (a analyzer) EvaluateDocumentWorkQuality(context.Context, string) (llm.WorkQualityResult, error) {
	return a.quality, nil
}
func (a analyzer) ReviewObjectionEvidence(context.Context, string) (llm.ObjectionEvidenceResult, error) {
	return a.objection, nil
}

func TestIdentifyAssumptionsAndRisksDistinguishesCriticalityAndLinksEvidence(t *testing.T) {
	store := &memoryStore{}
	service := New(store, analyzer{result: llm.Result{Assumptions: []llm.Assumption{{Key: "a1", Statement: "Domestic data exists", Criticality: "critical", CriticalityReason: "Without it the product cannot operate", FailureImpact: "Implementation is blocked", Confidence: .8, SourceBlockOrdinal: 3}}, Risks: []llm.Risk{{Statement: "Required data may be unavailable", FailureImpact: "Core feature fails", Likelihood: .7, Impact: 1, Reversibility: .2, Confidence: .75, AssumptionKeys: []string{"a1"}, SourceBlockOrdinal: 3}}}}, "evaluation", "0.1.0")
	response, err := service.IdentifyAssumptionsAndRisks(context.Background(), &evaluationv1.IdentifyAssumptionsAndRisksRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_ASSUMPTION, Content: "Domestic data exists", SourceBlockOrdinal: 3, SourceLocation: &documentv1.SourceLocation{PageNumber: 2}}}})
	if err != nil || len(response.GetAssumptions()) != 1 || response.GetAssumptions()[0].GetCriticality() != evaluationv1.AssumptionCriticality_ASSUMPTION_CRITICALITY_CRITICAL || response.GetAssumptions()[0].GetSourceLocation().GetPageNumber() != 2 {
		t.Fatalf("assumption response = %#v, %v", response, err)
	}
	if len(response.GetRisks()) != 1 || response.GetRisks()[0].GetAssumptionIds()[0] != response.GetAssumptions()[0].GetId() || store.value.Model != "test-model" {
		t.Fatalf("risk response = %#v", response)
	}
}

func TestIdentifyAssumptionsAndRisksRejectsInventedEvidenceReference(t *testing.T) {
	service := New(&memoryStore{}, analyzer{result: llm.Result{Assumptions: []llm.Assumption{{Key: "a1", Statement: "Unknown", Criticality: "general", CriticalityReason: "May affect scope", FailureImpact: "Rework", Confidence: .5, SourceBlockOrdinal: 99}}}}, "evaluation", "0.1.0")
	_, err := service.IdentifyAssumptionsAndRisks(context.Background(), &evaluationv1.IdentifyAssumptionsAndRisksRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Content: "Known", SourceBlockOrdinal: 1}}})
	if status.Code(err) != codes.Internal {
		t.Fatalf("error = %v", err)
	}
}

func TestIdentifyAssumptionsAndRisksRejectsUnknownAssumptionLink(t *testing.T) {
	service := New(&memoryStore{}, analyzer{result: llm.Result{Risks: []llm.Risk{{Statement: "Risk", FailureImpact: "Failure", Likelihood: .5, Impact: .5, Reversibility: .5, Confidence: .5, AssumptionKeys: []string{"missing"}, SourceBlockOrdinal: 1}}}}, "evaluation", "0.1.0")
	_, err := service.IdentifyAssumptionsAndRisks(context.Background(), &evaluationv1.IdentifyAssumptionsAndRisksRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Content: "Known", SourceBlockOrdinal: 1}}})
	if status.Code(err) != codes.Internal {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateValidationPlanStoresSelectedAndUnselectedReasons(t *testing.T) {
	items := make([]llm.PlanItem, 0, len(validationAreas))
	for _, area := range validationAreas {
		item := llm.PlanItem{Area: area, Selected: false, Depth: "none", Reason: "No related dependency"}
		if area == "data" {
			item = llm.PlanItem{Area: area, Selected: true, Depth: "deep", Reason: "Critical domestic data dependency", DriverIDs: []string{"a1", "d1"}}
		}
		items = append(items, item)
	}
	service := New(&memoryStore{}, analyzer{plan: llm.PlanResult{Items: items}}, "evaluation", "0.1.0")
	response, err := service.CreateValidationPlan(context.Background(), &evaluationv1.CreateValidationPlanRequest{DocumentId: "doc", DocumentVersion: 1, Purpose: "Validate a data product", Country: "KR", Domain: "healthcare", Assumptions: []*evaluationv1.Assumption{{Id: "a1", Criticality: evaluationv1.AssumptionCriticality_ASSUMPTION_CRITICALITY_CRITICAL}}, Dependencies: []*documentv1.Dependency{{Id: "d1", Type: documentv1.DependencyType_DEPENDENCY_TYPE_DATA, Content: "Domestic clinical data"}}})
	if err != nil || len(response.GetItems()) != len(validationAreas) {
		t.Fatalf("validation plan = %#v, %v", response, err)
	}
	var data *evaluationv1.ValidationPlanItem
	for _, item := range response.GetItems() {
		if item.GetArea() == evaluationv1.ValidationArea_VALIDATION_AREA_DATA {
			data = item
		}
	}
	if data == nil || !data.GetSelected() || data.GetDepth() != evaluationv1.ValidationDepth_VALIDATION_DEPTH_DEEP || len(data.GetDriverIds()) != 2 {
		t.Fatalf("data validation = %#v", data)
	}
}

func TestCreateValidationPlanRejectsMissingArea(t *testing.T) {
	service := New(&memoryStore{}, analyzer{plan: llm.PlanResult{Items: []llm.PlanItem{{Area: "purpose", Selected: true, Depth: "basic", Reason: "Required"}}}}, "evaluation", "0.1.0")
	_, err := service.CreateValidationPlan(context.Background(), &evaluationv1.CreateValidationPlanRequest{DocumentId: "doc", DocumentVersion: 1, Purpose: "Plan", Country: "KR", Domain: "retail"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateValidationPlanEnforcesRequiredAreasAndDrivers(t *testing.T) {
	items := make([]llm.PlanItem, 0, len(validationAreas))
	for _, area := range validationAreas {
		items = append(items, llm.PlanItem{Area: area, Selected: false, Depth: "none", Reason: "Model considered it unnecessary"})
	}
	service := New(&memoryStore{}, analyzer{plan: llm.PlanResult{Items: items}}, "evaluation", "0.1.0")
	response, err := service.CreateValidationPlan(context.Background(), &evaluationv1.CreateValidationPlanRequest{DocumentId: "doc", DocumentVersion: 1, Purpose: "Data product", Country: "KR", Domain: "healthcare", Assumptions: []*evaluationv1.Assumption{{Id: "a1", Criticality: evaluationv1.AssumptionCriticality_ASSUMPTION_CRITICALITY_CRITICAL}}, Dependencies: []*documentv1.Dependency{{Id: "d1", Type: documentv1.DependencyType_DEPENDENCY_TYPE_DATA, Content: "Required dataset"}}})
	if err != nil {
		t.Fatal(err)
	}
	selected := map[evaluationv1.ValidationArea]*evaluationv1.ValidationPlanItem{}
	for _, item := range response.GetItems() {
		if item.GetSelected() {
			selected[item.GetArea()] = item
		}
	}
	if selected[evaluationv1.ValidationArea_VALIDATION_AREA_DATA] == nil {
		t.Fatal("required data validation was not selected")
	}
	verification := selected[evaluationv1.ValidationArea_VALIDATION_AREA_VERIFIABILITY]
	if verification == nil || verification.GetDepth() != evaluationv1.ValidationDepth_VALIDATION_DEPTH_DEEP || len(verification.GetDriverIds()) != 1 || verification.GetDriverIds()[0] != "a1" {
		t.Fatalf("critical assumption was not enforced: %#v", verification)
	}
}

func TestCreateValidationPlanRemovesInventedDriver(t *testing.T) {
	items := make([]llm.PlanItem, 0, len(validationAreas))
	for _, area := range validationAreas {
		items = append(items, llm.PlanItem{Area: area, Selected: false, Depth: "none", Reason: "Not applicable"})
	}
	items[0] = llm.PlanItem{Area: "purpose", Selected: true, Depth: "basic", Reason: "Driven", DriverIDs: []string{"invented"}}
	service := New(&memoryStore{}, analyzer{plan: llm.PlanResult{Items: items}}, "evaluation", "0.1.0")
	response, err := service.CreateValidationPlan(context.Background(), &evaluationv1.CreateValidationPlanRequest{DocumentId: "doc", DocumentVersion: 1, Purpose: "Plan", Country: "KR", Domain: "retail"})
	if err != nil {
		t.Fatal(err)
	}
	if got := response.GetItems()[0].GetDriverIds(); len(got) != 0 {
		t.Fatalf("invented drivers were retained: %v", got)
	}
	if !strings.Contains(response.GetItems()[0].GetReason(), "미등록 판단 요소") {
		t.Fatalf("normalization reason missing: %q", response.GetItems()[0].GetReason())
	}
}

func TestRecordValidationResultsExposesExcludedStatusWithoutDefect(t *testing.T) {
	service := New(&memoryStore{}, analyzer{}, "evaluation", "0.1.0")
	response, err := service.RecordValidationResults(context.Background(), &evaluationv1.RecordValidationResultsRequest{ValidationPlanId: "plan-1", Results: []*evaluationv1.ValidationResultInput{
		{Area: evaluationv1.ValidationArea_VALIDATION_AREA_PURPOSE, Status: evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED, Reason: "Purpose mismatch"},
		{Area: evaluationv1.ValidationArea_VALIDATION_AREA_DATA, Status: evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_APPLICABLE, Reason: "No data dependency"},
		{Area: evaluationv1.ValidationArea_VALIDATION_AREA_API, Status: evaluationv1.ValidationStatus_VALIDATION_STATUS_SKIPPED, Reason: "Connector disabled"},
	}})
	if err != nil || response.GetDefectCount() != 1 || response.GetScoredCount() != 1 || response.GetExcludedCount() != 2 {
		t.Fatalf("result summary = %#v, %v", response, err)
	}
	if response.GetResults()[1].GetCountedAsDefect() || response.GetResults()[1].GetIncludedInScore() || response.GetResults()[2].GetCountedAsDefect() {
		t.Fatalf("excluded status = %#v", response.GetResults())
	}
}

func TestEvaluatePurposeAlignmentReportsPurposelessFeatureAndLogicalGapWithSources(t *testing.T) {
	service := New(&memoryStore{}, analyzer{alignment: llm.AlignmentResult{Findings: []llm.AlignmentFinding{
		{Type: "purposeless_feature", Statement: "Add an AI chatbot", Finding: "The feature is not connected to the stated customer problem", ReasoningSummary: "The problem concerns delivery delay but the solution describes a chatbot without a causal link", Impact: "Scope and cost increase without measurable benefit", Severity: "high", Confidence: .9, RequiredAction: "Connect the feature to a verified problem or remove it", SourceBlockOrdinals: []uint32{1, 3}},
		{Type: "logical_gap", Statement: "Automation will reduce churn", Finding: "No mechanism or success criterion connects automation to churn", ReasoningSummary: "The goal follows from the solution without supporting steps", Impact: "Expected outcome cannot be validated", Severity: "medium", Confidence: .8, RequiredAction: "Define the causal mechanism and churn metric", SourceBlockOrdinals: []uint32{2, 3}},
	}}}, "evaluation", "0.1.0")
	response, err := service.EvaluatePurposeAlignment(context.Background(), &evaluationv1.EvaluatePurposeAlignmentRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{
		{Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_PROBLEM, Content: "Delivery is delayed", SourceBlockOrdinal: 1, SourceLocation: &documentv1.SourceLocation{PageNumber: 1}},
		{Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_GOAL, Content: "Reduce churn", SourceBlockOrdinal: 2, SourceLocation: &documentv1.SourceLocation{PageNumber: 2}},
		{Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_SOLUTION, Content: "Add an AI chatbot", SourceBlockOrdinal: 3, SourceLocation: &documentv1.SourceLocation{PageNumber: 3}},
	}})
	if err != nil || response.GetStatus() != evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED || len(response.GetFindings()) != 2 {
		t.Fatalf("alignment = %#v, %v", response, err)
	}
	if len(response.GetFindings()[0].GetSourceLocations()) != 2 || response.GetFindings()[0].GetSourceLocations()[1].GetPageNumber() != 3 {
		t.Fatalf("finding sources = %#v", response.GetFindings()[0])
	}
}

func TestEvaluatePurposeAlignmentAcceptsExplicitDocumentAbsence(t *testing.T) {
	service := New(&memoryStore{}, analyzer{alignment: llm.AlignmentResult{Findings: []llm.AlignmentFinding{{Type: "missing", Statement: "Success criterion", Finding: "The goal has no measurable success criterion", ReasoningSummary: "No metric or threshold is supplied", Impact: "Completion cannot be judged", Severity: "medium", Confidence: .85, RequiredAction: "Add a metric, baseline, target and period", DocumentAbsence: true}}}}, "evaluation", "0.1.0")
	response, err := service.EvaluatePurposeAlignment(context.Background(), &evaluationv1.EvaluatePurposeAlignmentRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_GOAL, Content: "Improve retention", SourceBlockOrdinal: 1}}})
	if err != nil || !response.GetFindings()[0].GetDocumentAbsence() || len(response.GetFindings()[0].GetSourceBlockOrdinals()) != 0 {
		t.Fatalf("absence finding = %#v, %v", response, err)
	}
}

func TestEvaluatePurposeAlignmentMarksInventedSourceUnverifiable(t *testing.T) {
	service := New(&memoryStore{}, analyzer{alignment: llm.AlignmentResult{Findings: []llm.AlignmentFinding{{Type: "logical_gap", Statement: "Claim", Finding: "Gap", ReasoningSummary: "Reason", Impact: "Impact", Severity: "high", Confidence: .8, RequiredAction: "Fix", SourceBlockOrdinals: []uint32{99}}}}}, "evaluation", "0.1.0")
	response, err := service.EvaluatePurposeAlignment(context.Background(), &evaluationv1.EvaluatePurposeAlignmentRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Content: "Known", SourceBlockOrdinal: 1}}})
	if err != nil || len(response.GetFindings()) != 1 || !response.GetFindings()[0].GetDocumentAbsence() || response.GetFindings()[0].GetConfidence() != 0 {
		t.Fatalf("unverifiable finding = %#v, %v", response, err)
	}
}

func TestDetectContradictionsReturnsBothSourceLocations(t *testing.T) {
	service := New(&memoryStore{}, analyzer{contradictions: llm.ContradictionResult{Contradictions: []llm.Contradiction{{Category: "figure", FirstStatement: "Launch budget is 10 million won", SecondStatement: "Total launch budget is 30 million won", Finding: "The same launch budget has two incompatible totals", ReasoningSummary: "Both statements use the same scope and period but specify different totals", Impact: "Approval and resource allocation cannot use a reliable baseline", Severity: "high", Confidence: .95, RequiredAction: "Select one authoritative budget and update all references", SourceBlockOrdinals: []uint32{2, 7}}}}}, "evaluation", "0.1.0")
	response, err := service.DetectContradictions(context.Background(), &evaluationv1.DetectContradictionsRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_CLAIM, Content: "Launch budget is 10 million won", SourceBlockOrdinal: 2, SourceLocation: &documentv1.SourceLocation{PageNumber: 1}}, {Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_CLAIM, Content: "Total launch budget is 30 million won", SourceBlockOrdinal: 7, SourceLocation: &documentv1.SourceLocation{PageNumber: 4}}}})
	if err != nil || response.GetStatus() != evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED || len(response.GetContradictions()) != 1 {
		t.Fatalf("contradictions = %#v, %v", response, err)
	}
	finding := response.GetContradictions()[0].GetFinding()
	if finding.GetArea() != evaluationv1.ValidationArea_VALIDATION_AREA_CONSISTENCY || len(finding.GetSourceLocations()) != 2 || finding.GetSourceLocations()[1].GetPageNumber() != 4 {
		t.Fatalf("contradiction sources = %#v", finding)
	}
}

func TestDetectContradictionsRejectsSingleOrInventedSource(t *testing.T) {
	for name, ordinals := range map[string][]uint32{"single": {1}, "invented": {1, 99}} {
		t.Run(name, func(t *testing.T) {
			service := New(&memoryStore{}, analyzer{contradictions: llm.ContradictionResult{Contradictions: []llm.Contradiction{{Category: "policy", FirstStatement: "Login required", SecondStatement: "Guest access", Finding: "Conflict", ReasoningSummary: "Same flow has incompatible access policy", Impact: "Authorization is undefined", Severity: "high", Confidence: .8, RequiredAction: "Choose access policy", SourceBlockOrdinals: ordinals}}}}, "evaluation", "0.1.0")
			_, err := service.DetectContradictions(context.Background(), &evaluationv1.DetectContradictionsRequest{DocumentId: "doc", DocumentVersion: 1, Requirements: []*documentv1.Requirement{{Content: "Login required", SourceBlockOrdinal: 1}, {Content: "Guest access", SourceBlockOrdinal: 2}}})
			if status.Code(err) != codes.Internal {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestDetectContradictionsReturnsSatisfiedWhenNoConflict(t *testing.T) {
	service := New(&memoryStore{}, analyzer{}, "evaluation", "0.1.0")
	response, err := service.DetectContradictions(context.Background(), &evaluationv1.DetectContradictionsRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Content: "Pilot in Q1", SourceBlockOrdinal: 1}, {Content: "General release in Q2", SourceBlockOrdinal: 2}}})
	if err != nil || response.GetStatus() != evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED || len(response.GetContradictions()) != 0 {
		t.Fatalf("no conflict = %#v, %v", response, err)
	}
}

func TestEvaluateRequirementCompletenessReturnsSpecificMissingDecisions(t *testing.T) {
	service := New(&memoryStore{}, analyzer{completeness: llm.CompletenessResult{Gaps: []llm.RequirementGap{
		{Category: "permission", Statement: "Managers may approve refunds", Finding: "The required role and unauthorized behavior are undefined", ReasoningSummary: "Approval changes financial state and therefore needs an explicit authorization boundary", Impact: "Unauthorized refunds or inconsistent implementations may occur", Severity: "high", Confidence: .92, MissingDecision: "Define which manager roles may approve each refund amount and the response for unauthorized attempts", SourceBlockOrdinals: []uint32{4}},
		{Category: "failure", Statement: "Send payment to the external API", Finding: "Timeout and duplicate-request handling are missing", ReasoningSummary: "The dependency can fail after accepting a request without returning a result", Impact: "A retry may charge the customer twice", Severity: "blocking", Confidence: .9, MissingDecision: "Define timeout, idempotency key, retry limit, and reconciliation behavior", SourceBlockOrdinals: []uint32{6, 8}},
	}}}, "evaluation", "0.1.0")
	response, err := service.EvaluateRequirementCompleteness(context.Background(), &evaluationv1.EvaluateRequirementCompletenessRequest{DocumentId: "doc", DocumentVersion: 1, Requirements: []*documentv1.Requirement{{Type: documentv1.RequirementType_REQUIREMENT_TYPE_POLICY, Content: "Managers may approve refunds", SourceBlockOrdinal: 4, SourceLocation: &documentv1.SourceLocation{PageNumber: 2}}, {Type: documentv1.RequirementType_REQUIREMENT_TYPE_REQUIREMENT, Content: "Send payment to the external API", SourceBlockOrdinal: 6, SourceLocation: &documentv1.SourceLocation{PageNumber: 3}}}, Dependencies: []*documentv1.Dependency{{Type: documentv1.DependencyType_DEPENDENCY_TYPE_API, Content: "Payment API", SourceBlockOrdinal: 8, SourceLocation: &documentv1.SourceLocation{PageNumber: 5}}}})
	if err != nil || response.GetStatus() != evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED || len(response.GetGaps()) != 2 {
		t.Fatalf("completeness = %#v, %v", response, err)
	}
	gap := response.GetGaps()[1]
	if gap.GetCategory() != evaluationv1.RequirementGapCategory_REQUIREMENT_GAP_CATEGORY_FAILURE || !strings.Contains(gap.GetMissingDecision(), "idempotency") || len(gap.GetFinding().GetSourceLocations()) != 2 || !gap.GetFinding().GetDocumentAbsence() {
		t.Fatalf("failure gap = %#v", gap)
	}
}

func TestEvaluateRequirementCompletenessRejectsVagueOrInventedGap(t *testing.T) {
	tests := map[string]llm.RequirementGap{"vague": {Category: "exception", Statement: "Upload file", Finding: "Details missing", ReasoningSummary: "Needs detail", Impact: "Unknown behavior", Severity: "medium", Confidence: .7, SourceBlockOrdinals: []uint32{1}}, "invented": {Category: "boundary", Statement: "Upload file", Finding: "Size boundary missing", ReasoningSummary: "Resource use needs a limit", Impact: "Memory exhaustion", Severity: "high", Confidence: .8, MissingDecision: "Set maximum file size", SourceBlockOrdinals: []uint32{99}}}
	for name, gap := range tests {
		t.Run(name, func(t *testing.T) {
			service := New(&memoryStore{}, analyzer{completeness: llm.CompletenessResult{Gaps: []llm.RequirementGap{gap}}}, "evaluation", "0.1.0")
			_, err := service.EvaluateRequirementCompleteness(context.Background(), &evaluationv1.EvaluateRequirementCompletenessRequest{DocumentId: "doc", DocumentVersion: 1, Requirements: []*documentv1.Requirement{{Content: "Upload file", SourceBlockOrdinal: 1}}})
			if status.Code(err) != codes.Internal {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestEvaluateRequirementCompletenessReturnsSatisfiedWhenComplete(t *testing.T) {
	service := New(&memoryStore{}, analyzer{}, "evaluation", "0.1.0")
	response, err := service.EvaluateRequirementCompleteness(context.Background(), &evaluationv1.EvaluateRequirementCompletenessRequest{DocumentId: "doc", DocumentVersion: 1, Requirements: []*documentv1.Requirement{{Content: "Complete requirement", SourceBlockOrdinal: 1}}})
	if err != nil || response.GetStatus() != evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED || len(response.GetGaps()) != 0 {
		t.Fatalf("complete = %#v, %v", response, err)
	}
}

func TestEvaluateDocumentWorkQualityUsesOnlyDocumentEvidence(t *testing.T) {
	service := New(&memoryStore{}, analyzer{quality: llm.WorkQualityResult{Results: []llm.WorkQualityItem{
		{Dimension: "research", Status: "partially_satisfied", Finding: "One market figure has a named source but no scope", ReasoningSummary: "The document cites a report but omits region and period", Confidence: .85, SourceBlockOrdinals: []uint32{1}},
		{Dimension: "validation", Status: "not_satisfied", Finding: "The key demand assumption has no test or counter-evidence review", ReasoningSummary: "The document presents demand as a conclusion without a validation method", Confidence: .9, SourceBlockOrdinals: []uint32{2}},
		{Dimension: "logic", Status: "satisfied", Finding: "The selected feature is explicitly linked to the stated problem", ReasoningSummary: "Problem, mechanism and expected outcome are present in adjacent elements", Confidence: .8, SourceBlockOrdinals: []uint32{2, 3}},
	}}}, "evaluation", "0.1.0")
	response, err := service.EvaluateDocumentWorkQuality(context.Background(), &evaluationv1.EvaluateDocumentWorkQualityRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_EVIDENCE, Content: "Market report says demand is 30%", SourceBlockOrdinal: 1, SourceLocation: &documentv1.SourceLocation{PageNumber: 1}}, {Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_ASSUMPTION, Content: "Customers demand automation", SourceBlockOrdinal: 2, SourceLocation: &documentv1.SourceLocation{PageNumber: 2}}, {Type: documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_SOLUTION, Content: "Automate the repeated task", SourceBlockOrdinal: 3, SourceLocation: &documentv1.SourceLocation{PageNumber: 3}}}})
	if err != nil || len(response.GetResults()) != 3 || response.GetResults()[1].GetStatus() != evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED || response.GetResults()[2].GetSourceLocations()[1].GetPageNumber() != 3 {
		t.Fatalf("quality = %#v, %v", response, err)
	}
}

func TestEvaluateDocumentWorkQualityRejectsMissingDimensionAndInventedEvidence(t *testing.T) {
	tests := map[string][]llm.WorkQualityItem{"missing": {{Dimension: "research", Status: "satisfied", Finding: "ok", ReasoningSummary: "supported", Confidence: .8, SourceBlockOrdinals: []uint32{1}}}, "invented": {{Dimension: "research", Status: "satisfied", Finding: "ok", ReasoningSummary: "supported", Confidence: .8, SourceBlockOrdinals: []uint32{1}}, {Dimension: "validation", Status: "unverified", Finding: "unknown", ReasoningSummary: "not documented", Confidence: .5, SourceBlockOrdinals: []uint32{1}}, {Dimension: "logic", Status: "not_satisfied", Finding: "gap", ReasoningSummary: "unsupported jump", Confidence: .8, SourceBlockOrdinals: []uint32{99}}}}
	for name, items := range tests {
		t.Run(name, func(t *testing.T) {
			service := New(&memoryStore{}, analyzer{quality: llm.WorkQualityResult{Results: items}}, "evaluation", "0.1.0")
			_, err := service.EvaluateDocumentWorkQuality(context.Background(), &evaluationv1.EvaluateDocumentWorkQualityRequest{DocumentId: "doc", DocumentVersion: 1, Elements: []*documentv1.DocumentElement{{Content: "Known", SourceBlockOrdinal: 1}}})
			if status.Code(err) != codes.Internal {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
