package investigation

import "testing"

func TestBuildPlanPrioritizesBlockingAssumptionsAndSelectsValidation(t *testing.T) {
	plan, err := BuildPlan("evaluation-1", "KR", "healthcare", []Assumption{
		{ID: "low", Statement: "일반 사례가 있다", BlockingLikelihood: .2, DependencyTypes: []string{"benchmark"}},
		{ID: "high", Statement: "공공 데이터와 API를 확보할 수 있다", BlockingLikelihood: .9, DependencyTypes: []string{"public_data", "api"}},
	}, Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 3 || plan.Tasks[0].AssumptionID != "high" || plan.Tasks[0].Depth != "deep" {
		t.Fatalf("tasks = %#v", plan.Tasks)
	}
	if plan.Limit.MaxQueries == 0 || plan.Limit.MaxSources == 0 || plan.Limit.SaturationQueries == 0 {
		t.Fatalf("defaults missing: %#v", plan.Limit)
	}
}

func TestPublicDataDimensionsKeepCommercialUseSeparate(t *testing.T) {
	dimensions := DimensionDefaults("public_data")
	want := map[string]bool{"existence=unverified": true, "access=unverified": true, "cost=unverified": true, "commercial_use=unverified": true, "storage_processing_ai_derivatives_redistribution=unverified": true}
	for _, dimension := range dimensions {
		delete(want, dimension)
	}
	if len(want) > 0 {
		t.Fatalf("missing dimensions: %#v", want)
	}
}
