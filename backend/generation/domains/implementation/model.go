package implementation

import (
	"context"
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid implementation input")

type File struct {
	Path, Language, ContentHash string
	Size                        uint64
	Symbols                     []string
}
type Repository struct {
	ID, ProjectID, Root, Revision string
	Files                         []File
	Features                      []string
	ReadOnly                      bool
}
type Expectation struct {
	RequirementID, Statement, AcceptanceCriteria string
	ExpectedPaths, ExpectedTestPaths             []string
}
type Mismatch struct {
	ID, RequirementID, Kind, PlanLocation, Finding, RequiredChange, Status string
	CodeLocations                                                          []string
}
type Review struct {
	ID, ProjectID, EvaluationID, RepositoryRevision, RepositoryRoot string
	Mismatches                                                      []Mismatch
	SatisfiedRequirementIDs                                         []string
}
type Patch struct {
	Path, UnifiedDiff           string
	MismatchIDs, RequirementIDs []string
}
type Proposal struct {
	ID, ReviewID, BaseRevision, ProposedBranch string
	Patches                                    []Patch
	WorkingTreeModified                        bool
}
type HistoryItem struct {
	ID, ProjectID, Kind, SourceID, ResultID, Revision, ActorID, Target, Status, PayloadHash string
	CreatedAt                                                                               time.Time
}
type Store interface {
	Append(context.Context, HistoryItem) (HistoryItem, error)
	History(context.Context, string) ([]HistoryItem, error)
}
