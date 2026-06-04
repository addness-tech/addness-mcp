package main

import "testing"

func TestResolveCodexGoalID(t *testing.T) {
	idMap := map[string]string{"tmp-1": "real-1"}
	if got := resolveCodexGoalID("tmp-1", idMap); got != "real-1" {
		t.Fatalf("expected mapped id, got %q", got)
	}
	if got := resolveCodexGoalID("other", idMap); got != "other" {
		t.Fatalf("expected passthrough id, got %q", got)
	}
}

func TestApplyAddnessCodexRequestValidation(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		if _, err := applyAddnessCodexTodaysGoalsChanges(t.Context(), &AddnessClient{}, addnessCodexApplyRequest{
			Version: 0,
			Date:    "2026-06-04",
		}); err == nil || err.Error() != "unsupported version: 0" {
			t.Fatalf("expected version error, got %v", err)
		}
	})
}
