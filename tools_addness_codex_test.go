package main

import (
	"encoding/json"
	"testing"
)

func TestAddnessCodexToolRegistrationFlags(t *testing.T) {
	t.Setenv("ADDNESS_MCP_ENABLE_ADDNESS_CODEX_TOOLS", "")
	t.Setenv("ADDNESS_MCP_ONLY_ADDNESS_CODEX_TOOLS", "")
	if !shouldRegisterDefaultTools() {
		t.Fatal("default tools should be registered without Addness Codex env flags")
	}
	if shouldRegisterAddnessCodexTools() {
		t.Fatal("Addness Codex tools should not be registered without Addness Codex env flags")
	}

	t.Setenv("ADDNESS_MCP_ENABLE_ADDNESS_CODEX_TOOLS", "1")
	if !shouldRegisterDefaultTools() {
		t.Fatal("default tools should stay registered when Addness Codex tools are added")
	}
	if !shouldRegisterAddnessCodexTools() {
		t.Fatal("Addness Codex tools should be registered when ADDNESS_MCP_ENABLE_ADDNESS_CODEX_TOOLS=1")
	}

	t.Setenv("ADDNESS_MCP_ENABLE_ADDNESS_CODEX_TOOLS", "")
	t.Setenv("ADDNESS_MCP_ONLY_ADDNESS_CODEX_TOOLS", "1")
	if shouldRegisterDefaultTools() {
		t.Fatal("default tools should not be registered in Addness Codex-only mode")
	}
	if !shouldRegisterAddnessCodexTools() {
		t.Fatal("Addness Codex tools should be registered in Addness Codex-only mode")
	}
}

func TestParseAddnessCodexTodaysGoalsView(t *testing.T) {
	ids := NewShortIDCache()
	payload, err := parseAddnessCodexTodaysGoalsView([]byte(`{
		"data": {
			"nodes": [
				{
					"id": "goal-root-000000000000000000000001",
					"parentId": null,
					"depth": 0,
					"title": "最優先でさばくタスク",
					"status": "IN_PROGRESS",
					"completedAt": null,
					"orderNo": 10,
					"isLeaf": false,
					"hasRecurring": true,
					"isRecurring": false,
					"owner": {
						"name": "Kodai Hayashida",
						"avatarUrl": "https://example.com/kodai.png"
					},
					"unresolvedCommentCount": 2
				},
				{
					"id": "goal-child-000000000000000000000001",
					"parentId": "goal-root-000000000000000000000001",
					"depth": 1,
					"title": "PRレビューコメントとCI失敗を解消する",
					"status": "NONE",
					"completedAt": null,
					"orderNo": 20,
					"isLeaf": true,
					"hasRecurring": false,
					"isRecurring": true,
					"execution": {
						"id": "execution-000000000000000000000001",
						"status": "COMPLETED",
						"completedAt": "2026-06-04T01:02:03Z"
					},
					"owner": {
						"name": "智也",
						"avatar": {
							"thumbnailUrl": "https://example.com/tomoya-thumb.png"
						}
					},
					"unresolvedCommentCount": 1
				}
			]
		}
	}`), ids, "2026-06-04")
	if err != nil {
		t.Fatalf("parseAddnessCodexTodaysGoalsView returned error: %v", err)
	}

	if payload.Version != 1 || payload.Date != "2026-06-04" || payload.Title != "今日のゴール" {
		t.Fatalf("unexpected payload header: %#v", payload)
	}
	if payload.Meta.Source != "addness-mcp:addness_codex_get_todays_goals_view" {
		t.Fatalf("unexpected source: %q", payload.Meta.Source)
	}
	if len(payload.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(payload.Nodes))
	}

	root := payload.Nodes[0]
	if root.ParentID != nil {
		t.Fatalf("root parentId should be nil, got %q", *root.ParentID)
	}
	if root.ChildCount != 1 {
		t.Fatalf("expected root child count 1, got %d", root.ChildCount)
	}
	if root.OwnerName == nil || *root.OwnerName != "Kodai Hayashida" {
		t.Fatalf("unexpected root owner: %#v", root.OwnerName)
	}
	if root.OwnerAvatarURL == nil || *root.OwnerAvatarURL != "https://example.com/kodai.png" {
		t.Fatalf("unexpected root avatar: %#v", root.OwnerAvatarURL)
	}
	if root.UnresolvedCommentCount == nil || *root.UnresolvedCommentCount != 2 {
		t.Fatalf("unexpected root comment count: %#v", root.UnresolvedCommentCount)
	}

	child := payload.Nodes[1]
	if child.ParentID == nil || *child.ParentID != root.ID {
		t.Fatalf("child parentId should be root short id %q, got %#v", root.ID, child.ParentID)
	}
	if child.CompletedAt == nil || *child.CompletedAt != "2026-06-04T01:02:03Z" {
		t.Fatalf("child completedAt should fall back to execution completedAt, got %#v", child.CompletedAt)
	}
	if child.ExecutionID == nil || child.ExecutionStatus == nil || *child.ExecutionStatus != "COMPLETED" {
		t.Fatalf("unexpected execution fields: id=%#v status=%#v", child.ExecutionID, child.ExecutionStatus)
	}
	if child.OwnerAvatarURL == nil || *child.OwnerAvatarURL != "https://example.com/tomoya-thumb.png" {
		t.Fatalf("unexpected child avatar fallback: %#v", child.OwnerAvatarURL)
	}

	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("payload should be JSON serializable: %v", err)
	}
}
