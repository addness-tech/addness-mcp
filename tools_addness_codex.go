package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type addnessCodexGetViewRequest struct {
	Version  int    `json:"version"`
	Date     string `json:"date,omitempty"`
	MemberID string `json:"member_id,omitempty"`
}

func addnessCodexGetTodaysGoalsViewTool() mcp.Tool {
	return mcp.NewTool("addness_codex_get_todays_goals_view",
		mcp.WithDescription("Addness Codex専用。today goals UI表示用に、階層順・owner・コメント数・子数を含む構造化JSONを取得するread-only tool。"),
		mcp.WithString("date",
			mcp.Description("Date in YYYY-MM-DD format (default: today)"),
		),
		mcp.WithString("member_id",
			mcp.Description("Member ID (short ID) to view another member's goals. Omit to see your own goals only."),
		),
	)
}

func handleAddnessCodexGetTodaysGoalsView(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		date := argStr(args, "date")
		if date == "" {
			date = currentActivityDateString(defaultActivityTimezone, defaultActivityCutoffHour)
		}

		path := fmt.Sprintf("/api/v2/organizations/%s/todays-goals?date=%s", client.OrganizationID(), url.QueryEscape(date))
		viewingMemberID := client.MemberID()
		if memberID := argStr(args, "member_id"); memberID != "" {
			resolved, err := client.ids.Resolve(memberID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			viewingMemberID = resolved
			path += "&member_id=" + url.QueryEscape(resolved)
		} else if myID := client.MemberID(); myID != "" {
			path += "&member_id=" + url.QueryEscape(myID)
		}

		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		payload, err := parseAddnessCodexTodaysGoalsView(data, client.ids, date, viewingMemberID)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		out, err := json.Marshal(payload)
		if err != nil {
			return errResult(fmt.Sprintf("marshal error: %v", err)), nil
		}
		return textResult(string(out)), nil
	}
}

type addnessCodexTodaysGoalsViewPayload struct {
	Version int                               `json:"version"`
	Date    string                            `json:"date"`
	Title   string                            `json:"title"`
	Nodes   []addnessCodexTodaysGoalsViewNode `json:"nodes"`
	Meta    addnessCodexTodaysGoalsViewMeta   `json:"meta"`
}

type addnessCodexTodaysGoalsViewMeta struct {
	Source string `json:"source"`
}

type addnessCodexTodaysGoalsViewNode struct {
	ID                     string   `json:"id"`
	ParentID               *string  `json:"parentId"`
	Depth                  int      `json:"depth"`
	Title                  string   `json:"title"`
	Status                 *string  `json:"status"`
	CompletedAt            *string  `json:"completedAt"`
	OrderNo                float64  `json:"orderNo"`
	OwnerName              *string  `json:"ownerName,omitempty"`
	OwnerAvatarURL         *string  `json:"ownerAvatarUrl,omitempty"`
	OwnerMemberID          *string  `json:"ownerMemberId,omitempty"`
	UnresolvedCommentCount *int     `json:"unresolvedCommentCount,omitempty"`
	ChildCount             int      `json:"childCount"`
	IsLeaf                 bool     `json:"isLeaf"`
	HasRecurring           bool     `json:"hasRecurring"`
	IsRecurring            bool     `json:"isRecurring"`
	IsContext              bool     `json:"isContext"`
	ExecutionID            *string  `json:"executionId,omitempty"`
	ExecutionStatus        *string  `json:"executionStatus,omitempty"`
	Permissions            []string `json:"permissions,omitempty"`
}

func parseAddnessCodexTodaysGoalsView(data []byte, ids *ShortIDCache, date string, viewingMemberID string) (addnessCodexTodaysGoalsViewPayload, error) {
	raw, err := parseAddnessCodexTodaysGoalsViewNodes(data)
	if err != nil {
		return addnessCodexTodaysGoalsViewPayload{}, err
	}

	nodes := make([]addnessCodexTodaysGoalsViewNode, 0, len(raw))
	childCounts := make(map[string]int)
	for _, nm := range raw {
		fullID, _ := nm["id"].(string)
		shortID := ids.Shorten(fullID)
		parentID := shortenOptionalID(ids, stringField(nm, "parentId"))
		if parentID != nil {
			childCounts[*parentID]++
		}

		completedAt := stringPtrField(nm, "completedAt")
		executionID, executionStatus, executionCompletedAt := executionFields(nm, ids)
		if completedAt == nil {
			completedAt = executionCompletedAt
		}

		ownerName, ownerAvatarURL, ownerMemberID := codexOwnerDisplayFields(nm, viewingMemberID)

		node := addnessCodexTodaysGoalsViewNode{
			ID:                     shortID,
			ParentID:               parentID,
			Depth:                  intNumber(nm, "depth"),
			Title:                  stringField(nm, "title"),
			Status:                 stringPtrField(nm, "status"),
			CompletedAt:            completedAt,
			OrderNo:                floatNumber(nm, "orderNo"),
			OwnerName:              ownerName,
			OwnerAvatarURL:         ownerAvatarURL,
			OwnerMemberID:          ownerMemberID,
			UnresolvedCommentCount: intPtrField(nm, "unresolvedCommentCount"),
			IsLeaf:                 boolField(nm, "isLeaf"),
			HasRecurring:           boolField(nm, "hasRecurring"),
			IsRecurring:            boolField(nm, "isRecurring"),
			IsContext:              codexIsContextNode(nm),
			ExecutionID:            executionID,
			ExecutionStatus:        executionStatus,
			Permissions:            stringSliceField(nm, "permissions"),
		}
		nodes = append(nodes, node)
	}

	for i := range nodes {
		nodes[i].ChildCount = childCounts[nodes[i].ID]
	}

	return addnessCodexTodaysGoalsViewPayload{
		Version: 1,
		Date:    date,
		Title:   "今日のゴール",
		Nodes:   nodes,
		Meta: addnessCodexTodaysGoalsViewMeta{
			Source: "addness-mcp:addness_codex_get_todays_goals_view",
		},
	}, nil
}

func parseAddnessCodexTodaysGoalsViewNodes(data []byte) ([]map[string]any, error) {
	unwrapped := unwrapData(data)

	var raw []map[string]any
	if err := json.Unmarshal(unwrapped, &raw); err == nil {
		return raw, nil
	}

	var wrapper struct {
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal(unwrapped, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Nodes, nil
}

func shortenOptionalID(ids *ShortIDCache, fullID string) *string {
	if fullID == "" {
		return nil
	}
	shortID := ids.Shorten(fullID)
	return &shortID
}

func stringField(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return value
}

func stringPtrField(raw map[string]any, key string) *string {
	value, _ := raw[key].(string)
	if value == "" {
		return nil
	}
	return &value
}

func intNumber(raw map[string]any, key string) int {
	value, _ := raw[key].(float64)
	return int(value)
}

func floatNumber(raw map[string]any, key string) float64 {
	value, _ := raw[key].(float64)
	return value
}

func intPtrField(raw map[string]any, key string) *int {
	value, ok := raw[key].(float64)
	if !ok {
		return nil
	}
	intValue := int(value)
	return &intValue
}

func boolField(raw map[string]any, key string) bool {
	value, _ := raw[key].(bool)
	return value
}

func ownerStringField(raw map[string]any, key string) *string {
	owner, ok := raw["owner"].(map[string]any)
	if !ok {
		return nil
	}
	return stringPtrField(owner, key)
}

// codexOwnerDisplayFields は Codex UI 向けに owner 表示フィールドを返す。
// 閲覧対象メンバー自身のゴールには ownerName / ownerAvatarUrl を付けない（Web の self view と同じ）。
func codexOwnerDisplayFields(raw map[string]any, viewingMemberID string) (*string, *string, *string) {
	ownerMemberID := ownerOrganizationMemberID(raw)
	if ownerMemberID != nil && viewingMemberID != "" && *ownerMemberID == viewingMemberID {
		return nil, nil, ownerMemberID
	}
	return ownerStringField(raw, "name"), ownerAvatarURL(raw), ownerMemberID
}

func ownerOrganizationMemberID(raw map[string]any) *string {
	owner, ok := raw["owner"].(map[string]any)
	if !ok {
		return nil
	}
	return stringPtrField(owner, "organizationMemberId")
}

func codexIsContextNode(raw map[string]any) bool {
	if value, ok := raw["isContext"].(bool); ok {
		return value
	}
	kind := stringField(raw, "kind")
	return kind == "context" || kind == "CONTEXT"
}

func stringSliceField(raw map[string]any, key string) []string {
	value, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(value))
	for _, item := range value {
		str, ok := item.(string)
		if ok && str != "" {
			out = append(out, str)
		}
	}
	return out
}

func ownerAvatarURL(raw map[string]any) *string {
	if value := ownerStringField(raw, "avatarUrl"); value != nil {
		return value
	}

	owner, ok := raw["owner"].(map[string]any)
	if !ok {
		return nil
	}
	avatar, ok := owner["avatar"].(map[string]any)
	if !ok {
		return nil
	}
	if value := stringPtrField(avatar, "url"); value != nil {
		return value
	}
	return stringPtrField(avatar, "thumbnailUrl")
}

func executionFields(raw map[string]any, ids *ShortIDCache) (*string, *string, *string) {
	execution, ok := raw["execution"].(map[string]any)
	if !ok {
		return nil, nil, nil
	}
	return shortenOptionalID(ids, stringField(execution, "id")),
		stringPtrField(execution, "status"),
		stringPtrField(execution, "completedAt")
}

func runGetTodaysGoalsViewCLI() error {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	var request addnessCodexGetViewRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("invalid stdin JSON: %w", err)
	}
	if request.Version != 1 {
		return fmt.Errorf("unsupported version: %d", request.Version)
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

	date := request.Date
	if date == "" {
		date = currentActivityDateString(defaultActivityTimezone, defaultActivityCutoffHour)
	}
	if err := requireOrg(client); err != nil {
		return err
	}

	result, err := fetchAddnessCodexTodaysGoalsView(context.Background(), client, date, request.MemberID)
	if err != nil {
		return err
	}
	out, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}
