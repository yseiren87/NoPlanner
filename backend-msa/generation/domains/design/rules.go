package design

import (
	"fmt"
	"strconv"
	"strings"
)

var requiredStates = []string{"normal", "loading", "empty", "error", "forbidden"}
var requiredValidations = []string{"build", "render", "interaction", "accessibility", "responsive"}

func Generate(planID, baseRevision string, requirements []Requirement, userFlows []string, validations []Validation) (ProductDesign, error) {
	if strings.TrimSpace(planID) == "" || strings.TrimSpace(baseRevision) == "" || len(requirements) == 0 || len(userFlows) == 0 {
		return ProductDesign{}, ErrInvalid
	}
	result := ProductDesign{ID: "design-" + planID, PlanID: planID, Executable: true}
	result.ChangeSet = ChangeSet{ID: "change-" + planID, BaseRevision: baseRevision, BranchName: "noplanner/design-" + planID, Stack: "React + TypeScript + Tailwind CSS + shadcn/ui"}
	result.InformationArchitecture = append(result.InformationArchitecture, InformationNode{ID: "ia-root", Label: "제품", RequirementIDs: requirementIDs(requirements)})
	for index, requirement := range requirements {
		if requirement.ID == "" || requirement.Statement == "" || requirement.AcceptanceCriteria == "" {
			return ProductDesign{}, ErrInvalid
		}
		goalID := "goal-" + requirement.ID
		screenID := "screen-" + requirement.ID
		result.Goals = append(result.Goals, Goal{ID: goalID, RequirementID: requirement.ID, User: "대상 사용자", Goal: requirement.Statement, SuccessSignal: requirement.AcceptanceCriteria})
		next := ""
		if index+1 < len(requirements) {
			next = "step-" + requirements[index+1].ID
		}
		flow := userFlows[index%len(userFlows)]
		result.Journey = append(result.Journey, JourneyStep{ID: "step-" + requirement.ID, GoalID: goalID, RequirementID: requirement.ID, UserAction: flow, SystemResponse: "행동 결과와 다음 선택지를 즉시 표시한다.", NextStepID: next})
		states := []State{}
		for _, kind := range requiredStates {
			states = append(states, State{Kind: kind, Presentation: statePresentation(kind), AvailableActions: stateActions(kind)})
		}
		result.Screens = append(result.Screens, Screen{ID: screenID, Name: requirement.Statement, Purpose: requirement.AcceptanceCriteria, RequirementIDs: []string{requirement.ID}, DataFields: []string{"status", "content", "error"}, Actions: []Action{{ID: "action-" + requirement.ID, Label: "계속", ResultScreenID: nextScreen(requirements, index), RequirementID: requirement.ID}}, States: states})
		result.InformationArchitecture = append(result.InformationArchitecture, InformationNode{ID: "ia-" + requirement.ID, Label: requirement.Statement, ParentID: "ia-root", RequirementIDs: []string{requirement.ID}})
		path := fmt.Sprintf("src/generated/%s.tsx", screenID)
		result.ChangeSet.Files = append(result.ChangeSet.Files, FileChange{Path: path, Action: "create", Content: reactProposal(screenID, requirement.Statement), RequirementIDs: []string{requirement.ID}, ScreenIDs: []string{screenID}})
		result.Traceability = append(result.Traceability, Trace{RequirementID: requirement.ID, GoalIDs: []string{goalID}, JourneyStepIDs: []string{"step-" + requirement.ID}, ScreenIDs: []string{screenID}, FilePaths: []string{path}})
	}
	result.Validations = automaticValidations(result)
	for _, validation := range validations {
		if validation.Status == "passed" {
			continue
		}
		result.Validations = append(result.Validations, validation)
	}
	for _, validation := range result.Validations {
		if validation.Status != "passed" {
			result.Executable = false
			result.Blockers = append(result.Blockers, validation.ID)
			result.Findings = append(result.Findings, Finding{ID: "finding-" + validation.ID, Area: validation.Kind, Severity: "blocking", Finding: validation.Detail, Correction: "검증 실패를 수정하고 동일 조건으로 재실행한다.", Status: "open", ScreenIDs: validation.ScreenIDs})
		}
	}
	for _, screen := range result.Screens {
		if issue := evaluateScreen(screen); issue != "" {
			result.Executable = false
			result.Blockers = append(result.Blockers, issue)
			result.Findings = append(result.Findings, Finding{ID: issue, Area: "design_quality", Severity: "high", Finding: "필수 상태 또는 추적 정보가 누락되었습니다.", Correction: "정상·로딩·빈·오류·권한 상태와 요구사항 연결을 보완합니다.", Status: "open", ScreenIDs: []string{screen.ID}, RequirementIDs: screen.RequirementIDs})
		}
	}
	return result, nil
}

func automaticValidations(result ProductDesign) []Validation {
	checks := map[string]bool{"build": true, "render": len(result.Screens) > 0, "interaction": true, "accessibility": true, "responsive": true}
	allScreens := map[string]bool{"complete": true}
	for _, screen := range result.Screens {
		allScreens[screen.ID] = true
	}
	for _, file := range result.ChangeSet.Files {
		checks["build"] = checks["build"] && strings.Contains(file.Content, "export function") && strings.Contains(file.Content, "return <")
		checks["accessibility"] = checks["accessibility"] && strings.Contains(file.Content, "aria-")
		checks["responsive"] = checks["responsive"] && (strings.Contains(file.Content, "sm:") || strings.Contains(file.Content, "md:") || strings.Contains(file.Content, "lg:"))
	}
	for _, screen := range result.Screens {
		checks["render"] = checks["render"] && evaluateScreen(screen) == ""
		for _, action := range screen.Actions {
			checks["interaction"] = checks["interaction"] && allScreens[action.ResultScreenID]
		}
	}
	commands := map[string]string{"build": "typescript component contract", "render": "required UI state contract", "interaction": "action target graph", "accessibility": "semantic label contract", "responsive": "responsive utility contract"}
	values := make([]Validation, 0, len(requiredValidations))
	for _, kind := range requiredValidations {
		status, detail := "passed", "서버 생성 결과의 "+commands[kind]+" 검증을 통과했습니다."
		if !checks[kind] {
			status, detail = "failed", "서버 생성 결과가 "+commands[kind]+" 검증을 통과하지 못했습니다."
		}
		values = append(values, Validation{ID: "automatic-" + kind, Kind: kind, Status: status, CommandOrViewport: commands[kind], Detail: detail})
	}
	return values
}

// ReplaceExecutionValidations replaces structural preflight results with results
// produced by the isolated UI execution environment.
func ReplaceExecutionValidations(result ProductDesign, validations []Validation) ProductDesign {
	result.Validations = append([]Validation(nil), validations...)
	result.Executable = true
	result.Blockers = nil
	result.Findings = nil
	for _, validation := range validations {
		if validation.Status == "passed" {
			continue
		}
		result.Executable = false
		result.Blockers = append(result.Blockers, validation.ID)
		result.Findings = append(result.Findings, Finding{ID: "finding-" + validation.ID, Area: validation.Kind, Severity: "blocking", Finding: validation.Detail, Correction: "실행 검증 실패를 수정하고 동일한 격리 환경에서 다시 실행한다.", Status: "open", ScreenIDs: validation.ScreenIDs})
	}
	for _, screen := range result.Screens {
		if issue := evaluateScreen(screen); issue != "" {
			result.Executable = false
			result.Blockers = append(result.Blockers, issue)
			result.Findings = append(result.Findings, Finding{ID: issue, Area: "design_quality", Severity: "high", Finding: "필수 상태 또는 추적 정보가 누락되었습니다.", Correction: "정상·로딩·빈·오류·권한 상태와 요구사항 연결을 보완합니다.", Status: "open", ScreenIDs: []string{screen.ID}, RequirementIDs: screen.RequirementIDs})
		}
	}
	return result
}

func evaluateScreen(screen Screen) string {
	seen := map[string]bool{}
	for _, state := range screen.States {
		seen[state.Kind] = true
	}
	for _, kind := range requiredStates {
		if !seen[kind] {
			return "missing-" + kind + "-" + screen.ID
		}
	}
	if len(screen.RequirementIDs) == 0 {
		return "missing-trace-" + screen.ID
	}
	return ""
}
func requirementIDs(values []Requirement) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.ID)
	}
	return out
}
func nextScreen(values []Requirement, index int) string {
	if index+1 < len(values) {
		return "screen-" + values[index+1].ID
	}
	return "complete"
}
func statePresentation(kind string) string {
	return map[string]string{"normal": "핵심 데이터와 주요 행동 표시", "loading": "진행 상태와 중복 실행 방지", "empty": "빈 이유와 생성 행동 표시", "error": "오류 원인과 재시도 행동 표시", "forbidden": "권한 부족 이유와 복귀 행동 표시"}[kind]
}
func stateActions(kind string) []string {
	if kind == "error" {
		return []string{"retry", "back"}
	}
	if kind == "empty" {
		return []string{"create"}
	}
	if kind == "forbidden" {
		return []string{"back"}
	}
	return []string{"continue"}
}
func reactProposal(id, title string) string {
	return fmt.Sprintf(`import { useState } from 'react';
import { Button } from '@/components/ui/button';

export type UIState = 'normal' | 'loading' | 'empty' | 'error' | 'forbidden';

export function %s({ state = 'normal' }: { state?: UIState }) {
  const [completed, setCompleted] = useState(false);
  const messages: Record<Exclude<UIState, 'normal'>, string> = {
    loading: '처리 중입니다.', empty: '표시할 내용이 없습니다.',
    error: '처리하지 못했습니다. 다시 시도해주세요.', forbidden: '접근 권한이 없습니다.'
  };
  const titleId = %q + '-' + state;
  return <main data-state={state} className="mx-auto grid max-w-5xl gap-6 overflow-x-hidden p-4 md:p-8" aria-labelledby={titleId}>
    <h1 id={titleId} className="text-2xl font-semibold">{%s}</h1>
    {state === 'normal' ? <><p role="status">{completed ? '완료되었습니다.' : '실행할 수 있습니다.'}</p><Button onClick={() => setCompleted(true)}>계속</Button></> : <p role={state === 'error' ? 'alert' : 'status'}>{messages[state]}</p>}
  </main>;
}
`, strings.ReplaceAll(id, "-", "_"), "page-title-"+id, strconv.Quote(title))
}
