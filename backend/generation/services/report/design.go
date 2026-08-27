package report

import (
	"context"
	"errors"

	design "noplanner/backend/generation/domains/design"
	"noplanner/backend/generation/modules/rpcerror"
	"noplanner/backend/generation/modules/uivalidator"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
)

func (s *Service) GenerateProductDesign(ctx context.Context, req *generationv1.GenerateProductDesignRequest) (*generationv1.ProductDesign, error) {
	requirements := make([]design.Requirement, 0, len(req.GetRequirements()))
	for _, value := range req.GetRequirements() {
		requirements = append(requirements, design.Requirement{ID: value.GetId(), Statement: value.GetStatement(), AcceptanceCriteria: value.GetAcceptanceCriteria()})
	}
	validations := make([]design.Validation, 0, len(req.GetValidationResults()))
	for _, value := range req.GetValidationResults() {
		validations = append(validations, design.Validation{ID: value.GetId(), Kind: value.GetKind(), Status: value.GetStatus(), CommandOrViewport: value.GetCommandOrViewport(), Detail: value.GetDetail(), ScreenIDs: value.GetScreenIds()})
	}
	result, err := design.Generate(req.GetPlanId(), req.GetBaseRevision(), requirements, req.GetUserFlows(), validations)
	if err != nil {
		return nil, invalidPlanning(ctx)
	}
	if s.uiValidator == nil {
		result = design.ReplaceExecutionValidations(result, []design.Validation{{ID: "execution-validator-unavailable", Kind: "build", Status: "failed", Detail: "생성 UI 실행 검증기가 구성되지 않았습니다."}})
	} else {
		executionInput := uivalidator.Product{}
		for _, screen := range result.Screens {
			executionInput.ScreenIDs = append(executionInput.ScreenIDs, screen.ID)
		}
		for _, file := range result.ChangeSet.Files {
			executionInput.Files = append(executionInput.Files, uivalidator.File{Path: file.Path, Content: file.Content, ScreenIDs: file.ScreenIDs})
		}
		executionResults := s.uiValidator.Validate(ctx, executionInput)
		verified := make([]design.Validation, 0, len(executionResults))
		for _, validation := range executionResults {
			verified = append(verified, design.Validation{ID: validation.ID, Kind: validation.Kind, Status: validation.Status, CommandOrViewport: validation.Command, Detail: validation.Detail, ScreenIDs: validation.ScreenIDs})
		}
		result = design.ReplaceExecutionValidations(result, verified)
	}
	result.OwnerSubject = req.GetOwnerSubject()
	if s.designStore != nil {
		if _, err = s.designStore.Save(ctx, result); err != nil {
			return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "제품 설계를 저장하지 못했습니다.", true)
		}
	}
	return productDesignProto(result), nil
}

func (s *Service) GetProductDesign(ctx context.Context, req *generationv1.GetProductDesignRequest) (*generationv1.ProductDesign, error) {
	if s.designStore == nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "제품 설계 저장소가 없습니다.", false)
	}
	value, err := s.designStore.Get(ctx, req.GetDesignId())
	if errors.Is(err, design.ErrNotFound) {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_NOT_FOUND, "제품 설계를 찾을 수 없습니다.", false)
	}
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "제품 설계를 불러오지 못했습니다.", true)
	}
	return productDesignProto(value), nil
}

func productDesignProto(value design.ProductDesign) *generationv1.ProductDesign {
	out := &generationv1.ProductDesign{Id: value.ID, PlanId: value.PlanID, Executable: value.Executable, Blockers: value.Blockers, OwnerSubject: value.OwnerSubject}
	for _, item := range value.Goals {
		out.Goals = append(out.Goals, &generationv1.ExperienceGoal{Id: item.ID, RequirementId: item.RequirementID, User: item.User, Goal: item.Goal, SuccessSignal: item.SuccessSignal})
	}
	for _, item := range value.Journey {
		out.Journey = append(out.Journey, &generationv1.JourneyStep{Id: item.ID, GoalId: item.GoalID, RequirementId: item.RequirementID, UserAction: item.UserAction, SystemResponse: item.SystemResponse, NextStepId: item.NextStepID})
	}
	for _, item := range value.InformationArchitecture {
		out.InformationArchitecture = append(out.InformationArchitecture, &generationv1.InformationNode{Id: item.ID, Label: item.Label, ParentId: item.ParentID, RequirementIds: item.RequirementIDs})
	}
	for _, item := range value.Screens {
		screen := &generationv1.ScreenSpecification{Id: item.ID, Name: item.Name, Purpose: item.Purpose, RequirementIds: item.RequirementIDs, DataFields: item.DataFields}
		for _, action := range item.Actions {
			screen.Actions = append(screen.Actions, &generationv1.ScreenAction{Id: action.ID, Label: action.Label, ResultScreenId: action.ResultScreenID, RequirementId: action.RequirementID})
		}
		for _, state := range item.States {
			screen.States = append(screen.States, &generationv1.UIState{Kind: stateKindProto(state.Kind), Presentation: state.Presentation, AvailableActions: state.AvailableActions})
		}
		out.Screens = append(out.Screens, screen)
	}
	for _, item := range value.Findings {
		out.Findings = append(out.Findings, &generationv1.DesignFinding{Id: item.ID, Area: item.Area, Severity: item.Severity, Finding: item.Finding, Correction: item.Correction, Status: item.Status, ScreenIds: item.ScreenIDs, RequirementIds: item.RequirementIDs})
	}
	out.ChangeSet = &generationv1.UIChangeSet{Id: value.ChangeSet.ID, BaseRevision: value.ChangeSet.BaseRevision, BranchName: value.ChangeSet.BranchName, Stack: value.ChangeSet.Stack}
	for _, item := range value.ChangeSet.Files {
		out.ChangeSet.Files = append(out.ChangeSet.Files, &generationv1.CodeFileChange{Path: item.Path, Action: item.Action, Content: item.Content, RequirementIds: item.RequirementIDs, ScreenIds: item.ScreenIDs})
	}
	for _, item := range value.Validations {
		out.Validations = append(out.Validations, &generationv1.ValidationCheck{Id: item.ID, Kind: item.Kind, Status: item.Status, CommandOrViewport: item.CommandOrViewport, Detail: item.Detail, ScreenIds: item.ScreenIDs})
	}
	for _, item := range value.Traceability {
		out.Traceability = append(out.Traceability, &generationv1.TraceLink{RequirementId: item.RequirementID, GoalIds: item.GoalIDs, JourneyStepIds: item.JourneyStepIDs, ScreenIds: item.ScreenIDs, FilePaths: item.FilePaths})
	}
	return out
}

func stateKindProto(value string) generationv1.UIStateKind {
	return map[string]generationv1.UIStateKind{"normal": generationv1.UIStateKind_UI_STATE_KIND_NORMAL, "loading": generationv1.UIStateKind_UI_STATE_KIND_LOADING, "empty": generationv1.UIStateKind_UI_STATE_KIND_EMPTY, "error": generationv1.UIStateKind_UI_STATE_KIND_ERROR, "forbidden": generationv1.UIStateKind_UI_STATE_KIND_FORBIDDEN}[value]
}
