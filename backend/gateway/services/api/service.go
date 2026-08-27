package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"noplanner/backend/gateway/modules/clients"
	"noplanner/backend/gateway/modules/jobs"
	"noplanner/backend/gateway/modules/token"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	projectv1 "noplanner/backend/proto/dist/golang/project/v1"
	researchv1 "noplanner/backend/proto/dist/golang/research/v1"
	userv1 "noplanner/backend/proto/dist/golang/user/v1"
)

type Service struct {
	clients  *clients.Clients
	verifier *token.Verifier
	queue    *jobs.Queue
}

func New(c *clients.Clients, v *token.Verifier, q *jobs.Queue) *Service {
	return &Service{clients: c, verifier: v, queue: q}
}

func (s *Service) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/auth/google", s.beginLogin)
	m.HandleFunc("POST /api/auth/google/callback", s.completeLogin)
	m.HandleFunc("GET /api/me", s.auth(s.me))
	m.HandleFunc("POST /api/projects", s.auth(s.createProject))
	m.HandleFunc("GET /api/projects", s.auth(s.listProjects))
	m.HandleFunc("GET /api/projects/{id}", s.auth(s.getProject))
	m.HandleFunc("PATCH /api/projects/{id}", s.auth(s.updateProject))
	m.HandleFunc("PUT /api/projects/{id}/members/{subject}", s.auth(s.setProjectMemberRole))
	m.HandleFunc("PUT /api/projects/{id}/output-language", s.auth(s.updateProjectOutputLanguage))
	m.HandleFunc("POST /api/evaluations", s.auth(s.evaluate))
	m.HandleFunc("POST /api/evaluation-jobs", s.auth(s.enqueueEvaluation))
	m.HandleFunc("GET /api/evaluation-jobs/{id}", s.auth(s.getEvaluationJob))
	m.HandleFunc("GET /api/reports/{id}", s.auth(s.getReport))
	m.HandleFunc("GET /api/reports/{id}/exports/{format}", s.auth(s.exportReport))
	m.HandleFunc("GET /api/reports/{id}/verdict-history", s.auth(s.getVerdictHistory))
	m.HandleFunc("POST /api/reports/{id}/improvement-plan", s.auth(s.createImprovementPlan))
	m.HandleFunc("POST /api/reports/{id}/improved-plan", s.auth(s.improveExistingPlan))
	m.HandleFunc("POST /api/reports/{id}/objections", s.auth(s.submitObjection))
	m.HandleFunc("POST /api/reports/{id}/revisions", s.auth(s.reviseReport))
	m.HandleFunc("POST /api/planning/questions", s.auth(s.discoverPlanningQuestions))
	m.HandleFunc("POST /api/autonomous-plans", s.auth(s.generateAutonomousPlan))
	m.HandleFunc("GET /api/autonomous-plans/{id}", s.auth(s.getAutonomousPlan))
	m.HandleFunc("POST /api/product-designs", s.auth(s.generateProductDesign))
	m.HandleFunc("GET /api/product-designs/{id}", s.auth(s.getProductDesign))
	m.HandleFunc("POST /api/repositories/analyze", s.auth(s.analyzeRepository))
	m.HandleFunc("POST /api/implementation-reviews", s.auth(s.compareImplementation))
	m.HandleFunc("POST /api/implementation-proposals", s.auth(s.createImplementationProposal))
	m.HandleFunc("GET /api/connectors", s.auth(s.listConnectors))
	m.HandleFunc("POST /api/integrations/deliver", s.auth(s.deliverIntegration))
	m.HandleFunc("GET /api/projects/{id}/implementation-history", s.auth(s.getImplementationHistory))
	return cors(m)
}

func (s *Service) reportForAuthorizedRequest(w http.ResponseWriter, r *http.Request) (*generationv1.EvaluationReport, bool) {
	report, err := s.clients.Generation.GetReport(r.Context(), &generationv1.GetReportRequest{ReportId: r.PathValue("id")})
	if err != nil {
		grpcProblem(w, err)
		return nil, false
	}
	if !s.authorizeReport(w, r, report) {
		return nil, false
	}
	return report, true
}

func (s *Service) getVerdictHistory(w http.ResponseWriter, r *http.Request) {
	report, ok := s.reportForAuthorizedRequest(w, r)
	if !ok {
		return
	}
	value, err := s.clients.Evaluation.GetVerdictHistory(r.Context(), &evaluationv1.GetVerdictHistoryRequest{DocumentId: report.GetDocumentId()})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) createImprovementPlan(w http.ResponseWriter, r *http.Request) {
	report, ok := s.reportForAuthorizedRequest(w, r)
	if !ok {
		return
	}
	value, err := s.clients.Generation.CreateImprovementPlan(r.Context(), &generationv1.CreateImprovementPlanRequest{EvaluationId: report.GetEvaluationId(), Findings: report.GetFindings()})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) improveExistingPlan(w http.ResponseWriter, r *http.Request) {
	report, ok := s.reportForAuthorizedRequest(w, r)
	if !ok {
		return
	}
	req := &generationv1.ImproveExistingPlanRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	plan, err := s.clients.Generation.CreateImprovementPlan(r.Context(), &generationv1.CreateImprovementPlanRequest{EvaluationId: report.GetEvaluationId(), Findings: report.GetFindings()})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	req.ImprovementPlan = plan
	value, err := s.clients.Generation.ImproveExistingPlan(r.Context(), req)
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Service) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		sub, err := s.verifier.Verify(raw)
		if err != nil {
			problem(w, http.StatusUnauthorized, "authentication_required", err.Error())
			return
		}
		ctx := metadata.AppendToOutgoingContext(r.Context(), "authorization", "Bearer "+raw)
		ctx = context.WithValue(ctx, subjectKey{}, sub)
		next(w, r.WithContext(ctx))
	}
}

type subjectKey struct{}

func (s *Service) enqueueEvaluation(w http.ResponseWriter, r *http.Request) {
	if s.queue == nil {
		problem(w, 503, "queue_unavailable", "Redis 평가 큐가 설정되지 않았습니다.")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		problem(w, 400, "invalid_upload", err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		problem(w, 400, "file_required", err.Error())
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		problem(w, 400, "invalid_upload", err.Error())
		return
	}
	projectID := r.FormValue("projectId")
	if projectID == "" && strings.TrimSpace(r.FormValue("documentId")) == "" {
		project, e := s.clients.Project.CreateProject(r.Context(), &projectv1.CreateProjectRequest{Name: valueOr(r.FormValue("projectName"), "새 평가")})
		if e != nil {
			grpcProblem(w, e)
			return
		}
		projectID = project.Id
	}
	p := jobs.Payload{Owner: r.Context().Value(subjectKey{}).(string), ProjectID: projectID, ProjectName: r.FormValue("projectName"), FileName: header.Filename, MediaType: header.Header.Get("Content-Type"), Country: r.FormValue("country"), Domain: r.FormValue("domain"), TargetUser: r.FormValue("targetUser"), Document: body}
	j, err := s.queue.Enqueue(r.Context(), p, r.Header.Get("Idempotency-Key"))
	if err != nil {
		problem(w, 503, "queue_unavailable", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
	write(w, publicJob(j))
}
func (s *Service) getEvaluationJob(w http.ResponseWriter, r *http.Request) {
	if s.queue == nil {
		problem(w, 503, "queue_unavailable", "Redis 평가 큐가 설정되지 않았습니다.")
		return
	}
	j, e := s.queue.Get(r.Context(), r.PathValue("id"), r.Context().Value(subjectKey{}).(string))
	if e != nil {
		problem(w, 404, "job_not_found", "평가 작업을 찾을 수 없습니다.")
		return
	}
	write(w, publicJob(j))
}
func publicJob(j jobs.Job) map[string]any {
	return map[string]any{"id": j.ID, "status": j.Status, "stage": j.Stage, "reportId": j.ReportID, "error": j.Error, "attempts": j.Attempts, "maxAttempts": j.MaxAttempts, "createdAt": j.CreatedAt, "updatedAt": j.UpdatedAt}
}
func (s *Service) RunWorker(ctx context.Context) {
	if s.queue == nil {
		return
	}
	s.queue.Run(ctx, s.processEvaluationJob)
}
func (s *Service) processEvaluationJob(ctx context.Context, id string, p jobs.Payload) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, strings.ReplaceAll(p.FileName, `"`, "")))
	header.Set("Content-Type", valueOr(p.MediaType, "application/octet-stream"))
	part, e := writer.CreatePart(header)
	if e != nil {
		return "", e
	}
	if _, e = part.Write(p.Document); e != nil {
		return "", e
	}
	for k, v := range map[string]string{"projectId": p.ProjectID, "projectName": p.ProjectName, "country": p.Country, "domain": p.Domain, "targetUser": p.TargetUser, "ownerSubject": p.Owner} {
		if e = writer.WriteField(k, v); e != nil {
			return "", e
		}
	}
	if e = writer.Close(); e != nil {
		return "", e
	}
	req := httptest.NewRequest(http.MethodPost, "/api/evaluations", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Evaluation-Job-ID", id)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	_ = s.queue.Progress(ctx, id, "analyzing")
	s.evaluate(rec, req)
	if rec.Code < 200 || rec.Code >= 300 {
		return "", fmt.Errorf("evaluation pipeline returned %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if e = json.Unmarshal(rec.Body.Bytes(), &out); e != nil {
		return "", e
	}
	if out.ID == "" {
		return "", errors.New("report id missing")
	}
	return out.ID, nil
}

func (s *Service) beginLogin(w http.ResponseWriter, r *http.Request) {
	v, e := s.clients.User.BeginGoogleLogin(r.Context(), &userv1.BeginGoogleLoginRequest{})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	write(w, v)
}
func (s *Service) completeLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Code, State string }
	if !decode(w, r, &in) {
		return
	}
	v, e := s.clients.User.CompleteGoogleLogin(r.Context(), &userv1.CompleteGoogleLoginRequest{Code: in.Code, State: in.State})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	write(w, v)
}
func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	v, e := s.clients.User.GetCurrentUser(r.Context(), &userv1.GetCurrentUserRequest{})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	write(w, v)
}
func (s *Service) createProject(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if !decode(w, r, &in) {
		return
	}
	v, e := s.clients.Project.CreateProject(r.Context(), &projectv1.CreateProjectRequest{Name: in.Name})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	write(w, v)
}

func (s *Service) listProjects(w http.ResponseWriter, r *http.Request) {
	value, err := s.clients.Project.ListProjects(r.Context(), &projectv1.ListProjectsRequest{})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) getProject(w http.ResponseWriter, r *http.Request) {
	value, err := s.clients.Project.GetProject(r.Context(), &projectv1.GetProjectRequest{ProjectId: r.PathValue("id")})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) updateProject(w http.ResponseWriter, r *http.Request) {
	req := &projectv1.UpdateProjectRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	req.ProjectId = r.PathValue("id")
	value, err := s.clients.Project.UpdateProject(r.Context(), req)
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) setProjectMemberRole(w http.ResponseWriter, r *http.Request) {
	req := &projectv1.SetProjectMemberRoleRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	req.ProjectId = r.PathValue("id")
	req.Subject = r.PathValue("subject")
	value, err := s.clients.Project.SetProjectMemberRole(r.Context(), req)
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) updateProjectOutputLanguage(w http.ResponseWriter, r *http.Request) {
	req := &projectv1.UpdateOutputLanguageRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	req.ProjectId = r.PathValue("id")
	value, err := s.clients.Project.UpdateOutputLanguage(r.Context(), req)
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) evaluate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		problem(w, 400, "invalid_upload", err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		problem(w, 400, "file_required", err.Error())
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		problem(w, 400, "invalid_upload", err.Error())
		return
	}
	projectID := r.FormValue("projectId")
	if projectID == "" && strings.TrimSpace(r.FormValue("documentId")) == "" {
		p, e := s.clients.Project.CreateProject(r.Context(), &projectv1.CreateProjectRequest{Name: valueOr(r.FormValue("projectName"), "새 평가")})
		if e != nil {
			grpcProblem(w, e)
			return
		}
		projectID = p.Id
	}
	var doc *documentv1.DocumentVersion
	var e error
	if documentID := strings.TrimSpace(r.FormValue("documentId")); documentID != "" {
		doc, e = s.clients.Document.AddDocumentVersion(r.Context(), &documentv1.AddDocumentVersionRequest{DocumentId: documentID, FileName: header.Filename, MediaType: header.Header.Get("Content-Type"), Original: body})
	} else {
		doc, e = s.clients.Document.CreateDocument(r.Context(), &documentv1.CreateDocumentRequest{ProjectId: projectID, FileName: header.Filename, MediaType: header.Header.Get("Content-Type"), Original: body})
	}
	if e != nil {
		grpcProblem(w, e)
		return
	}
	s.progress(r, "analyzing")
	elements, e := s.clients.Document.AnalyzeDocumentVersion(r.Context(), &documentv1.AnalyzeDocumentVersionRequest{DocumentId: doc.DocumentId, Version: doc.Version, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	reqs, e := s.clients.Document.AnalyzeRequirements(r.Context(), &documentv1.AnalyzeDocumentVersionRequest{DocumentId: doc.DocumentId, Version: doc.Version, OutputLanguage: commonv1.OutputLanguage_OUTPUT_LANGUAGE_KOREAN})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	analysis, e := s.clients.Evaluation.IdentifyAssumptionsAndRisks(r.Context(), &evaluationv1.IdentifyAssumptionsAndRisksRequest{DocumentId: doc.DocumentId, DocumentVersion: doc.Version, Elements: elements.Elements, Dependencies: reqs.Dependencies})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	purpose := element(elements.Elements, documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_PURPOSE)
	s.progress(r, "planning_research")
	country := r.FormValue("country")
	domain := r.FormValue("domain")
	plan, e := s.clients.Evaluation.CreateValidationPlan(r.Context(), &evaluationv1.CreateValidationPlanRequest{DocumentId: doc.DocumentId, DocumentVersion: doc.Version, Purpose: purpose, Country: country, Domain: domain, Assumptions: analysis.Assumptions, Risks: analysis.Risks, Dependencies: reqs.Dependencies})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	var pe *evaluationv1.PurposeAlignmentEvaluation
	var ce *evaluationv1.ContradictionEvaluation
	var rc *evaluationv1.RequirementCompletenessEvaluation
	var qe *evaluationv1.DocumentWorkQualityEvaluation
	var peErr, ceErr, rcErr, qeErr error
	var evaluations sync.WaitGroup
	evaluations.Add(4)
	go func() {
		defer evaluations.Done()
		pe, peErr = s.clients.Evaluation.EvaluatePurposeAlignment(r.Context(), &evaluationv1.EvaluatePurposeAlignmentRequest{DocumentId: doc.DocumentId, DocumentVersion: doc.Version, Elements: elements.Elements})
	}()
	go func() {
		defer evaluations.Done()
		ce, ceErr = s.clients.Evaluation.DetectContradictions(r.Context(), &evaluationv1.DetectContradictionsRequest{DocumentId: doc.DocumentId, DocumentVersion: doc.Version, Elements: elements.Elements, Requirements: reqs.Requirements})
	}()
	go func() {
		defer evaluations.Done()
		rc, rcErr = s.clients.Evaluation.EvaluateRequirementCompleteness(r.Context(), &evaluationv1.EvaluateRequirementCompletenessRequest{DocumentId: doc.DocumentId, DocumentVersion: doc.Version, Requirements: reqs.Requirements, Dependencies: reqs.Dependencies})
	}()
	go func() {
		defer evaluations.Done()
		qe, qeErr = s.clients.Evaluation.EvaluateDocumentWorkQuality(r.Context(), &evaluationv1.EvaluateDocumentWorkQualityRequest{DocumentId: doc.DocumentId, DocumentVersion: doc.Version, Elements: elements.Elements})
	}()
	evaluations.Wait()
	for _, evaluationErr := range []error{peErr, ceErr, rcErr, qeErr} {
		if evaluationErr != nil {
			grpcProblem(w, evaluationErr)
			return
		}
	}
	rAssumptions := make([]*researchv1.Assumption, 0, len(analysis.Assumptions))
	for _, a := range analysis.Assumptions {
		rAssumptions = append(rAssumptions, &researchv1.Assumption{Id: a.Id, Statement: a.Statement, FailureImpact: a.FailureImpact, BlockingLikelihood: confidence(a.Criticality == evaluationv1.AssumptionCriticality_ASSUMPTION_CRITICALITY_CRITICAL)})
	}
	rp, e := s.clients.Research.PlanResearch(r.Context(), &researchv1.PlanResearchRequest{EvaluationId: analysis.Id, TargetCountry: country, TargetDomain: domain, Assumptions: rAssumptions, Limit: &researchv1.ResearchLimit{MaxQueries: 12, MaxSources: 20, MaxSeconds: 180, SaturationQueries: 3}})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	s.progress(r, "researching")
	rr, e := s.clients.Research.ExecuteResearch(r.Context(), &researchv1.ExecuteResearchRequest{Plan: rp})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	s.progress(r, "reviewing")
	findings := findingInputs(pe, ce, rc, qe)
	areas := make([]*generationv1.AreaResult, 0, len(plan.Items))
	validationResults := make([]*evaluationv1.ValidationResultInput, 0, len(plan.Items))
	for _, i := range plan.Items {
		statusValue := evaluationv1.ValidationStatus_VALIDATION_STATUS_UNVERIFIED
		if !i.Selected {
			statusValue = evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_APPLICABLE
		}
		switch i.Area {
		case evaluationv1.ValidationArea_VALIDATION_AREA_PURPOSE:
			statusValue = pe.GetStatus()
		case evaluationv1.ValidationArea_VALIDATION_AREA_CONSISTENCY:
			statusValue = ce.GetStatus()
		case evaluationv1.ValidationArea_VALIDATION_AREA_COMPLETENESS:
			statusValue = rc.GetStatus()
		}
		statusText := strings.ToLower(strings.TrimPrefix(statusValue.String(), "VALIDATION_STATUS_"))
		areas = append(areas, &generationv1.AreaResult{Area: i.Area.String(), Status: statusText, Reason: i.Reason})
		validationResults = append(validationResults, &evaluationv1.ValidationResultInput{Area: i.Area, Status: statusValue, Reason: i.Reason})
	}
	if _, e = s.clients.Evaluation.RecordValidationResults(r.Context(), &evaluationv1.RecordValidationResultsRequest{ValidationPlanId: plan.GetId(), Results: validationResults}); e != nil {
		grpcProblem(w, e)
		return
	}
	researchResults := make([]*generationv1.ResearchResult, 0, len(rr.Results))
	evidence := map[string]*researchv1.SourceEvidence{}
	for _, v := range rr.Evidence {
		evidence[v.Id] = v
	}
	for _, v := range rr.Results {
		x := &generationv1.ResearchResult{Question: v.VerificationType, Conclusion: v.Conclusion, Uncertainties: v.RemainingUncertainties}
		for _, id := range v.EvidenceIds {
			if ev := evidence[id]; ev != nil {
				x.Sources = append(x.Sources, &generationv1.EvidenceLink{Id: ev.Id, Title: ev.Title, Url: ev.Url, SourceLocation: ev.SourceLocation, Grade: ev.ReliabilityGrade, Relation: ev.Relation})
			}
		}
		researchResults = append(researchResults, x)
	}
	assumptions := make([]*generationv1.AssumptionResult, 0, len(analysis.GetAssumptions()))
	for _, assumption := range analysis.GetAssumptions() {
		assumptions = append(assumptions, &generationv1.AssumptionResult{Id: assumption.GetId(), Statement: assumption.GetStatement(), Status: "unverified"})
	}
	contradictions := make([]*generationv1.Contradiction, 0, len(ce.GetContradictions()))
	for _, contradiction := range ce.GetContradictions() {
		finding := contradiction.GetFinding()
		firstLocation, secondLocation := "", ""
		if len(finding.GetSourceLocations()) > 0 {
			firstLocation = sourceLocation(finding.GetSourceLocations()[0])
		}
		if len(finding.GetSourceLocations()) > 1 {
			secondLocation = sourceLocation(finding.GetSourceLocations()[1])
		}
		contradictions = append(contradictions, &generationv1.Contradiction{Id: finding.GetId(), FirstLocation: firstLocation, FirstStatement: contradiction.GetFirstStatement(), SecondLocation: secondLocation, SecondStatement: contradiction.GetSecondStatement(), Reason: finding.GetReasoningSummary()})
	}
	progress := []*generationv1.ProgressStep{{Code: "document", Label: "문서 구조화", Status: "completed"}, {Code: "internal", Label: "내부 평가", Status: "completed"}, {Code: "research", Label: "외부 조사와 원출처 확인", Status: rr.GetStatus()}, {Code: "report", Label: "판정과 리포트", Status: "completed"}}
	report, e := s.clients.Generation.CreateReport(r.Context(), &generationv1.CreateReportRequest{EvaluationId: analysis.Id, ProjectId: projectID, ProjectName: valueOr(r.FormValue("projectName"), projectID), OwnerSubject: subject(r), DocumentId: doc.DocumentId, DocumentName: header.Filename, DocumentVersion: doc.Version, Country: country, Domain: domain, TargetUser: r.FormValue("targetUser"), CriteriaVersion: "1.0", Model: elements.Model, Summary: &generationv1.PlanSummary{Purpose: purpose, Problem: element(elements.Elements, documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_PROBLEM), TargetUser: element(elements.Elements, documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_TARGET), Solution: element(elements.Elements, documentv1.DocumentElementType_DOCUMENT_ELEMENT_TYPE_SOLUTION)}, Findings: findings, Areas: areas, Assumptions: assumptions, Contradictions: contradictions, Research: researchResults, Limitations: rr.RemainingUncertainties, Progress: progress, EvaluationStatus: generationv1.EvaluationStatus_EVALUATION_STATUS_COMPLETED})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	write(w, report)
}
func findingInputs(p *evaluationv1.PurposeAlignmentEvaluation, c *evaluationv1.ContradictionEvaluation, g *evaluationv1.RequirementCompletenessEvaluation, q *evaluationv1.DocumentWorkQualityEvaluation) []*generationv1.FindingInput {
	out := []*generationv1.FindingInput{}
	add := func(f *evaluationv1.Finding) {
		if f == nil {
			return
		}
		loc := ""
		if len(f.SourceLocations) > 0 {
			loc = fmt.Sprintf("page:%d paragraph:%d", f.SourceLocations[0].PageNumber, f.SourceLocations[0].ParagraphNumber)
		}
		findingStatus := "open"
		if loc == "" {
			findingStatus = "unverified"
		}
		out = append(out, &generationv1.FindingInput{Id: f.Id, Area: f.Area.String(), ProblemType: f.Type.String(), SourceLocation: loc, Statement: f.Statement, Finding: f.Finding, ReasoningSummary: f.ReasoningSummary, Impact: f.Impact, Likelihood: f.Confidence, ImpactScore: severityScore(f.Severity), RequiredAction: f.RequiredAction, Status: findingStatus, CriticalAssumption: f.Severity == evaluationv1.Severity_SEVERITY_BLOCKING})
	}
	for _, f := range p.Findings {
		add(f)
	}
	for _, x := range c.Contradictions {
		add(x.Finding)
	}
	for _, x := range g.Gaps {
		add(x.Finding)
	}
	for _, item := range q.GetResults() {
		if item.GetStatus() == evaluationv1.ValidationStatus_VALIDATION_STATUS_SATISFIED {
			continue
		}
		location := ""
		if len(item.GetSourceLocations()) > 0 {
			location = sourceLocation(item.GetSourceLocations()[0])
		}
		severity := .25
		if item.GetStatus() == evaluationv1.ValidationStatus_VALIDATION_STATUS_NOT_SATISFIED {
			severity = .5
		}
		out = append(out, &generationv1.FindingInput{Id: "quality-" + strings.ToLower(strings.TrimPrefix(item.GetDimension().String(), "WORK_QUALITY_DIMENSION_")), Area: "work_quality", ProblemType: "document_work_quality", SourceLocation: location, Statement: item.GetFinding(), Finding: item.GetFinding(), ReasoningSummary: item.GetReasoningSummary(), Impact: "기획서의 검증 가능성과 의사결정 신뢰도를 낮춥니다.", Likelihood: item.GetConfidence(), ImpactScore: severity, RequiredAction: "문서 안에 확인 가능한 조사·검증·논리 근거를 보완합니다.", Status: "open"})
	}
	return out
}

func sourceLocation(value *documentv1.SourceLocation) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("page:%d slide:%d sheet:%s paragraph:%d section:%s", value.GetPageNumber(), value.GetSlideNumber(), value.GetSheetName(), value.GetParagraphNumber(), strings.Join(value.GetSectionPath(), "/"))
}

func severityScore(value evaluationv1.Severity) float64 {
	switch value {
	case evaluationv1.Severity_SEVERITY_BLOCKING:
		return 1
	case evaluationv1.Severity_SEVERITY_HIGH:
		return .75
	case evaluationv1.Severity_SEVERITY_MEDIUM:
		return .5
	case evaluationv1.Severity_SEVERITY_LOW:
		return .25
	default:
		return 0
	}
}
func (s *Service) getReport(w http.ResponseWriter, r *http.Request) {
	v, e := s.clients.Generation.GetReport(r.Context(), &generationv1.GetReportRequest{ReportId: r.PathValue("id")})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	if !s.authorizeReport(w, r, v) {
		return
	}
	write(w, v)
}
func (s *Service) exportReport(w http.ResponseWriter, r *http.Request) {
	report, e := s.clients.Generation.GetReport(r.Context(), &generationv1.GetReportRequest{ReportId: r.PathValue("id")})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	if !s.authorizeReport(w, r, report) {
		return
	}
	formats := map[string]generationv1.ExportFormat{"markdown": generationv1.ExportFormat_EXPORT_FORMAT_MARKDOWN, "pdf": generationv1.ExportFormat_EXPORT_FORMAT_PDF, "docx": generationv1.ExportFormat_EXPORT_FORMAT_DOCX, "json": generationv1.ExportFormat_EXPORT_FORMAT_JSON}
	v, e := s.clients.Generation.ExportReport(r.Context(), &generationv1.ExportReportRequest{ReportId: r.PathValue("id"), Format: formats[r.PathValue("format")]})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	w.Header().Set("Content-Type", v.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+v.Filename+`"`)
	_, _ = w.Write(v.Content)
}
func (s *Service) discoverPlanningQuestions(w http.ResponseWriter, r *http.Request) {
	req := &generationv1.DiscoverPlanningQuestionsRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	v, e := s.clients.Generation.DiscoverPlanningQuestions(r.Context(), req)
	if e != nil {
		grpcProblem(w, e)
		return
	}
	write(w, v)
}
func (s *Service) generateAutonomousPlan(w http.ResponseWriter, r *http.Request) {
	req := &generationv1.GenerateAutonomousPlanRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	req.OwnerSubject = subject(r)
	if len(req.Research) == 0 {
		environment := ""
		if req.Context != nil {
			environment = req.Context.Environment
		}
		rp, e := s.clients.Research.PlanResearch(r.Context(), &researchv1.PlanResearchRequest{EvaluationId: "autonomous-" + fmt.Sprint(time.Now().UnixNano()), TargetCountry: environment, Assumptions: []*researchv1.Assumption{{Id: "idea", Statement: req.Idea, FailureImpact: "기획의 핵심 전제가 성립하지 않음", BlockingLikelihood: 1}}, Limit: &researchv1.ResearchLimit{MaxQueries: 8, MaxSources: 12, MaxSeconds: 120, SaturationQueries: 3}})
		if e != nil {
			grpcProblem(w, e)
			return
		}
		rr, e := s.clients.Research.ExecuteResearch(r.Context(), &researchv1.ExecuteResearchRequest{Plan: rp})
		if e != nil {
			grpcProblem(w, e)
			return
		}
		evidence := map[string]*researchv1.SourceEvidence{}
		for _, item := range rr.Evidence {
			evidence[item.Id] = item
		}
		for _, item := range rr.Results {
			fact := &generationv1.ResearchFact{Id: item.TaskId, Question: item.VerificationType, Conclusion: item.Conclusion, Verified: item.Status == "verified", Uncertainties: item.RemainingUncertainties}
			for _, id := range item.EvidenceIds {
				if source := evidence[id]; source != nil {
					fact.Evidence = append(fact.Evidence, &generationv1.EvidenceLink{Id: source.Id, Title: source.Title, Url: source.Url, SourceLocation: source.SourceLocation, Grade: source.ReliabilityGrade, Relation: source.Relation})
				}
			}
			req.Research = append(req.Research, fact)
		}
	}
	if len(req.Alternatives) == 0 {
		ids := []string{}
		for _, fact := range req.Research {
			for _, e := range fact.Evidence {
				ids = append(ids, e.Id)
			}
		}
		req.Alternatives = []*generationv1.SolutionAlternative{{Id: "evidence-based", Name: "근거 기반 실행안", Description: "검증된 근거와 제약을 반영한 실행안", EvidenceIds: ids, Selected: true, DecisionReason: "확인 가능한 근거를 가장 직접 반영"}, {Id: "defer", Name: "추가 조사 후 결정", Description: "핵심 전제가 부족하면 구현을 보류", Selected: false, DecisionReason: "현재 근거로 실행 가능한 경우 우선안 선택"}}
	}
	v, e := s.clients.Generation.GenerateAutonomousPlan(r.Context(), req)
	if e != nil {
		grpcProblem(w, e)
		return
	}
	write(w, v)
}
func (s *Service) getAutonomousPlan(w http.ResponseWriter, r *http.Request) {
	v, e := s.clients.Generation.GetAutonomousPlan(r.Context(), &generationv1.GetAutonomousPlanRequest{PlanId: r.PathValue("id")})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	if v.GetOwnerSubject() != subject(r) {
		problem(w, http.StatusNotFound, "plan_not_found", "자율 기획을 찾을 수 없습니다.")
		return
	}
	write(w, v)
}
func (s *Service) generateProductDesign(w http.ResponseWriter, r *http.Request) {
	req := &generationv1.GenerateProductDesignRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	plan, e := s.clients.Generation.GetAutonomousPlan(r.Context(), &generationv1.GetAutonomousPlanRequest{PlanId: req.GetPlanId()})
	if e != nil || plan.GetOwnerSubject() != subject(r) {
		problem(w, http.StatusNotFound, "plan_not_found", "자율 기획을 찾을 수 없습니다.")
		return
	}
	req.OwnerSubject = subject(r)
	v, e := s.clients.Generation.GenerateProductDesign(r.Context(), req)
	if e != nil {
		grpcProblem(w, e)
		return
	}
	write(w, v)
}
func (s *Service) getProductDesign(w http.ResponseWriter, r *http.Request) {
	v, e := s.clients.Generation.GetProductDesign(r.Context(), &generationv1.GetProductDesignRequest{DesignId: r.PathValue("id")})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	if v.GetOwnerSubject() != subject(r) {
		problem(w, http.StatusNotFound, "design_not_found", "제품 설계를 찾을 수 없습니다.")
		return
	}
	write(w, v)
}

func (s *Service) analyzeRepository(w http.ResponseWriter, r *http.Request) {
	req := &generationv1.AnalyzeRepositoryRequest{}
	if !decodeProto(w, r, req) || !s.authorizeProject(w, r, req.GetProjectId()) {
		return
	}
	value, err := s.clients.Generation.AnalyzeRepository(r.Context(), req)
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) compareImplementation(w http.ResponseWriter, r *http.Request) {
	req := &generationv1.CompareImplementationRequest{}
	if !decodeProto(w, r, req) || req.GetRepository() == nil || !s.authorizeProject(w, r, req.GetRepository().GetProjectId()) {
		return
	}
	value, err := s.clients.Generation.CompareImplementation(r.Context(), req)
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) createImplementationProposal(w http.ResponseWriter, r *http.Request) {
	req := &generationv1.CreateImplementationProposalRequest{}
	if !decodeProto(w, r, req) || req.GetReview() == nil || !s.authorizeProject(w, r, req.GetReview().GetProjectId()) {
		return
	}
	req.ActorId = subject(r)
	value, err := s.clients.Generation.CreateImplementationProposal(r.Context(), req)
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) listConnectors(w http.ResponseWriter, r *http.Request) {
	value, err := s.clients.Generation.ListConnectors(r.Context(), &generationv1.ListConnectorsRequest{})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) deliverIntegration(w http.ResponseWriter, r *http.Request) {
	req := &generationv1.DeliverIntegrationRequest{}
	if !decodeProto(w, r, req) || !s.authorizeProject(w, r, req.GetProjectId()) {
		return
	}
	req.ActorId = subject(r)
	value, err := s.clients.Generation.DeliverIntegration(r.Context(), req)
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}

func (s *Service) getImplementationHistory(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if !s.authorizeProject(w, r, projectID) {
		return
	}
	value, err := s.clients.Generation.GetImplementationHistory(r.Context(), &generationv1.GetImplementationHistoryRequest{ProjectId: projectID})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	write(w, value)
}
func (s *Service) submitObjection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FindingID   string
		Explanation string
		Evidence    []struct{ ID, Title, URLOrLocation, ApplicableScope, ContentHash string }
	}
	if !decode(w, r, &input) {
		return
	}
	report, e := s.clients.Generation.GetReport(r.Context(), &generationv1.GetReportRequest{ReportId: r.PathValue("id")})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	if !s.authorizeReport(w, r, report) {
		return
	}
	if len(input.Evidence) == 0 {
		problem(w, http.StatusBadRequest, "evidence_required", "이의 제기에는 검증할 원출처 URL이 필요합니다.")
		return
	}
	sources := make([]*researchv1.SourceReference, 0, len(input.Evidence))
	for _, v := range input.Evidence {
		sources = append(sources, &researchv1.SourceReference{Id: v.ID, Title: v.Title, Url: v.URLOrLocation, ApplicableScope: v.ApplicableScope})
	}
	verification, e := s.clients.Research.VerifySources(r.Context(), &researchv1.VerifySourcesRequest{EvaluationId: report.GetEvaluationId(), TargetCountry: report.GetCountry(), TargetDomain: report.GetDomain(), Sources: sources})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	if len(verification.GetEvidence()) == 0 {
		problem(w, http.StatusUnprocessableEntity, "evidence_unreachable", "제출한 원출처에 접근하지 못해 이의 제기를 접수하지 않았습니다.")
		return
	}
	evidence := make([]*evaluationv1.ObjectionEvidence, 0, len(verification.GetEvidence()))
	for _, v := range verification.GetEvidence() {
		evidence = append(evidence, &evaluationv1.ObjectionEvidence{Id: v.GetId(), Title: v.GetTitle(), UrlOrLocation: valueOr(v.GetOriginalUrl(), v.GetUrl()), ApplicableScope: valueOr(v.GetApplicableScope(), report.GetCountry()+" / "+report.GetDomain()), ContentHash: v.GetContentHash()})
	}
	previousFindings := reportFindingSnapshots(report)
	var target *evaluationv1.FindingSnapshot
	for _, finding := range previousFindings {
		if finding.GetId() == input.FindingID {
			target = finding
			break
		}
	}
	if target == nil {
		problem(w, http.StatusBadRequest, "finding_not_found", "이 리포트에 해당 Finding이 없습니다.")
		return
	}
	review, e := s.clients.Evaluation.ReviewObjectionEvidence(r.Context(), &evaluationv1.ReviewObjectionEvidenceRequest{Finding: target, Explanation: input.Explanation, Evidence: evidence})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	result, e := s.clients.Evaluation.SubmitObjection(r.Context(), &evaluationv1.SubmitObjectionRequest{EvaluationId: report.EvaluationId, FindingId: input.FindingID, Explanation: input.Explanation, Evidence: evidence, ResearchId: verification.GetId(), VerificationStatus: review.GetStatus(), RemainingUncertainties: verification.GetRemainingUncertainties()})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	currentFindings := make([]*evaluationv1.FindingSnapshot, 0, len(previousFindings))
	for _, finding := range previousFindings {
		if finding.GetId() == input.FindingID && review.GetStatus() == "contradicts_finding" && review.GetConfidence() >= .7 {
			continue
		}
		copy := proto.Clone(finding).(*evaluationv1.FindingSnapshot)
		if copy.GetId() == input.FindingID {
			copy.EvidenceIds = append(copy.EvidenceIds, review.GetEvidenceIds()...)
		}
		currentFindings = append(currentFindings, copy)
	}
	reevaluation, e := s.clients.Evaluation.Reevaluate(r.Context(), &evaluationv1.ReevaluateRequest{PreviousEvaluationId: report.GetEvaluationId(), CurrentEvaluationId: report.GetEvaluationId(), DocumentComparison: &evaluationv1.DocumentVersionComparison{DocumentId: report.GetDocumentId(), PreviousVersion: report.GetDocumentVersion(), CurrentVersion: report.GetDocumentVersion(), AffectedValidationAreas: []string{target.GetArea()}, AffectedFindingIds: []string{target.GetId()}, ResearchFirstIds: []string{verification.GetId()}}, PreviousFindings: previousFindings, CurrentFindings: currentFindings, PreviousVerdict: verdictName(report.GetVerdict()), CriteriaVersion: report.GetCriteriaVersion(), Model: report.GetModel(), ObjectionId: result.GetId(), ObjectionFindingId: target.GetId(), NewEvidenceIds: review.GetEvidenceIds()})
	if e != nil {
		grpcProblem(w, e)
		return
	}
	result.Status = "reevaluated"
	result.ResultingReevaluationId = reevaluation.GetId()
	write(w, result)
}

func (s *Service) reviseReport(w http.ResponseWriter, r *http.Request) {
	previous, err := s.clients.Generation.GetReport(r.Context(), &generationv1.GetReportRequest{ReportId: r.PathValue("id")})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	if !s.authorizeReport(w, r, previous) {
		return
	}
	if strings.TrimSpace(previous.GetDocumentId()) == "" {
		problem(w, http.StatusConflict, "document_identity_missing", "이 리포트에는 수정본을 연결할 문서 ID가 없습니다.")
		return
	}
	if err = r.ParseMultipartForm(32 << 20); err != nil {
		problem(w, http.StatusBadRequest, "invalid_upload", err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		problem(w, http.StatusBadRequest, "file_required", err.Error())
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_upload", err.Error())
		return
	}

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, strings.ReplaceAll(header.Filename, `"`, "")))
	partHeader.Set("Content-Type", valueOr(header.Header.Get("Content-Type"), "application/octet-stream"))
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		problem(w, http.StatusInternalServerError, "revision_failed", err.Error())
		return
	}
	if _, err = part.Write(body); err != nil {
		problem(w, http.StatusInternalServerError, "revision_failed", err.Error())
		return
	}
	fields := map[string]string{
		"documentId": previous.GetDocumentId(), "projectId": previous.GetProjectId(), "projectName": previous.GetProjectName(),
		"country": previous.GetCountry(), "domain": previous.GetDomain(), "targetUser": previous.GetTargetUser(),
	}
	for key, value := range fields {
		if err = writer.WriteField(key, value); err != nil {
			problem(w, http.StatusInternalServerError, "revision_failed", err.Error())
			return
		}
	}
	if err = writer.Close(); err != nil {
		problem(w, http.StatusInternalServerError, "revision_failed", err.Error())
		return
	}
	request := httptest.NewRequest(http.MethodPost, "/api/evaluations", &requestBody).WithContext(r.Context())
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	s.evaluate(recorder, request)
	if recorder.Code < 200 || recorder.Code >= 300 {
		w.Header().Set("Content-Type", recorder.Header().Get("Content-Type"))
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
		return
	}
	current := &generationv1.EvaluationReport{}
	if err = protojson.Unmarshal(recorder.Body.Bytes(), current); err != nil {
		problem(w, http.StatusInternalServerError, "revision_failed", "새 리포트 응답을 해석하지 못했습니다.")
		return
	}
	comparison, err := s.clients.Evaluation.CompareDocumentVersions(r.Context(), &evaluationv1.CompareDocumentVersionsRequest{DocumentId: previous.GetDocumentId(), PreviousVersion: previous.GetDocumentVersion(), CurrentVersion: current.GetDocumentVersion(), PreviousItems: reportVersionItems(previous), CurrentItems: reportVersionItems(current)})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	result, err := s.clients.Evaluation.Reevaluate(r.Context(), &evaluationv1.ReevaluateRequest{PreviousEvaluationId: previous.GetEvaluationId(), CurrentEvaluationId: current.GetEvaluationId(), DocumentComparison: comparison, PreviousFindings: reportFindingSnapshots(previous), CurrentFindings: reportFindingSnapshots(current), PreviousVerdict: verdictName(previous.GetVerdict()), CriteriaVersion: current.GetCriteriaVersion(), Model: current.GetModel()})
	if err != nil {
		grpcProblem(w, err)
		return
	}
	options := protojson.MarshalOptions{UseProtoNames: false, EmitUnpopulated: true}
	reportJSON, _ := options.Marshal(current)
	comparisonJSON, _ := options.Marshal(comparison)
	resultJSON, _ := options.Marshal(result)
	write(w, map[string]json.RawMessage{"report": reportJSON, "comparison": comparisonJSON, "reevaluation": resultJSON})
}

func reportVersionItems(report *generationv1.EvaluationReport) []*evaluationv1.VersionItem {
	items := make([]*evaluationv1.VersionItem, 0, len(report.GetFindings())+6)
	if summary := report.GetSummary(); summary != nil {
		for id, value := range map[string]string{"problem": summary.GetProblem(), "purpose": summary.GetPurpose(), "target": summary.GetTargetUser(), "environment": summary.GetEnvironment(), "solution": summary.GetSolution(), "goal": summary.GetSuccessCriteria()} {
			if strings.TrimSpace(value) != "" {
				items = append(items, &evaluationv1.VersionItem{Id: "summary-" + id, Kind: id, Content: value})
			}
		}
	}
	for _, finding := range report.GetFindings() {
		detail := finding.GetDetail()
		if detail == nil || detail.GetId() == "" {
			continue
		}
		items = append(items, &evaluationv1.VersionItem{Id: detail.GetId(), Kind: strings.ToLower(strings.TrimPrefix(detail.GetArea(), "VALIDATION_AREA_")), Content: detail.GetStatement() + "\n" + detail.GetFinding(), RelatedFindingIds: []string{detail.GetId()}})
	}
	return items
}

func reportFindingSnapshots(report *generationv1.EvaluationReport) []*evaluationv1.FindingSnapshot {
	items := make([]*evaluationv1.FindingSnapshot, 0, len(report.GetFindings()))
	for _, finding := range report.GetFindings() {
		detail := finding.GetDetail()
		if detail == nil {
			continue
		}
		evidenceIDs := make([]string, 0, len(detail.GetEvidence()))
		for _, evidence := range detail.GetEvidence() {
			evidenceIDs = append(evidenceIDs, evidence.GetId())
		}
		items = append(items, &evaluationv1.FindingSnapshot{Id: detail.GetId(), Area: detail.GetArea(), Statement: detail.GetStatement(), Finding: detail.GetFinding(), Severity: severityName(finding.GetSeverity()), Status: detail.GetStatus(), EvidenceIds: evidenceIDs, SourceLocation: detail.GetSourceLocation()})
	}
	return items
}

func verdictName(value generationv1.Verdict) string {
	return strings.ToLower(strings.TrimPrefix(value.String(), "VERDICT_"))
}

func severityName(value generationv1.Severity) string {
	return strings.ToLower(strings.TrimPrefix(value.String(), "SEVERITY_"))
}

func element(values []*documentv1.DocumentElement, t documentv1.DocumentElementType) string {
	for _, v := range values {
		if v.Type == t {
			return v.Content
		}
	}
	return ""
}
func confidence(v bool) float64 {
	if v {
		return 1
	}
	return .5
}
func subject(r *http.Request) string {
	if value, ok := r.Context().Value(subjectKey{}).(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return strings.TrimSpace(r.FormValue("ownerSubject"))
}
func (s *Service) authorizeReport(w http.ResponseWriter, r *http.Request, report *generationv1.EvaluationReport) bool {
	caller := subject(r)
	if report.GetProjectId() != "" {
		if _, err := s.clients.Project.GetProject(r.Context(), &projectv1.GetProjectRequest{ProjectId: report.GetProjectId()}); err != nil {
			problem(w, http.StatusNotFound, "report_not_found", "리포트를 찾을 수 없습니다.")
			return false
		}
		return true
	}
	if caller != "" && report.GetOwnerSubject() == caller {
		return true
	}
	problem(w, http.StatusNotFound, "report_not_found", "리포트를 찾을 수 없습니다.")
	return false
}
func (s *Service) authorizeProject(w http.ResponseWriter, r *http.Request, projectID string) bool {
	if strings.TrimSpace(projectID) == "" {
		problem(w, http.StatusBadRequest, "project_required", "프로젝트 ID가 필요합니다.")
		return false
	}
	if _, err := s.clients.Project.GetProject(r.Context(), &projectv1.GetProjectRequest{ProjectId: projectID}); err != nil {
		problem(w, http.StatusNotFound, "project_not_found", "프로젝트를 찾을 수 없습니다.")
		return false
	}
	return true
}
func (s *Service) progress(r *http.Request, stage string) {
	if s.queue != nil && r.Header.Get("X-Evaluation-Job-ID") != "" {
		_ = s.queue.Progress(r.Context(), r.Header.Get("X-Evaluation-Job-ID"), stage)
	}
}
func valueOr(v, f string) string {
	if strings.TrimSpace(v) == "" {
		return f
	}
	return v
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v); err != nil {
		problem(w, 400, "invalid_json", err.Error())
		return false
	}
	return true
}
func decodeProto(w http.ResponseWriter, r *http.Request, v proto.Message) bool {
	body, e := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if e != nil {
		problem(w, 400, "invalid_json", e.Error())
		return false
	}
	options := protojson.UnmarshalOptions{DiscardUnknown: false}
	if e = options.Unmarshal(body, v); e != nil {
		problem(w, 400, "invalid_json", e.Error())
		return false
	}
	return true
}
func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if p, ok := v.(proto.Message); ok {
		b, _ := protojson.MarshalOptions{UseProtoNames: false, EmitUnpopulated: true}.Marshal(p)
		_, _ = w.Write(b)
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "detail": detail, "status": status, "timestamp": time.Now().UTC()})
}
func grpcProblem(w http.ResponseWriter, e error) { problem(w, 502, "downstream_error", e.Error()) }
