package assessment

import "testing"

func TestSummarizeExcludesNotApplicableAndSkippedFromDefectsAndScore(t *testing.T) {
	summary, err := Summarize("plan-1", []ValidationResult{
		{Area: "purpose", Status: "satisfied", Reason: "Passed"},
		{Area: "data", Status: "partially_satisfied", Reason: "Some data missing"},
		{Area: "api", Status: "not_satisfied", Reason: "API unavailable"},
		{Area: "technology", Status: "unverified", Reason: "Prototype required"},
		{Area: "resource", Status: "not_applicable", Reason: "No resource dependency"},
		{Area: "operations", Status: "skipped", Reason: "Connector not configured"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.DefectCount != 2 || summary.ScoredCount != 4 || summary.ExcludedCount != 2 || summary.SatisfactionRate != .375 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.Results[4].CountedAsDefect || summary.Results[4].Scored || summary.Results[5].CountedAsDefect || summary.Results[5].Scored {
		t.Fatalf("excluded results = %#v", summary.Results[4:])
	}
}

func TestSummarizeRejectsDuplicateArea(t *testing.T) {
	_, err := Summarize("plan-1", []ValidationResult{{Area: "data", Status: "satisfied", Reason: "ok"}, {Area: "data", Status: "skipped", Reason: "no config"}})
	if err == nil {
		t.Fatal("expected duplicate area error")
	}
}
