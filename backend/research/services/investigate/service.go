package investigate

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	researchv1 "noplanner/backend/proto/dist/golang/research/v1"
	"noplanner/backend/research/domains/investigation"
	"noplanner/backend/research/modules/rpcerror"
	webclient "noplanner/backend/research/modules/web"
)

type Service struct {
	researchv1.UnimplementedResearchServiceServer
	store            investigation.Store
	searcher         investigation.Searcher
	searchConfigured bool
	queryCost        float64
	name, version    string
}

func New(store investigation.Store, searcher investigation.Searcher, searchConfigured bool, queryCost float64, identity ...string) *Service {
	service := &Service{store: store, searcher: searcher, searchConfigured: searchConfigured, queryCost: queryCost}
	if len(identity) > 0 {
		service.name = identity[0]
	}
	if len(identity) > 1 {
		service.version = identity[1]
	}
	return service
}

func (s *Service) GetStatus(context.Context, *commonv1.StatusRequest) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{Service: s.name, Version: s.version, State: "ready"}, nil
}

func (s *Service) PlanResearch(ctx context.Context, request *researchv1.PlanResearchRequest) (*researchv1.ResearchPlan, error) {
	assumptions := make([]investigation.Assumption, 0, len(request.GetAssumptions()))
	for _, item := range request.GetAssumptions() {
		assumptions = append(assumptions, investigation.Assumption{ID: item.GetId(), Statement: item.GetStatement(), FailureImpact: item.GetFailureImpact(), BlockingLikelihood: item.GetBlockingLikelihood(), DependencyTypes: item.GetDependencyTypes()})
	}
	plan, err := investigation.BuildPlan(request.GetEvaluationId(), request.GetTargetCountry(), request.GetTargetDomain(), assumptions, fromProtoLimit(request.GetLimit()))
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "조사 계획 입력이 올바르지 않습니다.", false)
	}
	plan, err = s.store.SavePlan(ctx, plan)
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "조사 계획을 저장하지 못했습니다.", true)
	}
	return toProtoPlan(plan), nil
}

func (s *Service) ExecuteResearch(ctx context.Context, request *researchv1.ExecuteResearchRequest) (*researchv1.ResearchReport, error) {
	plan, err := fromProtoPlan(request.GetPlan())
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "조사 계획이 올바르지 않습니다.", false)
	}
	report := investigation.Report{ID: investigation.NewID(), PlanID: plan.ID, EvaluationID: plan.EvaluationID, Status: "completed"}
	started := time.Now()
	if !s.searchConfigured {
		report.Status = "limited"
		report.LimitReasons = append(report.LimitReasons, "google_search_configuration_missing")
		for _, task := range plan.Tasks {
			report.Results = append(report.Results, unverified(task, "검색 설정이 없어 외부 사실을 검증하지 못했습니다."))
		}
		return s.save(ctx, report)
	}
	staleQueries := uint32(0)
	for _, task := range plan.Tasks {
		result := unverified(task, "원출처에서 판정을 확정할 충분한 근거가 없습니다.")
		before := len(report.Evidence)
		for _, query := range task.Queries {
			if reason := limitReached(plan.Limit, report, started); reason != "" {
				report.LimitReasons = appendUnique(report.LimitReasons, reason)
				break
			}
			hits, searchErr := s.searcher.Search(query)
			report.QueriesUsed++
			report.EstimatedCost += s.queryCost
			trace := investigation.SearchTrace{Query: query}
			if searchErr != nil {
				if errors.Is(searchErr, webclient.ErrNotConfigured) {
					report.LimitReasons = appendUnique(report.LimitReasons, "google_search_configuration_missing")
				}
				report.SearchTraces = append(report.SearchTraces, trace)
				continue
			}
			for _, hit := range hits {
				trace.ResultURLs = append(trace.ResultURLs, hit.URL)
			}
			for _, hit := range prioritize(hits) {
				if report.SourcesUsed >= plan.Limit.MaxSources {
					report.LimitReasons = appendUnique(report.LimitReasons, "source_limit_reached")
					break
				}
				page, fetchErr := s.searcher.Fetch(hit.URL)
				if fetchErr != nil {
					continue
				}
				trace.VisitedURLs = append(trace.VisitedURLs, page.URL)
				report.SourcesUsed++
				evidence := buildEvidence(plan, task, hit, page)
				report.Evidence = append(report.Evidence, evidence)
				result.EvidenceIDs = append(result.EvidenceIDs, evidence.ID)
				if evidence.Relation == "contradicts" {
					result.ConflictingEvidenceIDs = append(result.ConflictingEvidenceIDs, evidence.ID)
				}
			}
			report.SearchTraces = append(report.SearchTraces, trace)
			if len(report.Evidence) == before {
				staleQueries++
			} else {
				staleQueries = 0
			}
			if staleQueries >= plan.Limit.SaturationQueries {
				report.LimitReasons = appendUnique(report.LimitReasons, "research_saturated")
				break
			}
		}
		result.DimensionResults = dimensions(task.VerificationType, report.Evidence[before:])
		if hasPrimaryEvidence(report.Evidence[before:]) {
			result.Status = "partially_verified"
			result.Conclusion = "원출처를 확인했으나 모든 판정 축과 교차검증이 완료되지는 않았습니다."
		}
		report.Results = append(report.Results, result)
		if len(report.LimitReasons) > 0 {
			break
		}
	}
	for len(report.Results) < len(plan.Tasks) {
		task := plan.Tasks[len(report.Results)]
		report.Results = append(report.Results, unverified(task, "조사 한도 도달로 확인하지 못했습니다."))
	}
	if len(report.LimitReasons) > 0 {
		report.Status = "limited"
	}
	report.ElapsedSeconds = uint32(time.Since(started).Seconds())
	for _, result := range report.Results {
		report.RemainingUncertainties = append(report.RemainingUncertainties, result.RemainingUncertainties...)
	}
	return s.save(ctx, report)
}

func (s *Service) VerifySources(ctx context.Context, request *researchv1.VerifySourcesRequest) (*researchv1.ResearchReport, error) {
	if strings.TrimSpace(request.GetEvaluationId()) == "" || len(request.GetSources()) == 0 {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "검증할 원출처가 필요합니다.", false)
	}
	plan := investigation.Plan{ID: investigation.NewID(), EvaluationID: request.GetEvaluationId(), TargetCountry: request.GetTargetCountry(), TargetDomain: request.GetTargetDomain()}
	for index, source := range request.GetSources() {
		plan.Tasks = append(plan.Tasks, investigation.Task{ID: first(source.GetId(), fmt.Sprintf("source-%d", index+1)), AssumptionID: first(source.GetId(), fmt.Sprintf("source-%d", index+1)), Question: source.GetUrl(), VerificationType: "submitted_evidence"})
	}
	savedPlan, err := s.store.SavePlan(ctx, plan)
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "직접 원출처 검증 계획을 저장하지 못했습니다.", true)
	}
	plan = savedPlan
	report := investigation.Report{ID: investigation.NewID(), PlanID: plan.ID, EvaluationID: request.GetEvaluationId(), Status: "completed"}
	if !s.searchConfigured {
		report.Status = "limited"
		report.LimitReasons = []string{"google_search_configuration_missing"}
		report.RemainingUncertainties = []string{"제출된 추가 근거의 원문에 접근하지 못했습니다."}
		return s.save(ctx, report)
	}
	for index, source := range request.GetSources() {
		rawURL := strings.TrimSpace(source.GetUrl())
		parsed, err := url.ParseRequestURI(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "검증 가능한 HTTP 원출처 URL이 필요합니다.", false)
		}
		page, err := s.searcher.Fetch(rawURL)
		if err != nil {
			report.RemainingUncertainties = append(report.RemainingUncertainties, rawURL+": 원문 접근 실패")
			continue
		}
		task := plan.Tasks[index]
		hit := investigation.SearchHit{Title: source.GetTitle(), URL: rawURL}
		evidence := buildEvidence(plan, task, hit, page)
		if scope := strings.TrimSpace(source.GetApplicableScope()); scope != "" {
			evidence.ApplicableScope = scope
		}
		report.Evidence = append(report.Evidence, evidence)
		report.SearchTraces = append(report.SearchTraces, investigation.SearchTrace{Query: "direct-source:" + rawURL, ResultURLs: []string{rawURL}, VisitedURLs: []string{page.URL}})
		report.Results = append(report.Results, investigation.Result{TaskID: task.ID, AssumptionID: task.AssumptionID, VerificationType: "submitted_evidence", Status: "partially_verified", Conclusion: "제출된 URL의 원문 접근과 내용 해시 생성을 완료했습니다. 주장 적용 여부는 재평가에서 판정합니다.", EvidenceIDs: []string{evidence.ID}, DimensionResults: []string{"source_access=confirmed", "content_hash=confirmed"}})
		report.SourcesUsed++
	}
	if len(report.Evidence) == 0 {
		report.Status = "limited"
		if len(report.RemainingUncertainties) == 0 {
			report.RemainingUncertainties = []string{"제출된 원출처를 확인하지 못했습니다."}
		}
	}
	return s.save(ctx, report)
}

func (s *Service) GetResearch(ctx context.Context, request *researchv1.GetResearchRequest) (*researchv1.ResearchReport, error) {
	if strings.TrimSpace(request.GetResearchId()) == "" {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "조사 ID가 필요합니다.", false)
	}
	report, err := s.store.GetReport(ctx, request.GetResearchId())
	if errors.Is(err, investigation.ErrNotFound) {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_NOT_FOUND, "조사 결과를 찾을 수 없습니다.", false)
	}
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "조사 결과를 불러오지 못했습니다.", true)
	}
	return toProtoReport(report), nil
}

func (s *Service) save(ctx context.Context, report investigation.Report) (*researchv1.ResearchReport, error) {
	saved, err := s.store.SaveReport(ctx, report)
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "조사 결과를 저장하지 못했습니다.", true)
	}
	return toProtoReport(saved), nil
}

func unverified(task investigation.Task, reason string) investigation.Result {
	return investigation.Result{TaskID: task.ID, AssumptionID: task.AssumptionID, VerificationType: task.VerificationType, Status: "unverified", Conclusion: reason, DimensionResults: investigation.DimensionDefaults(task.VerificationType), RemainingUncertainties: []string{task.Question}}
}

func limitReached(limit investigation.Limit, report investigation.Report, started time.Time) string {
	if report.QueriesUsed >= limit.MaxQueries {
		return "query_limit_reached"
	}
	if report.SourcesUsed >= limit.MaxSources {
		return "source_limit_reached"
	}
	if time.Since(started) >= time.Duration(limit.MaxSeconds)*time.Second {
		return "time_limit_reached"
	}
	if limit.MaxCost > 0 && report.EstimatedCost >= limit.MaxCost {
		return "cost_limit_reached"
	}
	return ""
}

func prioritize(hits []investigation.SearchHit) []investigation.SearchHit {
	out := append([]investigation.SearchHit(nil), hits...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			oi := investigation.IsGovernmentURL(out[i].URL) || strings.Contains(strings.ToLower(out[i].URL), "docs")
			oj := investigation.IsGovernmentURL(out[j].URL) || strings.Contains(strings.ToLower(out[j].URL), "docs")
			if oj && !oi {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func buildEvidence(plan investigation.Plan, task investigation.Task, hit investigation.SearchHit, page investigation.Page) investigation.Evidence {
	sourceType := inferSourceType(task, page.URL, page.Text, page.ContentType)
	grade, official := investigation.Grade(page.URL, sourceType)
	relation := "supports_or_context"
	lower := strings.ToLower(page.Text)
	if strings.Contains(lower, "not available") || strings.Contains(lower, "지원하지 않") || strings.Contains(lower, "폐지") {
		relation = "contradicts"
	}
	publisher := hit.Publisher
	if u, err := url.Parse(page.URL); err == nil && publisher == "" {
		publisher = u.Hostname()
	}
	excerpt := page.Text
	if len(excerpt) > 1200 {
		excerpt = excerpt[:1200]
	}
	return investigation.Evidence{ID: investigation.NewID(), TaskID: task.ID, Title: first(page.Title, hit.Title), Publisher: publisher, SourceType: sourceType, URL: hit.URL, OriginalURL: page.URL, VerifiedAt: time.Now().UTC().Format(time.RFC3339), TargetCountry: plan.TargetCountry, TargetDomain: plan.TargetDomain, ApplicableScope: fmt.Sprintf("%s / %s", plan.TargetCountry, plan.TargetDomain), SourceLocation: page.SourceLocation, Excerpt: excerpt, Relation: relation, ReliabilityGrade: grade, AccessStatus: "accessible", PricingStatus: detectPricing(lower), CommercialUseStatus: detectCommercialUse(lower), LicenseOrTermsURL: termsURL(page.URL, lower), UsageRestrictions: detectRestrictions(lower), Limitations: []string{"자동 수집 결과이며 문맥·약관의 전문 검토가 추가로 필요할 수 있음"}, ContentHash: page.ContentHash, OfficialSource: official, SampleCallVerified: strings.Contains(sourceType, "api_response")}
}

func inferSourceType(task investigation.Task, rawURL, text, contentType string) string {
	lower := strings.ToLower(rawURL + " " + text)
	if (task.VerificationType == "api" || task.VerificationType == "technology") && strings.Contains(strings.ToLower(contentType), "application/json") {
		return "api_response"
	}
	if strings.Contains(lower, "terms") || strings.Contains(lower, "license") || strings.Contains(lower, "이용약관") {
		return "license_terms"
	}
	if strings.Contains(lower, "pricing") || strings.Contains(lower, "가격") {
		return "pricing"
	}
	if task.VerificationType == "public_data" {
		return "official_dataset"
	}
	if task.VerificationType == "paper" || task.VerificationType == "statistic" || task.VerificationType == "quantitative_claim" {
		return "original_paper_or_statistics"
	}
	if task.VerificationType == "api" || task.VerificationType == "technology" {
		return "official_documentation"
	}
	return "official_primary_source"
}
func detectPricing(text string) string {
	if strings.Contains(text, "free") || strings.Contains(text, "무료") {
		return "free_or_free_tier_found"
	}
	if strings.Contains(text, "pricing") || strings.Contains(text, "유료") || strings.Contains(text, "price") {
		return "paid_or_pricing_found"
	}
	return "unverified"
}
func detectCommercialUse(text string) string {
	if strings.Contains(text, "commercial use prohibited") || strings.Contains(text, "상업적 이용 금지") {
		return "prohibited"
	}
	if strings.Contains(text, "commercial use") || strings.Contains(text, "상업적 이용") {
		return "terms_found_requires_review"
	}
	return "unverified"
}
func detectRestrictions(text string) []string {
	keys := []string{"redistribution", "재배포", "attribution", "출처 표시", "storage", "저장", "ai processing", "인공지능"}
	out := []string{}
	for _, key := range keys {
		if strings.Contains(text, key) {
			out = append(out, key)
		}
	}
	return out
}
func termsURL(rawURL, text string) string {
	if strings.Contains(strings.ToLower(rawURL), "terms") || strings.Contains(strings.ToLower(rawURL), "license") || strings.Contains(text, "이용약관") {
		return rawURL
	}
	return ""
}
func hasPrimaryEvidence(items []investigation.Evidence) bool {
	for _, item := range items {
		if item.ReliabilityGrade == "A" || item.ReliabilityGrade == "B" {
			return true
		}
	}
	return false
}
func dimensions(kind string, evidence []investigation.Evidence) []string {
	if len(evidence) == 0 {
		return investigation.DimensionDefaults(kind)
	}
	joined := ""
	publishers := map[string]bool{}
	hasOfficial, hasTerms, hasPricing, hasOriginal, hasSample := false, false, false, false, false
	for _, item := range evidence {
		joined += " " + strings.ToLower(item.Excerpt)
		publishers[item.Publisher] = true
		hasOfficial = hasOfficial || item.OfficialSource
		hasTerms = hasTerms || item.SourceType == "license_terms"
		hasPricing = hasPricing || item.PricingStatus != "unverified"
		hasOriginal = hasOriginal || item.SourceType == "original_paper_or_statistics" || item.ReliabilityGrade == "A"
		hasSample = hasSample || item.SampleCallVerified
	}
	independent := len(publishers) >= 2
	switch kind {
	case "public_data":
		return []string{"existence=" + state(hasOfficial), "access=confirmed", "target_fields_scope=" + keywordState(joined, "field", "필드", "schema", "항목"), "freshness_quality=" + keywordState(joined, "updated", "갱신", "quality", "품질", "결측"), "cost=" + state(hasPricing), "commercial_use=" + state(hasTerms), "storage_processing_ai_derivatives_redistribution=" + state(hasTerms)}
	case "api", "technology":
		return []string{"official_docs=" + state(hasOfficial), "required_capability=" + keywordState(joined, "endpoint", "기능", "response", "응답"), "limits_availability=" + keywordState(joined, "limit", "quota", "제한", "availability"), "price=" + state(hasPricing), "deprecation=" + keywordState(joined, "deprecated", "deprecation", "폐지", "지원 종료"), "operability=" + state(hasOfficial), "sample_call=" + state(hasSample)}
	case "paper", "statistic", "quantitative_claim":
		return []string{"population_unit_period_sample_calculation=" + keywordState(joined, "sample", "표본", "unit", "단위", "period", "기간"), "country_domain_conditions=" + keywordState(joined, "country", "국가", "domain", "대상"), "original_location=" + state(hasOriginal), "independent_crosscheck=" + state(independent)}
	case "benchmark":
		return []string{"actual_feature=" + keywordState(joined, "feature", "기능", "service", "서비스"), "target_country_price=" + state(hasPricing), "operating_conditions=" + keywordState(joined, "terms", "조건", "availability", "운영"), "marketing_claim_separated=" + state(independent)}
	default:
		return []string{"fact=" + state(hasOfficial || independent)}
	}
}

func state(confirmed bool) string {
	if confirmed {
		return "evidence_found_requires_interpretation"
	}
	return "unverified"
}

func keywordState(text string, keywords ...string) string {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return "evidence_found_requires_interpretation"
		}
	}
	return "unverified"
}
func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "Untitled source"
}
