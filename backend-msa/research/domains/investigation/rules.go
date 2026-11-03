package investigation

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

var ErrInvalid = errors.New("invalid investigation")
var ErrNotFound = errors.New("investigation not found")

func NewID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "id-unavailable"
	}
	return hex.EncodeToString(raw[:])
}

func BuildPlan(evaluationID, country, domain string, assumptions []Assumption, limit Limit) (Plan, error) {
	if strings.TrimSpace(evaluationID) == "" || len(assumptions) == 0 {
		return Plan{}, ErrInvalid
	}
	if limit.MaxQueries == 0 {
		limit.MaxQueries = 20
	}
	if limit.MaxSources == 0 {
		limit.MaxSources = 30
	}
	if limit.MaxSeconds == 0 {
		limit.MaxSeconds = 300
	}
	if limit.SaturationQueries == 0 {
		limit.SaturationQueries = 3
	}
	plan := Plan{ID: NewID(), EvaluationID: evaluationID, TargetCountry: country, TargetDomain: domain, Limit: limit}
	for _, assumption := range assumptions {
		if assumption.ID == "" || strings.TrimSpace(assumption.Statement) == "" || assumption.BlockingLikelihood < 0 || assumption.BlockingLikelihood > 1 {
			return Plan{}, ErrInvalid
		}
		for _, kind := range unique(assumption.DependencyTypes) {
			sources, normalizedKind := sourceTypes(kind)
			depth := "standard"
			if assumption.BlockingLikelihood >= .7 {
				depth = "deep"
			}
			question := fmt.Sprintf("%s에서 %s에 대해 '%s' 전제가 실제로 성립하는가?", country, domain, assumption.Statement)
			plan.Tasks = append(plan.Tasks, Task{ID: NewID(), AssumptionID: assumption.ID, Question: question, RequiredSourceTypes: sources, Depth: depth, Priority: uint32(1000 * assumption.BlockingLikelihood), VerificationType: normalizedKind, Queries: queries(question, normalizedKind)})
		}
	}
	sort.SliceStable(plan.Tasks, func(i, j int) bool { return plan.Tasks[i].Priority > plan.Tasks[j].Priority })
	return plan, nil
}

func sourceTypes(kind string) ([]string, string) {
	switch kind {
	case "public_data":
		return []string{"official_dataset", "license_terms"}, kind
	case "api", "technology":
		return []string{"official_documentation", "pricing", "terms", "deprecation_notice"}, kind
	case "paper", "statistic", "quantitative_claim":
		return []string{"original_paper_or_statistics", "independent_source"}, kind
	case "benchmark":
		return []string{"official_service", "pricing", "independent_source"}, kind
	default:
		return []string{"official_primary_source", "independent_source"}, "general"
	}
}
func queries(question, kind string) []string {
	return []string{question + " 공식 원문", question + " " + kind + " 가격 약관", question + " 반대 근거 한계"}
}
func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return []string{"general"}
	}
	return out
}

func Grade(rawURL, sourceType string) (string, bool) {
	_, err := url.Parse(rawURL)
	if err != nil {
		return "D", false
	}
	official := IsLikelyOfficialSource(rawURL, sourceType)
	if official && (strings.Contains(sourceType, "official") || strings.Contains(sourceType, "dataset")) {
		return "A", true
	}
	if strings.Contains(sourceType, "paper") || strings.Contains(sourceType, "pricing") || strings.Contains(sourceType, "terms") {
		return "B", official
	}
	return "D", official
}

func IsLikelyOfficialSource(rawURL, sourceType string) bool {
	if IsGovernmentURL(rawURL) {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := strings.ToLower(u.Hostname() + u.Path)
	technicalPath := strings.Contains(path, "docs.") || strings.Contains(path, "developer.") || strings.Contains(path, "/docs/") || strings.Contains(path, "/developers/") || strings.Contains(path, "/api/")
	return technicalPath && (strings.Contains(sourceType, "official_documentation") || strings.Contains(sourceType, "api_response"))
}

func IsGovernmentURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.HasSuffix(host, ".go.kr") || strings.HasSuffix(host, ".gov") || strings.HasSuffix(host, ".gov.uk")
}

func DimensionDefaults(kind string) []string {
	switch kind {
	case "public_data":
		return []string{"existence=unverified", "access=unverified", "target_fields_scope=unverified", "freshness_quality=unverified", "cost=unverified", "commercial_use=unverified", "storage_processing_ai_derivatives_redistribution=unverified"}
	case "api", "technology":
		return []string{"official_docs=unverified", "required_capability=unverified", "limits_availability=unverified", "price=unverified", "deprecation=unverified", "operability=unverified", "sample_call=unverified"}
	case "paper", "statistic", "quantitative_claim":
		return []string{"population_unit_period_sample_calculation=unverified", "country_domain_conditions=unverified", "original_location=unverified", "independent_crosscheck=unverified"}
	case "benchmark":
		return []string{"actual_feature=unverified", "target_country_price=unverified", "operating_conditions=unverified", "marketing_claim_separated=unverified"}
	default:
		return []string{"fact=unverified"}
	}
}
