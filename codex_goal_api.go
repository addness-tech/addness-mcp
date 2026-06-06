package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// codexCreateObjectiveForDate は今日のゴールに objective を作成して追加する。
func codexCreateObjectiveForDate(
	ctx context.Context,
	client *AddnessClient,
	date string,
	title string,
	parentObjectiveID *string,
	orderNo float64,
) (string, error) {
	if err := requireOrg(client); err != nil {
		return "", err
	}
	memberID := client.MemberID()
	if memberID == "" {
		return "", fmt.Errorf("member ID not resolved: use switch_organization first")
	}

	body := map[string]any{
		"title":             title,
		"organizationId":    client.OrganizationID(),
		"parentObjectiveId": parentObjectiveID,
		"orderNo":           orderNo,
		"ownerId":           memberID,
		"date":              date,
	}
	data, err := client.Post(ctx, "/api/v2/objective/create", body)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
		ID string `json:"id"`
	}
	if err := decodeAPIPayload(data, &parsed); err != nil {
		return "", fmt.Errorf("parse create objective response: %w", err)
	}
	id := parsed.Data.ID
	if id == "" {
		id = parsed.ID
	}
	if id == "" {
		return "", fmt.Errorf("create objective response missing id")
	}
	return client.ids.Shorten(id), nil
}

func codexPatchObjective(
	ctx context.Context,
	client *AddnessClient,
	goalID string,
	body map[string]any,
) error {
	fullID, err := client.ids.Resolve(goalID)
	if err != nil {
		return err
	}
	_, err = client.Patch(ctx, fmt.Sprintf("/api/v2/objectives/%s", fullID), body)
	return err
}

func codexUpdateObjectiveTitle(ctx context.Context, client *AddnessClient, goalID, title string) error {
	return codexPatchObjective(ctx, client, goalID, map[string]any{"title": title})
}

func codexUpdateObjectiveOrderNo(ctx context.Context, client *AddnessClient, goalID string, orderNo float64) error {
	return codexPatchObjective(ctx, client, goalID, map[string]any{"orderNo": orderNo})
}

func codexUpdateObjectiveStatusFields(
	ctx context.Context,
	client *AddnessClient,
	goalID string,
	status *string,
	completedAt *string,
) error {
	body := map[string]any{}
	if status != nil {
		body["status"] = *status
	}
	if completedAt != nil {
		if *completedAt == "" {
			body["completedAt"] = nil
		} else {
			body["completedAt"] = *completedAt
		}
	}
	if len(body) == 0 {
		return fmt.Errorf("no status fields to update")
	}
	return codexPatchObjective(ctx, client, goalID, body)
}

func codexUpdateExecutionStatusFields(
	ctx context.Context,
	client *AddnessClient,
	executionID string,
	status *string,
	completedAt *string,
) error {
	fullID, err := client.ids.Resolve(executionID)
	if err != nil {
		return err
	}
	body := map[string]any{}
	if status != nil {
		body["status"] = *status
	}
	if completedAt != nil {
		if *completedAt == "" {
			body["completedAt"] = nil
		} else {
			body["completedAt"] = *completedAt
		}
	}
	if len(body) == 0 {
		return fmt.Errorf("no execution fields to update")
	}
	_, err = client.Put(ctx, fmt.Sprintf("/api/v2/execute-goals/%s", fullID), body)
	return err
}

func resolveCodexGoalID(id string, idMap map[string]string) string {
	if mapped, ok := idMap[id]; ok {
		return mapped
	}
	return id
}

func codexMoveObjectiveParent(
	ctx context.Context,
	client *AddnessClient,
	goalID string,
	newParentID string,
	idMap map[string]string,
	orderNo float64,
) error {
	fullGoalID, err := client.ids.Resolve(resolveCodexGoalID(goalID, idMap))
	if err != nil {
		return err
	}
	body := map[string]any{"orderNo": orderNo}
	if newParentID != "" {
		fullParentID, err := client.ids.Resolve(resolveCodexGoalID(newParentID, idMap))
		if err != nil {
			return err
		}
		body["newParentId"] = fullParentID
	} else {
		body["newParentId"] = nil
	}
	_, err = client.Post(ctx, fmt.Sprintf("/api/v2/objectives/%s/parent", fullGoalID), body)
	return err
}

func codexDeleteObjectives(ctx context.Context, client *AddnessClient, goalIDs []string) error {
	resolved := make([]string, 0, len(goalIDs))
	for _, id := range goalIDs {
		fullID, err := client.ids.Resolve(id)
		if err != nil {
			return err
		}
		resolved = append(resolved, fullID)
	}
	body := map[string]any{"objectiveIds": resolved}
	_, err := client.Delete(ctx, "/api/v2/objectives/delete", body)
	return err
}

func codexCompleteObjectiveNow(ctx context.Context, client *AddnessClient, goalID string, undo bool) error {
	var completedAt any
	if undo {
		completedAt = nil
	} else {
		completedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return codexPatchObjective(ctx, client, goalID, map[string]any{"completedAt": completedAt})
}

func decodeAPIPayload(data []byte, target any) error {
	unwrapped := unwrapData(data)
	return json.Unmarshal(unwrapped, target)
}
