package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type addnessCodexListOrganizationsPayload struct {
	Version                int                            `json:"version"`
	Organizations          []addnessCodexOrganizationItem `json:"organizations"`
	ActiveOrganizationID   string                         `json:"activeOrganizationId,omitempty"`
	ActiveOrganizationName string                         `json:"activeOrganizationName,omitempty"`
}

type addnessCodexOrganizationItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Plan string `json:"plan,omitempty"`
}

type addnessCodexSwitchOrganizationRequest struct {
	Version        int    `json:"version"`
	OrganizationID string `json:"organization_id"`
}

type addnessCodexSwitchOrganizationResult struct {
	OK               bool   `json:"ok"`
	OrganizationID   string `json:"organizationId,omitempty"`
	OrganizationName string `json:"organizationName,omitempty"`
	Error            string `json:"error,omitempty"`
}

func newAddnessCodexClientFromEnv() (*AddnessClient, error) {
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
	return client, nil
}

func fetchAddnessCodexOrganizations(ctx context.Context, client *AddnessClient) (addnessCodexListOrganizationsPayload, error) {
	data, err := client.Get(ctx, "/api/v2/organizations/me")
	if err != nil {
		return addnessCodexListOrganizationsPayload{}, fmt.Errorf("list organizations: %w", err)
	}
	orgs, err := parseOrganizations(data, client.ids)
	if err != nil {
		return addnessCodexListOrganizationsPayload{}, fmt.Errorf("parse organizations: %w", err)
	}

	items := make([]addnessCodexOrganizationItem, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, addnessCodexOrganizationItem{
			ID:   org.ID,
			Name: org.Name,
			Plan: org.Plan,
		})
	}

	payload := addnessCodexListOrganizationsPayload{
		Version:       1,
		Organizations: items,
	}
	if activeID := client.OrganizationID(); activeID != "" {
		payload.ActiveOrganizationID = client.ids.Shorten(activeID)
		for _, org := range orgs {
			if org.fullID == activeID {
				payload.ActiveOrganizationName = org.Name
				break
			}
		}
	}
	return payload, nil
}

func runListOrganizationsCLI() error {
	client, err := newAddnessCodexClientFromEnv()
	if err != nil {
		return err
	}
	payload, err := fetchAddnessCodexOrganizations(context.Background(), client)
	if err != nil {
		return err
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

func runSwitchOrganizationCLI() error {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	var request addnessCodexSwitchOrganizationRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("invalid stdin JSON: %w", err)
	}
	if request.Version != 1 {
		return fmt.Errorf("unsupported version: %d", request.Version)
	}
	if request.OrganizationID == "" {
		return fmt.Errorf("organization_id is required")
	}

	client, err := newAddnessCodexClientFromEnv()
	if err != nil {
		return err
	}

	ctx := context.Background()
	previousOrgID := client.OrganizationID()
	previousMemberID := client.MemberID()
	selectedOrg, err := resolveAddnessCodexOrganization(ctx, client, request.OrganizationID)
	if err != nil {
		return err
	}

	client.SetOrganization(selectedOrg.fullID)
	if memberData, err := client.Get(ctx, "/api/v2/members?pageSize=100"); err == nil {
		if mid := findCurrentMemberID(memberData); mid != "" {
			client.SetMemberID(mid)
		}
	}

	if _, err := fetchAddnessCodexOrganizations(ctx, client); err != nil {
		client.restoreSession(previousOrgID, previousMemberID)
		return err
	}
	result := addnessCodexSwitchOrganizationResult{
		OK:               true,
		OrganizationID:   client.ids.Shorten(client.OrganizationID()),
		OrganizationName: selectedOrg.Name,
	}
	out, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

func resolveAddnessCodexOrganization(ctx context.Context, client *AddnessClient, organizationID string) (orgInfo, error) {
	data, err := client.Get(ctx, "/api/v2/organizations/me")
	if err != nil {
		return orgInfo{}, fmt.Errorf("list organizations: %w", err)
	}
	orgs, err := parseOrganizations(data, client.ids)
	if err != nil {
		return orgInfo{}, fmt.Errorf("parse organizations: %w", err)
	}

	resolvedID := client.ids.resolveOrFallback(organizationID)
	for _, org := range orgs {
		if org.fullID == resolvedID || org.ID == organizationID {
			return org, nil
		}
	}
	return orgInfo{}, fmt.Errorf("organization_id not found: %s", organizationID)
}
