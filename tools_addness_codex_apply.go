package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const addnessCodexApplySource = "addness-mcp:addness_codex_apply_todays_goals_changes"

type addnessCodexApplyRequest struct {
	Version  int                      `json:"version"`
	Date     string                   `json:"date"`
	MemberID string                   `json:"member_id,omitempty"`
	Changes  []addnessCodexApplyChange `json:"changes"`
}

type addnessCodexApplyChange struct {
	Type string `json:"type"`

	TempID      string   `json:"temp_id,omitempty"`
	Title       string   `json:"title,omitempty"`
	ParentID    *string  `json:"parent_id,omitempty"`
	AfterGoalID string   `json:"after_goal_id,omitempty"`
	OrderNo     *float64 `json:"order_no,omitempty"`

	GoalID      string  `json:"goal_id,omitempty"`
	NewParentID string  `json:"new_parent_id,omitempty"`
	ExecutionID string  `json:"execution_id,omitempty"`
	Status      *string `json:"status,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

type addnessCodexApplyFailure struct {
	OK            bool                    `json:"ok"`
	FailedIndex   int                     `json:"failed_index"`
	FailedChange  addnessCodexApplyChange `json:"failed_change"`
	Error         string                  `json:"error"`
	AppliedCount  int                     `json:"applied_count"`
	PartialResult *addnessCodexTodaysGoalsViewPayload `json:"partial_result,omitempty"`
}

func addnessCodexApplyTodaysGoalsChangesTool() mcp.Tool {
	return mcp.NewTool("addness_codex_apply_todays_goals_changes",
		mcp.WithDescription("Addness Codex専用。today goals のローカル編集差分を一括適用し、更新後の view JSON を返す。"),
		mcp.WithString("payload",
			mcp.Required(),
			mcp.Description("JSON payload: version, date, optional member_id, changes[]"),
		),
	)
}

func handleAddnessCodexApplyTodaysGoalsChanges(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw := argStr(req.GetArguments(), "payload")
		if raw == "" {
			return errResult("payload is required"), nil
		}
		var request addnessCodexApplyRequest
		if err := json.Unmarshal([]byte(raw), &request); err != nil {
			return errResult(fmt.Sprintf("invalid payload JSON: %v", err)), nil
		}
		result, err := applyAddnessCodexTodaysGoalsChanges(ctx, client, request)
		if err != nil {
			if failure, ok := err.(*applyChangesError); ok {
				out, marshalErr := json.Marshal(failure.body)
				if marshalErr != nil {
					return errResult(fmt.Sprintf("apply failed: %v", failure.body.Error)), nil
				}
				return textResult(string(out)), nil
			}
			return errResult(err.Error()), nil
		}
		out, err := json.Marshal(result)
		if err != nil {
			return errResult(fmt.Sprintf("marshal error: %v", err)), nil
		}
		return textResult(string(out)), nil
	}
}

type applyChangesError struct {
	body addnessCodexApplyFailure
}

func (e *applyChangesError) Error() string {
	return e.body.Error
}

func applyAddnessCodexTodaysGoalsChanges(
	ctx context.Context,
	client *AddnessClient,
	request addnessCodexApplyRequest,
) (addnessCodexTodaysGoalsViewPayload, error) {
	if request.Version != 1 {
		return addnessCodexTodaysGoalsViewPayload{}, fmt.Errorf("unsupported version: %d", request.Version)
	}
	if err := requireOrg(client); err != nil {
		return addnessCodexTodaysGoalsViewPayload{}, err
	}
	date := request.Date
	if date == "" {
		date = currentActivityDateString(defaultActivityTimezone, defaultActivityCutoffHour)
	}
	if len(request.Changes) == 0 {
		return fetchAddnessCodexTodaysGoalsView(ctx, client, date, request.MemberID)
	}

	idMap := map[string]string{}
	applied := 0

	for index, change := range request.Changes {
		if err := applySingleCodexChange(ctx, client, date, change, idMap); err != nil {
			partial, _ := fetchAddnessCodexTodaysGoalsView(ctx, client, date, request.MemberID)
			return addnessCodexTodaysGoalsViewPayload{}, &applyChangesError{
				body: addnessCodexApplyFailure{
					OK:           false,
					FailedIndex:  index,
					FailedChange: change,
					Error:        err.Error(),
					AppliedCount: applied,
					PartialResult: &partial,
				},
			}
		}
		applied++
	}

	view, err := fetchAddnessCodexTodaysGoalsView(ctx, client, date, request.MemberID)
	if err != nil {
		return addnessCodexTodaysGoalsViewPayload{}, err
	}
	view.Meta.Source = addnessCodexApplySource
	return view, nil
}

func applySingleCodexChange(
	ctx context.Context,
	client *AddnessClient,
	date string,
	change addnessCodexApplyChange,
	idMap map[string]string,
) error {
	switch change.Type {
	case "create_goal":
		return applyCodexCreateChange(ctx, client, date, change, idMap)
	case "move_goal":
		goalID := resolveCodexGoalID(change.GoalID, idMap)
		orderNo := 0.0
		if change.OrderNo != nil {
			orderNo = *change.OrderNo
		}
		return codexMoveObjectiveParent(ctx, client, goalID, change.NewParentID, idMap, orderNo)
	case "reorder_goal":
		goalID := resolveCodexGoalID(change.GoalID, idMap)
		if change.OrderNo == nil {
			return fmt.Errorf("reorder_goal requires order_no")
		}
		return codexUpdateObjectiveOrderNo(ctx, client, goalID, *change.OrderNo)
	case "update_title":
		goalID := resolveCodexGoalID(change.GoalID, idMap)
		if change.Title == "" {
			return fmt.Errorf("update_title requires title")
		}
		return codexUpdateObjectiveTitle(ctx, client, goalID, change.Title)
	case "update_status":
		return applyCodexStatusChange(ctx, client, change, idMap)
	case "delete_goal":
		goalID := resolveCodexGoalID(change.GoalID, idMap)
		return codexDeleteObjectives(ctx, client, []string{goalID})
	default:
		return fmt.Errorf("unknown change type: %q", change.Type)
	}
}

func applyCodexCreateChange(
	ctx context.Context,
	client *AddnessClient,
	date string,
	change addnessCodexApplyChange,
	idMap map[string]string,
) error {
	if change.TempID == "" {
		return fmt.Errorf("create_goal requires temp_id")
	}
	if change.Title == "" {
		return fmt.Errorf("create_goal requires title")
	}
	orderNo := 0.0
	if change.OrderNo != nil {
		orderNo = *change.OrderNo
	}
	var parentID *string
	if change.ParentID != nil && *change.ParentID != "" {
		resolved := resolveCodexGoalID(*change.ParentID, idMap)
		parentID = &resolved
	}
	createdID, err := codexCreateObjectiveForDate(ctx, client, date, change.Title, parentID, orderNo)
	if err != nil {
		return err
	}
	idMap[change.TempID] = createdID
	return nil
}

func applyCodexStatusChange(
	ctx context.Context,
	client *AddnessClient,
	change addnessCodexApplyChange,
	idMap map[string]string,
) error {
	goalID := resolveCodexGoalID(change.GoalID, idMap)
	if change.ExecutionID != "" {
		execID := resolveCodexGoalID(change.ExecutionID, idMap)
		return codexUpdateExecutionStatusFields(ctx, client, execID, change.Status, change.CompletedAt)
	}
	if change.CompletedAt != nil && *change.CompletedAt != "" {
		undo := false
		return codexCompleteObjectiveNow(ctx, client, goalID, undo)
	}
	if change.CompletedAt != nil && *change.CompletedAt == "" {
		return codexCompleteObjectiveNow(ctx, client, goalID, true)
	}
	return codexUpdateObjectiveStatusFields(ctx, client, goalID, change.Status, change.CompletedAt)
}

func fetchAddnessCodexTodaysGoalsView(
	ctx context.Context,
	client *AddnessClient,
	date string,
	memberID string,
) (addnessCodexTodaysGoalsViewPayload, error) {
	path := fmt.Sprintf("/api/v2/organizations/%s/todays-goals?date=%s", client.OrganizationID(), date)
	viewingMemberID := client.MemberID()
	if memberID != "" {
		resolved, err := client.ids.Resolve(memberID)
		if err != nil {
			return addnessCodexTodaysGoalsViewPayload{}, err
		}
		viewingMemberID = resolved
		path += "&member_id=" + resolved
	} else if myID := client.MemberID(); myID != "" {
		path += "&member_id=" + myID
	}
	data, err := client.Get(ctx, path)
	if err != nil {
		return addnessCodexTodaysGoalsViewPayload{}, err
	}
	return parseAddnessCodexTodaysGoalsView(data, client.ids, date, viewingMemberID)
}

func runApplyTodaysGoalsChangesCLI() error {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	var request addnessCodexApplyRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("invalid stdin JSON: %w", err)
	}

	baseURL := os.Getenv("ADDNESS_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	ids := NewShortIDCache()
	client := NewAddnessClient(baseURL, ids)
	if token := os.Getenv("ADDNESS_API_TOKEN"); token != "" {
		client.SetToken(token)
	} else if token := os.Getenv("ADDNESS_TOKEN"); token != "" {
		client.SetToken(token)
	} else if token := os.Getenv("ADDNESS_API_KEY"); token != "" {
		client.SetToken(token)
	}

	result, err := applyAddnessCodexTodaysGoalsChanges(context.Background(), client, request)
	if err != nil {
		if failure, ok := err.(*applyChangesError); ok {
			out, marshalErr := json.Marshal(failure.body)
			if marshalErr != nil {
				return marshalErr
			}
			_, _ = os.Stdout.Write(out)
			os.Exit(1)
		}
		return err
	}
	out, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}
