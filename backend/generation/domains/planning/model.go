package planning

import (
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid planning input")

type Finding struct {
	ID, Severity, RequiredAction, VerificationMethod string
}
type ImprovementTask struct {
	ID, Action, VerificationMethod string
	FindingIDs, RequiredResearch   []string
	Order                          uint32
	Blocking                       bool
}
type ImprovementPlan struct {
	ID, EvaluationID string
	Tasks            []ImprovementTask
}
type Claim struct {
	Statement, ID           string
	EvidenceIDs, FindingIDs []string
	Verified                bool
}
type Section struct {
	Key, Title, Content           string
	SourceFindingIDs, EvidenceIDs []string
}
type Change struct {
	SectionKey, Before, After, Reason string
	FindingIDs                        []string
}
type ImprovedPlan struct {
	ID, PreservedIntent  string
	Sections             []Section
	Changes              []Change
	UnresolvedFindingIDs []string
}
type Context struct {
	Purpose, TargetUser, Environment string
	Constraints                      []string
}
type Question struct{ ID, Field, Question, Reason string }
type ResearchFact struct {
	ID, Question, Conclusion string
	EvidenceIDs              []string
	Verified                 bool
	Uncertainties            []string
}
type Alternative struct {
	ID, Name, Description, DecisionReason  string
	EvidenceIDs, Advantages, Disadvantages []string
	Selected                               bool
}
type Requirement struct {
	ID, Statement, AcceptanceCriteria string
	EvidenceIDs                       []string
}
type Specification struct {
	Problem, Purpose, TargetUser, Scope, OutOfScope  string
	UserFlow, Policies, SuccessCriteria, Assumptions []string
	Requirements                                     []Requirement
}
type SelfEvaluation struct {
	Verdict, Confidence string
	Findings            []Finding
	UnresolvedBlockers  []string
}
type AutonomousPlan struct {
	ID, Idea, CriteriaVersion, Model, OwnerSubject string
	Context                                        Context
	Research                                       []ResearchFact
	Alternatives                                   []Alternative
	Specification                                  Specification
	SelfEvaluation                                 SelfEvaluation
	TraceIDs                                       []string
	CreatedAt                                      time.Time
}
