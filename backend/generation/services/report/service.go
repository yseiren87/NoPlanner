package report

import (
	"context"
	"errors"

	"google.golang.org/protobuf/types/known/timestamppb"
	design "noplanner/backend/generation/domains/design"
	implementation "noplanner/backend/generation/domains/implementation"
	planning "noplanner/backend/generation/domains/planning"
	domain "noplanner/backend/generation/domains/report"
	"noplanner/backend/generation/modules/connectors"
	"noplanner/backend/generation/modules/rpcerror"
	"noplanner/backend/generation/modules/uivalidator"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

type Service struct {
	generationv1.UnimplementedGenerationServiceServer
	store               domain.Store
	implementationStore implementation.Store
	planningStore       planning.Store
	designStore         design.Store
	connectors          *connectors.Registry
	intelligence        intelligencev1.IntelligenceServiceClient
	evaluation          evaluationv1.EvaluationServiceClient
	repositoryRoots     []string
	uiValidator         interface {
		Validate(context.Context, uivalidator.Product) []uivalidator.Validation
	}
	name, version string
}

func NewWithImplementation(store domain.Store, implementationStore implementation.Store, planningStore planning.Store, designStore design.Store, connectorRegistry *connectors.Registry, intelligence intelligencev1.IntelligenceServiceClient, evaluation evaluationv1.EvaluationServiceClient, repositoryRoots []string, name, version string) *Service {
	return &Service{store: store, implementationStore: implementationStore, planningStore: planningStore, designStore: designStore, connectors: connectorRegistry, intelligence: intelligence, evaluation: evaluation, repositoryRoots: append([]string(nil), repositoryRoots...), name: name, version: version}
}

func New(store domain.Store, name, version string) *Service {
	return &Service{store: store, name: name, version: version}
}

func (s *Service) SetUIValidator(validator interface {
	Validate(context.Context, uivalidator.Product) []uivalidator.Validation
}) {
	s.uiValidator = validator
}
func (s *Service) GetStatus(context.Context, *commonv1.StatusRequest) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{Service: s.name, Version: s.version, State: "ready"}, nil
}

func (s *Service) CreateReport(ctx context.Context, req *generationv1.CreateReportRequest) (*generationv1.EvaluationReport, error) {
	base := domain.Report{EvaluationID: req.GetEvaluationId(), ProjectID: req.GetProjectId(), ProjectName: req.GetProjectName(), OwnerSubject: req.GetOwnerSubject(), DocumentID: req.GetDocumentId(), DocumentName: req.GetDocumentName(), DocumentVersion: req.GetDocumentVersion(), Country: req.GetCountry(), Domain: req.GetDomain(), TargetUser: req.GetTargetUser(), CriteriaVersion: req.GetCriteriaVersion(), Model: req.GetModel(), Summary: mapSummary(req.GetSummary()), EvaluationStatus: statusName(req.GetEvaluationStatus()), Limitations: req.GetLimitations()}
	for _, v := range req.GetAreas() {
		base.Areas = append(base.Areas, domain.AreaResult{Area: v.GetArea(), Status: v.GetStatus(), Reason: v.GetReason(), FindingCount: v.GetFindingCount(), Confidence: confidenceName(v.GetConfidence())})
	}
	for _, v := range req.GetAssumptions() {
		base.Assumptions = append(base.Assumptions, domain.Assumption{ID: v.GetId(), Statement: v.GetStatement(), Status: v.GetStatus(), Dependencies: v.GetDependencies(), EvidenceIDs: v.GetEvidenceIds()})
	}
	for _, v := range req.GetContradictions() {
		base.Contradictions = append(base.Contradictions, domain.Contradiction{ID: v.GetId(), FirstLocation: v.GetFirstLocation(), FirstStatement: v.GetFirstStatement(), SecondLocation: v.GetSecondLocation(), SecondStatement: v.GetSecondStatement(), Reason: v.GetReason()})
	}
	for _, v := range req.GetResearch() {
		base.Research = append(base.Research, mapResearch(v))
	}
	for _, v := range req.GetProgress() {
		base.Progress = append(base.Progress, domain.ProgressStep{Code: v.GetCode(), Label: v.GetLabel(), Status: v.GetStatus(), Reason: v.GetReason()})
	}
	inputs := make([]domain.FindingInput, 0, len(req.GetFindings()))
	for _, v := range req.GetFindings() {
		inputs = append(inputs, mapFinding(v))
	}
	built, err := domain.Assemble(base, inputs)
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "리포트 입력과 근거가 올바르지 않습니다.", false)
	}
	saved, err := s.store.Save(ctx, built)
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "리포트를 저장하지 못했습니다.", true)
	}
	return toProto(saved), nil
}
func (s *Service) GetReport(ctx context.Context, req *generationv1.GetReportRequest) (*generationv1.EvaluationReport, error) {
	value, err := s.store.Get(ctx, req.GetReportId())
	if errors.Is(err, domain.ErrNotFound) {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_NOT_FOUND, "리포트를 찾을 수 없습니다.", false)
	}
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "리포트를 불러오지 못했습니다.", true)
	}
	return toProto(value), nil
}
func (s *Service) ExportReport(ctx context.Context, req *generationv1.ExportReportRequest) (*generationv1.ExportReportResponse, error) {
	value, err := s.store.Get(ctx, req.GetReportId())
	if errors.Is(err, domain.ErrNotFound) {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_NOT_FOUND, "리포트를 찾을 수 없습니다.", false)
	}
	format := map[generationv1.ExportFormat]string{generationv1.ExportFormat_EXPORT_FORMAT_MARKDOWN: "markdown", generationv1.ExportFormat_EXPORT_FORMAT_PDF: "pdf", generationv1.ExportFormat_EXPORT_FORMAT_DOCX: "docx", generationv1.ExportFormat_EXPORT_FORMAT_JSON: "json"}[req.GetFormat()]
	content, contentType, filename, err := domain.Export(value, format)
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "지원하지 않는 내보내기 형식입니다.", false)
	}
	return &generationv1.ExportReportResponse{Content: content, ContentType: contentType, Filename: filename}, nil
}

func mapEvidence(v *generationv1.EvidenceLink) domain.Evidence {
	return domain.Evidence{ID: v.GetId(), Title: v.GetTitle(), URL: v.GetUrl(), SourceLocation: v.GetSourceLocation(), Grade: v.GetGrade(), Relation: v.GetRelation()}
}
func mapFinding(v *generationv1.FindingInput) domain.FindingInput {
	out := domain.FindingInput{ID: v.GetId(), Area: v.GetArea(), ProblemType: v.GetProblemType(), SourceDocument: v.GetSourceDocument(), SourceLocation: v.GetSourceLocation(), Statement: v.GetStatement(), Finding: v.GetFinding(), ReasoningSummary: v.GetReasoningSummary(), Impact: v.GetImpact(), Likelihood: v.GetLikelihood(), ImpactScore: v.GetImpactScore(), RecoveryCost: v.GetRecoveryCost(), Reversibility: v.GetReversibility(), RequiredAction: v.GetRequiredAction(), VerificationMethod: v.GetVerificationMethod(), Status: v.GetStatus(), CriticalAssumption: v.GetCriticalAssumption(), ConfirmedFalse: v.GetConfirmedFalse()}
	for _, e := range v.GetEvidence() {
		out.Evidence = append(out.Evidence, mapEvidence(e))
	}
	return out
}
func mapResearch(v *generationv1.ResearchResult) domain.Research {
	out := domain.Research{Question: v.GetQuestion(), Conclusion: v.GetConclusion(), Uncertainties: v.GetUncertainties()}
	for _, e := range v.GetSources() {
		out.Sources = append(out.Sources, mapEvidence(e))
	}
	for _, e := range v.GetConflicts() {
		out.Conflicts = append(out.Conflicts, mapEvidence(e))
	}
	return out
}
func mapSummary(v *generationv1.PlanSummary) domain.Summary {
	if v == nil {
		return domain.Summary{}
	}
	return domain.Summary{Problem: v.GetProblem(), Purpose: v.GetPurpose(), TargetUser: v.GetTargetUser(), Environment: v.GetEnvironment(), Solution: v.GetSolution(), SuccessCriteria: v.GetSuccessCriteria()}
}
func confidenceName(v generationv1.Confidence) string {
	switch v {
	case generationv1.Confidence_CONFIDENCE_HIGH:
		return "high"
	case generationv1.Confidence_CONFIDENCE_MEDIUM:
		return "medium"
	default:
		return "low"
	}
}
func statusName(v generationv1.EvaluationStatus) string { return v.String() }
func statusProto(v string) generationv1.EvaluationStatus {
	if number, ok := generationv1.EvaluationStatus_value[v]; ok {
		return generationv1.EvaluationStatus(number)
	}
	return generationv1.EvaluationStatus_EVALUATION_STATUS_UNSPECIFIED
}
func severityProto(v string) generationv1.Severity {
	return map[string]generationv1.Severity{"low": generationv1.Severity_SEVERITY_LOW, "medium": generationv1.Severity_SEVERITY_MEDIUM, "high": generationv1.Severity_SEVERITY_HIGH, "blocking": generationv1.Severity_SEVERITY_BLOCKING}[v]
}
func confidenceProto(v string) generationv1.Confidence {
	return map[string]generationv1.Confidence{"low": generationv1.Confidence_CONFIDENCE_LOW, "medium": generationv1.Confidence_CONFIDENCE_MEDIUM, "high": generationv1.Confidence_CONFIDENCE_HIGH}[v]
}
func verdictProto(v string) generationv1.Verdict {
	return map[string]generationv1.Verdict{"executable": generationv1.Verdict_VERDICT_EXECUTABLE, "conditionally_executable": generationv1.Verdict_VERDICT_CONDITIONALLY_EXECUTABLE, "rewrite_required": generationv1.Verdict_VERDICT_REWRITE_REQUIRED, "not_executable": generationv1.Verdict_VERDICT_NOT_EXECUTABLE}[v]
}
func toProto(r domain.Report) *generationv1.EvaluationReport {
	out := &generationv1.EvaluationReport{Id: r.ID, EvaluationId: r.EvaluationID, ProjectId: r.ProjectID, ProjectName: r.ProjectName, OwnerSubject: r.OwnerSubject, DocumentId: r.DocumentID, DocumentName: r.DocumentName, DocumentVersion: r.DocumentVersion, Country: r.Country, Domain: r.Domain, TargetUser: r.TargetUser, CriteriaVersion: r.CriteriaVersion, Model: r.Model, CreatedAt: timestamppb.New(r.CreatedAt), Verdict: verdictProto(r.Verdict), Confidence: confidenceProto(r.Confidence), Conclusion: r.Conclusion, ImplementationMayStart: r.ImplementationMayStart, RequiredActions: r.RequiredActions, Recommendations: r.Recommendations, VerdictChangeConditions: r.VerdictChangeConditions, Limitations: r.Limitations, Summary: &generationv1.PlanSummary{Problem: r.Summary.Problem, Purpose: r.Summary.Purpose, TargetUser: r.Summary.TargetUser, Environment: r.Summary.Environment, Solution: r.Summary.Solution, SuccessCriteria: r.Summary.SuccessCriteria}, EvaluationStatus: statusProto(r.EvaluationStatus)}
	for _, f := range r.Findings {
		input := &generationv1.FindingInput{Id: f.Detail.ID, Area: f.Detail.Area, ProblemType: f.Detail.ProblemType, SourceDocument: f.Detail.SourceDocument, SourceLocation: f.Detail.SourceLocation, Statement: f.Detail.Statement, Finding: f.Detail.Finding, ReasoningSummary: f.Detail.ReasoningSummary, Impact: f.Detail.Impact, Likelihood: f.Detail.Likelihood, ImpactScore: f.Detail.ImpactScore, RecoveryCost: f.Detail.RecoveryCost, Reversibility: f.Detail.Reversibility, RequiredAction: f.Detail.RequiredAction, VerificationMethod: f.Detail.VerificationMethod, Status: f.Detail.Status, CriticalAssumption: f.Detail.CriticalAssumption, ConfirmedFalse: f.Detail.ConfirmedFalse}
		for _, e := range f.Detail.Evidence {
			input.Evidence = append(input.Evidence, &generationv1.EvidenceLink{Id: e.ID, Title: e.Title, Url: e.URL, SourceLocation: e.SourceLocation, Grade: e.Grade, Relation: e.Relation})
		}
		out.Findings = append(out.Findings, &generationv1.Finding{Detail: input, Severity: severityProto(f.Severity), Confidence: confidenceProto(f.Confidence)})
	}
	for _, a := range r.Areas {
		out.Areas = append(out.Areas, &generationv1.AreaResult{Area: a.Area, Status: a.Status, Reason: a.Reason, FindingCount: a.FindingCount, Confidence: confidenceProto(a.Confidence)})
	}
	for _, a := range r.Assumptions {
		out.Assumptions = append(out.Assumptions, &generationv1.AssumptionResult{Id: a.ID, Statement: a.Statement, Status: a.Status, Dependencies: a.Dependencies, EvidenceIds: a.EvidenceIDs})
	}
	for _, c := range r.Contradictions {
		out.Contradictions = append(out.Contradictions, &generationv1.Contradiction{Id: c.ID, FirstLocation: c.FirstLocation, FirstStatement: c.FirstStatement, SecondLocation: c.SecondLocation, SecondStatement: c.SecondStatement, Reason: c.Reason})
	}
	for _, q := range r.Research {
		p := &generationv1.ResearchResult{Question: q.Question, Conclusion: q.Conclusion, Uncertainties: q.Uncertainties}
		for _, e := range q.Sources {
			p.Sources = append(p.Sources, &generationv1.EvidenceLink{Id: e.ID, Title: e.Title, Url: e.URL, SourceLocation: e.SourceLocation, Grade: e.Grade, Relation: e.Relation})
		}
		for _, e := range q.Conflicts {
			p.Conflicts = append(p.Conflicts, &generationv1.EvidenceLink{Id: e.ID, Title: e.Title, Url: e.URL, SourceLocation: e.SourceLocation, Grade: e.Grade, Relation: e.Relation})
		}
		out.Research = append(out.Research, p)
	}
	for _, p := range r.Progress {
		out.Progress = append(out.Progress, &generationv1.ProgressStep{Code: p.Code, Label: p.Label, Status: p.Status, Reason: p.Reason})
	}
	return out
}
