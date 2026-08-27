package investigate

import (
	"context"
	"testing"

	researchv1 "noplanner/backend/proto/dist/golang/research/v1"
	"noplanner/backend/research/domains/investigation"
)

type fakeSearcher struct {
	hits  []investigation.SearchHit
	pages map[string]investigation.Page
}

func (f fakeSearcher) Search(string) ([]investigation.SearchHit, error) { return f.hits, nil }
func (f fakeSearcher) Fetch(rawURL string) (investigation.Page, error)  { return f.pages[rawURL], nil }

func testPlan() *researchv1.ResearchPlan {
	return &researchv1.ResearchPlan{Id: "plan-1", EvaluationId: "evaluation-1", TargetCountry: "KR", TargetDomain: "mobility", Limit: &researchv1.ResearchLimit{MaxQueries: 3, MaxSources: 2, MaxSeconds: 30, SaturationQueries: 2}, Tasks: []*researchv1.ResearchTask{{Id: "task-1", AssumptionId: "a-1", Question: "데이터 사용 가능?", Queries: []string{"official dataset terms"}, VerificationType: "public_data"}}}
}

func TestMissingSearchConfigCreatesLimitedUnverifiedReport(t *testing.T) {
	service := New(investigation.NewMemoryStore(), fakeSearcher{}, false, 0)
	report, err := service.ExecuteResearch(context.Background(), &researchv1.ExecuteResearchRequest{Plan: testPlan()})
	if err != nil {
		t.Fatal(err)
	}
	if report.GetStatus() != "limited" || report.GetResults()[0].GetStatus() != "unverified" || len(report.GetLimitReasons()) == 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestResearchRecordsQueryOriginalVisitHashAndSeparateDimensions(t *testing.T) {
	url := "https://data.go.kr/dataset/1/terms"
	searcher := fakeSearcher{hits: []investigation.SearchHit{{Title: "Official dataset terms", URL: url, Publisher: "data.go.kr"}}, pages: map[string]investigation.Page{url: {URL: url, Title: "Terms", Text: "무료. 상업적 이용 조건 및 출처 표시, 재배포 제한.", SourceLocation: "HTML body", ContentHash: "abc123"}}}
	service := New(investigation.NewMemoryStore(), searcher, true, 0)
	report, err := service.ExecuteResearch(context.Background(), &researchv1.ExecuteResearchRequest{Plan: testPlan()})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.GetSearchTraces()) != 1 || len(report.GetSearchTraces()[0].GetVisitedUrls()) != 1 {
		t.Fatalf("trace = %#v", report.GetSearchTraces())
	}
	evidence := report.GetEvidence()[0]
	if evidence.GetOriginalUrl() != url || evidence.GetContentHash() != "abc123" || evidence.GetCommercialUseStatus() == "available" {
		t.Fatalf("evidence = %#v", evidence)
	}
	if len(report.GetResults()[0].GetDimensionResults()) < 7 {
		t.Fatalf("dimensions = %#v", report.GetResults()[0].GetDimensionResults())
	}
}

func TestVerifySourcesVisitsSubmittedOriginalAndStoresHash(t *testing.T) {
	rawURL := "https://official.test/terms"
	service := New(investigation.NewMemoryStore(), fakeSearcher{pages: map[string]investigation.Page{rawURL: {URL: rawURL, Title: "Official terms", Text: "Commercial use terms", SourceLocation: "response body", ContentHash: "verified-hash"}}}, true, 0)
	report, err := service.VerifySources(context.Background(), &researchv1.VerifySourcesRequest{EvaluationId: "evaluation-1", TargetCountry: "KR", TargetDomain: "software", Sources: []*researchv1.SourceReference{{Id: "submitted-1", Title: "Terms", Url: rawURL, ApplicableScope: "KR"}}})
	if err != nil {
		t.Fatal(err)
	}
	if report.GetStatus() != "completed" || len(report.GetEvidence()) != 1 {
		t.Fatalf("report = %#v", report)
	}
	evidence := report.GetEvidence()[0]
	if evidence.GetOriginalUrl() != rawURL || evidence.GetContentHash() != "verified-hash" || evidence.GetApplicableScope() != "KR" {
		t.Fatalf("evidence = %#v", evidence)
	}
	if len(report.GetSearchTraces()) != 1 || report.GetSearchTraces()[0].GetVisitedUrls()[0] != rawURL {
		t.Fatalf("trace = %#v", report.GetSearchTraces())
	}
}

func TestQueryLimitLeavesRemainingTaskUnverified(t *testing.T) {
	plan := testPlan()
	plan.Limit.MaxQueries = 1
	plan.Tasks = append(plan.Tasks, &researchv1.ResearchTask{Id: "task-2", AssumptionId: "a-2", Question: "API?", Queries: []string{"api docs"}, VerificationType: "api"})
	service := New(investigation.NewMemoryStore(), fakeSearcher{}, true, 0)
	report, err := service.ExecuteResearch(context.Background(), &researchv1.ExecuteResearchRequest{Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if report.GetStatus() != "limited" || len(report.GetResults()) != 2 || report.GetResults()[1].GetStatus() != "unverified" {
		t.Fatalf("report = %#v", report)
	}
}

func TestCostLimitStopsResearchAndReportsUncertainty(t *testing.T) {
	plan := testPlan()
	plan.Limit.MaxCost = .5
	plan.Tasks[0].Queries = []string{"first", "second"}
	service := New(investigation.NewMemoryStore(), fakeSearcher{}, true, .5)
	report, err := service.ExecuteResearch(context.Background(), &researchv1.ExecuteResearchRequest{Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if report.GetStatus() != "limited" || report.GetEstimatedCost() != .5 || report.GetLimitReasons()[0] != "cost_limit_reached" {
		t.Fatalf("report = %#v", report)
	}
}
