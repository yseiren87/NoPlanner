package quality

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type goldenDataset struct {
	Version, Status, Description string
	Cases                        []goldenCase `json:"cases"`
}

type goldenCase struct {
	ID                 string
	Category           string
	SourceKind         string `json:"source_kind"`
	ExpertReviewStatus string `json:"expert_review_status"`
	Document           string
	ExpectedVerdict    string          `json:"expected_verdict"`
	RevisionPairID     string          `json:"revision_pair_id"`
	ExpectedFindings   []goldenFinding `json:"expected_findings"`
	ExpectedEvidence   []struct {
		Claim               string
		RequiredSourceTypes []string `json:"required_source_types"`
		MustVerify          []string `json:"must_verify"`
	} `json:"expected_evidence"`
}

type goldenFinding struct {
	ID             string
	Area           string
	ProblemType    string `json:"problem_type"`
	Severity       string
	SourceQuote    string `json:"source_quote"`
	Rationale      string
	RequiredAction string `json:"required_action"`
}

func TestGoldenDatasetContract(t *testing.T) {
	content, err := os.ReadFile("testdata/golden-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var dataset goldenDataset
	if err = json.Unmarshal(content, &dataset); err != nil {
		t.Fatal(err)
	}
	if dataset.Version != "golden-v1" || dataset.Status != "ready" || len(dataset.Cases) != 10 {
		t.Fatalf("invalid dataset header or case count: %#v", dataset)
	}
	ids, categories, pairs := map[string]bool{}, map[string]int{}, map[string]int{}
	for _, item := range dataset.Cases {
		if item.ID == "" || ids[item.ID] || item.SourceKind != "synthetic" || item.ExpertReviewStatus != "not_required" || item.Document == "" || item.ExpectedVerdict == "" {
			t.Fatalf("invalid case metadata: %#v", item)
		}
		ids[item.ID], categories[item.Category] = true, categories[item.Category]+1
		if item.RevisionPairID != "" {
			pairs[item.RevisionPairID]++
		}
		blocking := false
		findingIDs := map[string]bool{}
		for _, finding := range item.ExpectedFindings {
			if finding.ID == "" || findingIDs[finding.ID] || finding.Area == "" || finding.ProblemType == "" || finding.Severity == "" || finding.Rationale == "" || finding.RequiredAction == "" || !strings.Contains(item.Document, finding.SourceQuote) {
				t.Fatalf("invalid finding in %s: %#v", item.ID, finding)
			}
			findingIDs[finding.ID] = true
			blocking = blocking || finding.Severity == "blocking"
		}
		if blocking && item.ExpectedVerdict == "executable" {
			t.Fatalf("blocking finding was offset in %s", item.ID)
		}
		for _, evidence := range item.ExpectedEvidence {
			if evidence.Claim == "" || len(evidence.RequiredSourceTypes) == 0 || len(evidence.MustVerify) == 0 {
				t.Fatalf("invalid evidence expectation in %s", item.ID)
			}
		}
	}
	for _, required := range []string{"normal", "conditionally_executable", "rewrite_required", "not_executable", "before_revision", "after_revision"} {
		if categories[required] == 0 {
			t.Fatalf("missing category %s", required)
		}
	}
	if pairs["PAIR-001"] != 2 {
		t.Fatalf("revision pair is incomplete: %#v", pairs)
	}
}
