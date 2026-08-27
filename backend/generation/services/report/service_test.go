package report

import (
	"archive/zip"
	"bytes"
	"context"
	domain "noplanner/backend/generation/domains/report"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	"testing"
)

func fullRequest() *generationv1.CreateReportRequest {
	return &generationv1.CreateReportRequest{EvaluationId: "eval-1", ProjectName: "NoPlanner", DocumentName: "plan.pdf", DocumentVersion: 1, Country: "KR", Domain: "mobility", TargetUser: "developer", CriteriaVersion: "1", Model: "test", Summary: &generationv1.PlanSummary{Problem: "problem", Purpose: "purpose", TargetUser: "user", Environment: "KR", Solution: "solution", SuccessCriteria: "metric"}, Findings: []*generationv1.FindingInput{{Id: "DAT-001", Area: "data", ProblemType: "unavailable", SourceDocument: "plan.pdf", SourceLocation: "p.3", Statement: "공공데이터 사용", Finding: "필드 부족", Evidence: []*generationv1.EvidenceLink{{Id: "E-1", Url: "https://data.go.kr", SourceLocation: "fields", Grade: "A"}}, ReasoningSummary: "원문 필드에 없음", Impact: "기능 불가", Likelihood: 1, ImpactScore: 1, RecoveryCost: 1, Reversibility: 0, RequiredAction: "대체 데이터 확보", VerificationMethod: "샘플 확인", Status: "not_satisfied", CriticalAssumption: true, ConfirmedFalse: true}}, Areas: []*generationv1.AreaResult{{Area: "DAT", Status: "not_satisfied", FindingCount: 1, Confidence: generationv1.Confidence_CONFIDENCE_HIGH}}, Assumptions: []*generationv1.AssumptionResult{{Id: "A-1", Statement: "data exists", Status: "false", EvidenceIds: []string{"E-1"}}}, Contradictions: []*generationv1.Contradiction{{Id: "C-1", FirstLocation: "p1", FirstStatement: "free", SecondLocation: "p2", SecondStatement: "paid", Reason: "충돌"}}, Research: []*generationv1.ResearchResult{{Question: "exists?", Conclusion: "no", Sources: []*generationv1.EvidenceLink{{Id: "E-1", Url: "https://data.go.kr"}}}}, Limitations: []string{"비공개 자료 미확인"}, Progress: []*generationv1.ProgressStep{{Code: "research", Label: "외부 조사", Status: "completed"}}, EvaluationStatus: generationv1.EvaluationStatus_EVALUATION_STATUS_COMPLETED}
}
func TestReportE2ECreateGetAndAllExports(t *testing.T) {
	s := New(domain.NewMemoryStore(), "generation", "1")
	created, err := s.CreateReport(context.Background(), fullRequest())
	if err != nil {
		t.Fatal(err)
	}
	if created.GetVerdict() != generationv1.Verdict_VERDICT_NOT_EXECUTABLE || len(created.GetFindings()) != 1 || len(created.GetAreas()) != 1 || len(created.GetResearch()) != 1 {
		t.Fatalf("missing report sections: %#v", created)
	}
	got, err := s.GetReport(context.Background(), &generationv1.GetReportRequest{ReportId: created.GetId()})
	if err != nil || got.GetId() != created.GetId() {
		t.Fatal(err)
	}
	for _, format := range []generationv1.ExportFormat{generationv1.ExportFormat_EXPORT_FORMAT_MARKDOWN, generationv1.ExportFormat_EXPORT_FORMAT_PDF, generationv1.ExportFormat_EXPORT_FORMAT_DOCX, generationv1.ExportFormat_EXPORT_FORMAT_JSON} {
		exported, err := s.ExportReport(context.Background(), &generationv1.ExportReportRequest{ReportId: created.GetId(), Format: format})
		if err != nil || len(exported.GetContent()) == 0 {
			t.Fatalf("format %s: %v", format, err)
		}
		if format == generationv1.ExportFormat_EXPORT_FORMAT_DOCX {
			if _, err := zip.NewReader(bytes.NewReader(exported.GetContent()), int64(len(exported.GetContent()))); err != nil {
				t.Fatalf("invalid docx: %v", err)
			}
		}
	}
}
