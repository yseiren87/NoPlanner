package implementation

import "testing"

func TestCompareAndProposalRemainTraceable(t *testing.T) {
	repo := Repository{Revision: "abc", Files: []File{{Path: "src/app.tsx"}, {Path: "src/app.test.tsx"}}}
	review, err := Compare("EV-1", repo, []Expectation{{RequirementID: "REQ-1", AcceptanceCriteria: "동작", ExpectedPaths: []string{"src/app.tsx"}, ExpectedTestPaths: []string{"src/missing.test.tsx"}}})
	if err != nil || len(review.Mismatches) != 1 || review.Mismatches[0].RequirementID != "REQ-1" {
		t.Fatalf("review: %#v %v", review, err)
	}
	proposal, err := Propose(review, "abc", []Patch{{Path: "src/missing.test.tsx", UnifiedDiff: "--- /dev/null\n+++ b/src/missing.test.tsx", MismatchIDs: []string{review.Mismatches[0].ID}, RequirementIDs: []string{"REQ-1"}}})
	if err != nil || proposal.WorkingTreeModified || proposal.ProposedBranch == "" {
		t.Fatalf("proposal: %#v %v", proposal, err)
	}
}
