package handlers

import (
	"reflect"
	"testing"

	"github.com/praetordev/launch"
)

func TestRestoreRelaunchPromptsUsesOnlyExplicitPreviousAnswers(t *testing.T) {
	inventoryID := int64(11)
	credentialID := int64(13)
	limit := "web-*"
	req := jobLaunchRequest{}
	restoreRelaunchPrompts(&req, launch.Options{
		SnapshotVersion:    launch.CurrentSnapshotVersion,
		InventoryID:        &inventoryID,
		CredentialID:       &credentialID,
		ExtraVars:          map[string]interface{}{"template_default": true, "answer": "old"},
		Limit:              &limit,
		PromptedInventory:  true,
		PromptedCredential: false,
		PromptedExtraVars:  map[string]interface{}{"answer": "old"},
		PromptedLimit:      &limit,
	})

	if req.InventoryID == nil || *req.InventoryID != inventoryID {
		t.Fatalf("inventory answer was not restored: %#v", req)
	}
	if req.CredentialID != nil {
		t.Fatalf("template-default credential was incorrectly restored as a prompt: %#v", req)
	}
	if !reflect.DeepEqual(req.ExtraVars, map[string]interface{}{"answer": "old"}) {
		t.Fatalf("relaunch variables = %#v", req.ExtraVars)
	}
	if req.Limit == nil || *req.Limit != limit {
		t.Fatalf("relaunch limit = %#v", req.Limit)
	}
}

func TestRestoreRelaunchPromptsPreservesNewCallerAnswers(t *testing.T) {
	oldInventoryID := int64(11)
	newInventoryID := int64(21)
	oldLimit := "old-*"
	newLimit := "new-*"
	req := jobLaunchRequest{
		InventoryID: &newInventoryID,
		ExtraVars:   map[string]interface{}{"answer": "new"},
		Limit:       &newLimit,
	}
	restoreRelaunchPrompts(&req, launch.Options{
		SnapshotVersion:   launch.CurrentSnapshotVersion,
		InventoryID:       &oldInventoryID,
		PromptedInventory: true,
		PromptedExtraVars: map[string]interface{}{"answer": "old"},
		PromptedLimit:     &oldLimit,
	})
	if req.InventoryID == nil || *req.InventoryID != newInventoryID ||
		req.ExtraVars["answer"] != "new" || req.Limit == nil || *req.Limit != newLimit {
		t.Fatalf("new relaunch answers were overwritten: %#v", req)
	}
}
