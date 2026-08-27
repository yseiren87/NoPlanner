package implementation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func Compare(evaluationID string, repo Repository, expectations []Expectation) (Review, error) {
	if evaluationID == "" || repo.Revision == "" || len(expectations) == 0 {
		return Review{}, ErrInvalid
	}
	result := Review{ID: "review-" + stable(evaluationID+repo.Revision), ProjectID: repo.ProjectID, EvaluationID: evaluationID, RepositoryRevision: repo.Revision, RepositoryRoot: repo.Root}
	paths := map[string]File{}
	for _, file := range repo.Files {
		paths[filepath.ToSlash(file.Path)] = file
	}
	for _, expectation := range expectations {
		if expectation.RequirementID == "" || expectation.AcceptanceCriteria == "" {
			return Review{}, ErrInvalid
		}
		missingCode := missing(expectation.ExpectedPaths, paths)
		missingTests := missing(expectation.ExpectedTestPaths, paths)
		if len(missingCode) > 0 {
			result.Mismatches = append(result.Mismatches, Mismatch{ID: "mismatch-code-" + expectation.RequirementID, RequirementID: expectation.RequirementID, Kind: "missing_implementation", PlanLocation: expectation.RequirementID, CodeLocations: missingCode, Finding: "요구사항이 지정한 구현 파일을 찾을 수 없습니다.", RequiredChange: "요구사항을 구현하거나 실제 코드 위치를 연결합니다.", Status: "open"})
		}
		if len(missingTests) > 0 {
			result.Mismatches = append(result.Mismatches, Mismatch{ID: "mismatch-test-" + expectation.RequirementID, RequirementID: expectation.RequirementID, Kind: "missing_acceptance_test", PlanLocation: expectation.RequirementID, CodeLocations: missingTests, Finding: "인수 조건을 검증하는 테스트를 찾을 수 없습니다.", RequiredChange: "인수 조건과 연결된 자동 테스트를 추가합니다.", Status: "open"})
		}
		if len(missingCode) == 0 && len(missingTests) == 0 {
			result.SatisfiedRequirementIDs = append(result.SatisfiedRequirementIDs, expectation.RequirementID)
		}
	}
	return result, nil
}
func Propose(review Review, baseRevision string, patches []Patch) (Proposal, error) {
	if review.ID == "" || baseRevision == "" || baseRevision != review.RepositoryRevision || len(patches) == 0 {
		return Proposal{}, ErrInvalid
	}
	known := map[string]bool{}
	for _, m := range review.Mismatches {
		known[m.ID] = true
	}
	for _, patch := range patches {
		if patch.Path == "" || !strings.HasPrefix(patch.UnifiedDiff, "--- ") || len(patch.MismatchIDs) == 0 {
			return Proposal{}, ErrInvalid
		}
		for _, id := range patch.MismatchIDs {
			if !known[id] {
				return Proposal{}, ErrInvalid
			}
		}
	}
	return Proposal{ID: "proposal-" + stable(review.ID), ReviewID: review.ID, BaseRevision: baseRevision, ProposedBranch: "noplanner/implementation-" + stable(review.ID), Patches: patches, WorkingTreeModified: false}, nil
}
func PayloadHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func missing(expected []string, actual map[string]File) []string {
	var result []string
	for _, pattern := range expected {
		matched := false
		for path := range actual {
			ok, _ := filepath.Match(pattern, path)
			if ok || path == filepath.ToSlash(pattern) {
				matched = true
				break
			}
		}
		if !matched {
			result = append(result, pattern)
		}
	}
	sort.Strings(result)
	return result
}
func stable(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))[:12] }
