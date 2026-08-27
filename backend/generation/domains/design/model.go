package design

import "errors"

var ErrInvalid = errors.New("invalid product design input")

type Requirement struct{ ID, Statement, AcceptanceCriteria string }
type Goal struct{ ID, RequirementID, User, Goal, SuccessSignal string }
type JourneyStep struct{ ID, GoalID, RequirementID, UserAction, SystemResponse, NextStepID string }
type InformationNode struct {
	ID, Label, ParentID string
	RequirementIDs      []string
}
type State struct {
	Kind, Presentation string
	AvailableActions   []string
}
type Action struct{ ID, Label, ResultScreenID, RequirementID string }
type Screen struct {
	ID, Name, Purpose          string
	RequirementIDs, DataFields []string
	Actions                    []Action
	States                     []State
}
type Finding struct {
	ID, Area, Severity, Finding, Correction, Status string
	ScreenIDs, RequirementIDs                       []string
}
type FileChange struct {
	Path, Action, Content     string
	RequirementIDs, ScreenIDs []string
}
type ChangeSet struct {
	ID, BaseRevision, BranchName, Stack string
	Files                               []FileChange
}
type Validation struct {
	ID, Kind, Status, CommandOrViewport, Detail string
	ScreenIDs                                   []string
}
type Trace struct {
	RequirementID                                 string
	GoalIDs, JourneyStepIDs, ScreenIDs, FilePaths []string
}
type ProductDesign struct {
	ID, PlanID, OwnerSubject string
	Goals                    []Goal
	Journey                  []JourneyStep
	InformationArchitecture  []InformationNode
	Screens                  []Screen
	Findings                 []Finding
	ChangeSet                ChangeSet
	Validations              []Validation
	Traceability             []Trace
	Executable               bool
	Blockers                 []string
}
