package report

import (
	"context"
	"time"
)

type Evidence struct{ ID, Title, URL, SourceLocation, Grade, Relation string }
type FindingInput struct {
	ID, Area, ProblemType, SourceDocument, SourceLocation, Statement, Finding string
	Evidence                                                                  []Evidence
	ReasoningSummary, Impact                                                  string
	Likelihood, ImpactScore, RecoveryCost, Reversibility                      float64
	RequiredAction, VerificationMethod, Status                                string
	CriticalAssumption, ConfirmedFalse                                        bool
}
type Finding struct {
	Detail               FindingInput
	Severity, Confidence string
}
type AreaResult struct {
	Area, Status, Reason string
	FindingCount         uint32
	Confidence           string
}
type Assumption struct {
	ID, Statement, Status     string
	Dependencies, EvidenceIDs []string
}
type Contradiction struct{ ID, FirstLocation, FirstStatement, SecondLocation, SecondStatement, Reason string }
type Research struct {
	Question, Conclusion string
	Sources, Conflicts   []Evidence
	Uncertainties        []string
}
type Summary struct{ Problem, Purpose, TargetUser, Environment, Solution, SuccessCriteria string }
type ProgressStep struct{ Code, Label, Status, Reason string }
type Report struct {
	ID, EvaluationID, ProjectID, ProjectName, OwnerSubject, DocumentID, DocumentName, Country, Domain, TargetUser, CriteriaVersion, Model string
	DocumentVersion                                                                                                                       uint32
	CreatedAt                                                                                                                             time.Time
	Verdict, Confidence, Conclusion                                                                                                       string
	ImplementationMayStart                                                                                                                bool
	Findings                                                                                                                              []Finding
	Areas                                                                                                                                 []AreaResult
	Assumptions                                                                                                                           []Assumption
	Contradictions                                                                                                                        []Contradiction
	Research                                                                                                                              []Research
	RequiredActions, Recommendations, VerdictChangeConditions, Limitations                                                                []string
	Summary                                                                                                                               Summary
	Progress                                                                                                                              []ProgressStep
	EvaluationStatus                                                                                                                      string
}
type Store interface {
	Save(context.Context, Report) (Report, error)
	Get(context.Context, string) (Report, error)
}
