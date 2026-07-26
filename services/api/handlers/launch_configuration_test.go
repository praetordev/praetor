package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	rbac "github.com/praetordev/praetor/pkg/accesscontrol"
	"github.com/praetordev/praetor/services/api/handlers"
	"github.com/praetordev/praetor/services/api/middleware"
)

func TestLaunchConfigurationFiltersResourcesAndPreviewsEffectiveInputs(t *testing.T) {
	db := rbacTestDB(t)
	defer db.Close()
	resource := handlers.NewTemplatesResource(db, handlers.NewAuthorizer(db))
	access := rbac.NewStore(db, testResourceTables)

	uniq := time.Now().UnixNano()
	orgID := createOrg(t, db, fmt.Sprintf("launch-config-org-%d", uniq))
	otherOrgID := createOrg(t, db, fmt.Sprintf("launch-config-other-%d", uniq))
	operatorID := createUser(t, db, fmt.Sprintf("launch-config-operator-%d", uniq))
	var projectID, allowedInventoryID, hiddenInventoryID, foreignInventoryID int64
	mustScan := func(target *int64, query string, args ...interface{}) {
		t.Helper()
		if err := db.QueryRow(query, args...).Scan(target); err != nil {
			t.Fatalf("seed launch configuration: %v", err)
		}
	}
	mustScan(&projectID,
		`INSERT INTO projects (organization_id,name,scm_type,scm_url) VALUES ($1,$2,'git','https://example.invalid/launch.git') RETURNING id`,
		orgID, fmt.Sprintf("launch-project-%d", uniq))
	mustScan(&allowedInventoryID,
		`INSERT INTO inventories (organization_id,name) VALUES ($1,$2) RETURNING id`,
		orgID, fmt.Sprintf("allowed-inventory-%d", uniq))
	mustScan(&hiddenInventoryID,
		`INSERT INTO inventories (organization_id,name) VALUES ($1,$2) RETURNING id`,
		orgID, fmt.Sprintf("hidden-inventory-%d", uniq))
	mustScan(&foreignInventoryID,
		`INSERT INTO inventories (organization_id,name) VALUES ($1,$2) RETURNING id`,
		otherOrgID, fmt.Sprintf("foreign-inventory-%d", uniq))
	if _, err := db.Exec(`INSERT INTO hosts (inventory_id,name,enabled) VALUES ($1,'app-01',true),($1,'app-02',true),($1,'disabled',false)`, allowedInventoryID); err != nil {
		t.Fatalf("seed hosts: %v", err)
	}

	var machineTypeID, allowedCredentialID, hiddenCredentialID, foreignCredentialID int64
	mustScan(&machineTypeID, `SELECT id FROM credential_types WHERE lower(name)='machine' ORDER BY id LIMIT 1`)
	mustScan(&allowedCredentialID,
		`INSERT INTO credentials (organization_id,credential_type_id,name,inputs) VALUES ($1,$2,$3,'{"username":"operator","password":"secret"}') RETURNING id`,
		orgID, machineTypeID, fmt.Sprintf("allowed-credential-%d", uniq))
	mustScan(&hiddenCredentialID,
		`INSERT INTO credentials (organization_id,credential_type_id,name) VALUES ($1,$2,$3) RETURNING id`,
		orgID, machineTypeID, fmt.Sprintf("hidden-credential-%d", uniq))
	mustScan(&foreignCredentialID,
		`INSERT INTO credentials (organization_id,credential_type_id,name) VALUES ($1,$2,$3) RETURNING id`,
		otherOrgID, machineTypeID, fmt.Sprintf("foreign-credential-%d", uniq))

	admin := middleware.UserContext{UserID: operatorID, IsSuperuser: true}
	createBody := fmt.Sprintf(`{
		"organization_id":%d,
		"name":"prompt-template-%d",
		"project_id":%d,
		"inventory_id":%d,
		"credential_id":%d,
		"playbook":"site.yml",
		"extra_vars":{"saved":"value"},
		"limit":"all",
		"ask_inventory_on_launch":true,
		"ask_credential_on_launch":true,
		"ask_variables_on_launch":true,
		"ask_limit_on_launch":true
	}`, orgID, uniq, projectID, allowedInventoryID, allowedCredentialID)
	rec := callJSON(t, resource.CreateTemplate, http.MethodPost, createBody, admin, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create prompt template: want 201, got %d (%s)", rec.Code, rec.Body)
	}
	var created struct {
		ID                    int64 `json:"id"`
		UnifiedJobTemplateID  int64 `json:"unified_job_template_id"`
		AskInventoryOnLaunch  bool  `json:"ask_inventory_on_launch"`
		AskCredentialOnLaunch bool  `json:"ask_credential_on_launch"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil ||
		!created.AskInventoryOnLaunch || !created.AskCredentialOnLaunch {
		t.Fatalf("created template prompt flags: %#v err=%v body=%s", created, err, rec.Body)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM organizations WHERE id IN ($1,$2)`, orgID, otherOrgID)
		_, _ = db.Exec(`DELETE FROM unified_job_templates WHERE id=$1`, created.UnifiedJobTemplateID)
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, operatorID)
	})

	grantObjectRole(t, access, rbac.JobTemplate, created.ID, rbac.ExecuteRole, operatorID)
	grantObjectRole(t, access, rbac.Inventory, allowedInventoryID, rbac.UseRole, operatorID)
	grantObjectRole(t, access, rbac.Credential, allowedCredentialID, rbac.UseRole, operatorID)
	operator := middleware.UserContext{UserID: operatorID}
	params := map[string]string{"id": fmt.Sprint(created.ID)}

	rec = callJSON(t, resource.GetLaunchConfiguration, http.MethodGet, "", operator, params)
	if rec.Code != http.StatusOK {
		t.Fatalf("get launch configuration: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	var configuration struct {
		Inventories []struct {
			ID int64 `json:"id"`
		} `json:"inventories"`
		Credentials []struct {
			ID int64 `json:"id"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &configuration); err != nil {
		t.Fatalf("decode launch configuration: %v", err)
	}
	if len(configuration.Inventories) != 1 || configuration.Inventories[0].ID != allowedInventoryID {
		t.Fatalf("inventory choices = %#v, want only %d", configuration.Inventories, allowedInventoryID)
	}
	if len(configuration.Credentials) != 1 || configuration.Credentials[0].ID != allowedCredentialID {
		t.Fatalf("credential choices = %#v, want only %d", configuration.Credentials, allowedCredentialID)
	}
	for _, forbidden := range []string{"password", "username", "secret", "inputs", "secrets_service"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), forbidden) {
			t.Fatalf("launch configuration exposed forbidden credential material %q: %s", forbidden, rec.Body)
		}
	}

	previewBody := fmt.Sprintf(`{
		"inventory_id":%d,
		"credential_id":%d,
		"extra_vars":{"release":"canary"},
		"limit":"app-*"
	}`, allowedInventoryID, allowedCredentialID)
	rec = callJSON(t, resource.PreviewLaunch, http.MethodPost, previewBody, operator, params)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview authorized launch: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	var preview struct {
		Inventory struct {
			ID int64 `json:"id"`
		} `json:"inventory"`
		Credential struct {
			ID int64 `json:"id"`
		} `json:"credential"`
		ExtraVars           map[string]interface{} `json:"extra_vars"`
		Limit               string                 `json:"limit"`
		InventoryHostCount  int64                  `json:"inventory_host_count"`
		InventoryHostSample []string               `json:"inventory_host_sample"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode launch preview: %v", err)
	}
	if preview.Inventory.ID != allowedInventoryID || preview.Credential.ID != allowedCredentialID ||
		preview.ExtraVars["saved"] != "value" || preview.ExtraVars["release"] != "canary" ||
		preview.Limit != "app-*" || preview.InventoryHostCount != 2 || len(preview.InventoryHostSample) != 2 {
		t.Fatalf("unexpected launch preview: %#v", preview)
	}

	inventoryUseName, _ := rbac.BuiltinRoleName(rbac.Inventory, rbac.UseRole)
	inventoryUse, err := access.RoleByName(context.Background(), inventoryUseName)
	if err != nil {
		t.Fatalf("find inventory use role: %v", err)
	}
	inventoryResource := rbac.Object(rbac.Inventory, allowedInventoryID)
	inventoryAssignment := rbac.Assignment{
		RoleDefinitionID: inventoryUse.ID,
		Resource:         &inventoryResource,
		PrincipalKind:    rbac.UserPrincipal,
		PrincipalID:      operatorID,
	}
	if err := access.Revoke(context.Background(), inventoryAssignment); err != nil {
		t.Fatalf("revoke inventory use: %v", err)
	}
	rec = callJSON(t, resource.PreviewLaunch, http.MethodPost, previewBody, operator, params)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("preview after use revocation: want 403, got %d (%s)", rec.Code, rec.Body)
	}
	if err := access.Assign(context.Background(), inventoryAssignment); err != nil {
		t.Fatalf("restore inventory use: %v", err)
	}

	for name, body := range map[string]string{
		"same-org inventory without use":  fmt.Sprintf(`{"inventory_id":%d}`, hiddenInventoryID),
		"foreign inventory":               fmt.Sprintf(`{"inventory_id":%d}`, foreignInventoryID),
		"same-org credential without use": fmt.Sprintf(`{"credential_id":%d}`, hiddenCredentialID),
		"foreign credential":              fmt.Sprintf(`{"credential_id":%d}`, foreignCredentialID),
	} {
		t.Run(name, func(t *testing.T) {
			rec := callJSON(t, resource.PreviewLaunch, http.MethodPost, body, operator, params)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("want 403, got %d (%s)", rec.Code, rec.Body)
			}
		})
	}

	if _, err := db.Exec(`UPDATE job_templates SET ask_inventory_on_launch=false WHERE id=$1`, created.ID); err != nil {
		t.Fatalf("disable inventory prompt: %v", err)
	}
	rec = callJSON(t, resource.PreviewLaunch, http.MethodPost,
		fmt.Sprintf(`{"inventory_id":%d}`, allowedInventoryID), operator, params)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("disabled inventory prompt: want 400, got %d (%s)", rec.Code, rec.Body)
	}

	if _, err := db.Exec(`UPDATE job_templates
		SET ask_inventory_on_launch=true, survey_enabled=true, ask_variables_on_launch=false,
		    survey_spec='{"spec":[{"variable":"change_ticket","type":"text","required":true}]}'
		WHERE id=$1`, created.ID); err != nil {
		t.Fatalf("enable required survey: %v", err)
	}
	rec = callJSON(t, resource.PreviewLaunch, http.MethodPost, `{}`, operator, params)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "change_ticket") {
		t.Fatalf("missing required survey answer: want descriptive 400, got %d (%s)", rec.Code, rec.Body)
	}
	rec = callJSON(t, resource.PreviewLaunch, http.MethodPost,
		`{"extra_vars":{"change_ticket":"CHG-42"},"limit":"web\nall"}`, operator, params)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("control character in host limit: want 400, got %d (%s)", rec.Code, rec.Body)
	}

	nobody := createUser(t, db, fmt.Sprintf("launch-config-nobody-%d", uniq))
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id=$1`, nobody) })
	rec = callJSON(t, resource.GetLaunchConfiguration, http.MethodGet, "",
		middleware.UserContext{UserID: nobody}, params)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("configuration without execute: want 403, got %d (%s)", rec.Code, rec.Body)
	}
}
