package revision

import (
	"context"
	"time"
)

type Location struct {
	Page, Slide, Paragraph uint32
	Sheet                  string
	Sections               []string
}
type Item struct {
	ID, Kind, Content string
	Ordinal           uint32
	Location          Location
}
type Change struct {
	StableID, ItemKind, Kind          string
	Previous, Current                 *Item
	AffectedAreas, AffectedFindingIDs []string
	RequiresResearch                  bool
	Reason                            string
}
type Comparison struct {
	ID, DocumentID                                      string
	PreviousVersion, CurrentVersion                     uint32
	Changes                                             []Change
	AffectedAreas, AffectedFindingIDs, ResearchFirstIDs []string
	CreatedAt                                           time.Time
}
type Finding struct {
	ID, Area, Statement, Finding, Severity, Status, SourceLocation string
	EvidenceIDs                                                    []string
}
type FindingChange struct {
	FindingID, Status, Reason string
	Previous, Current         *Finding
	PreservedEvidenceIDs      []string
}
type Reevaluation struct {
	ID, DocumentID, PreviousEvaluationID, CurrentEvaluationID string
	FindingChanges                                            []FindingChange
	RerunAreas, RerunResearchIDs, PreservedAreas              []string
	PreviousVerdict, CurrentVerdict                           string
	VerdictChanged                                            bool
	VerdictChangeReasons                                      []string
	CriteriaVersion, Model, ObjectionID                       string
	NewEvidenceIDs                                            []string
	CreatedAt                                                 time.Time
}
type Evidence struct{ ID, Title, URLOrLocation, ApplicableScope, ContentHash string }
type Objection struct {
	ID, EvaluationID, FindingID, Explanation, Status, ResultingReevaluationID string
	ResearchID, VerificationStatus                                            string
	Evidence                                                                  []Evidence
	RemainingUncertainties                                                    []string
	CreatedAt                                                                 time.Time
}
type Store interface {
	SaveComparison(context.Context, Comparison) (Comparison, error)
	SaveReevaluation(context.Context, Reevaluation) (Reevaluation, error)
	SaveObjection(context.Context, Objection) (Objection, error)
	ListVerdictChanges(context.Context, string) ([]Reevaluation, error)
}
