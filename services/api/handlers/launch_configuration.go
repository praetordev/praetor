package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"github.com/praetordev/models"
	rbac "github.com/praetordev/praetor/pkg/accesscontrol"
	"github.com/praetordev/render"
)

const launchPreviewHostSampleLimit = 10

var errLaunchResourceUnavailable = errors.New("selected launch resource is unavailable")

type launchInventoryRef struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
	Kind string `db:"kind" json:"kind"`
}

type launchCredentialRef struct {
	ID               int64  `db:"id" json:"id"`
	Name             string `db:"name" json:"name"`
	CredentialTypeID int64  `db:"credential_type_id" json:"credential_type_id"`
	CredentialType   string `db:"credential_type" json:"credential_type"`
}

type launchPromptInput struct {
	InventoryID  *int64                 `json:"inventory_id,omitempty"`
	CredentialID *int64                 `json:"credential_id,omitempty"`
	ExtraVars    map[string]interface{} `json:"extra_vars,omitempty"`
	Limit        *string                `json:"limit,omitempty"`
}

type effectiveLaunchInputs struct {
	Inventory  *launchInventoryRef
	Credential *launchCredentialRef
	ExtraVars  map[string]interface{}
	Limit      string
}

type launchInputResolver struct {
	DB *sqlx.DB
	*Authorizer
}

type launchPreview struct {
	Template struct {
		ID                   int64  `json:"id"`
		UnifiedJobTemplateID int64  `json:"unified_job_template_id"`
		Name                 string `json:"name"`
	} `json:"template"`
	Inventory               *launchInventoryRef    `json:"inventory,omitempty"`
	Credential              *launchCredentialRef   `json:"credential,omitempty"`
	ExtraVars               map[string]interface{} `json:"extra_vars"`
	Limit                   string                 `json:"limit"`
	InventoryHostCount      int64                  `json:"inventory_host_count"`
	InventoryHostSample     []string               `json:"inventory_host_sample"`
	LimitAppliedAtExecution bool                   `json:"limit_applied_at_execution"`
}

// GetLaunchConfiguration GET /api/v1/job-templates/{id}/launch-configuration
// returns policy plus secret-free, use-authorized resource choices. The response
// helps clients render a form; it is never an authorization grant.
func (rs *TemplatesResource) GetLaunchConfiguration(w http.ResponseWriter, r *http.Request) {
	template, ok := rs.launchableTemplate(w, r)
	if !ok {
		return
	}

	inventories, err := rs.launchInventories(r, template.OrganizationID)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}
	credentials, err := rs.launchCredentials(r, template.OrganizationID)
	if err != nil {
		render.ErrInternal(err).Render(w, r)
		return
	}

	render.JSON(w, r, map[string]interface{}{
		"template": map[string]interface{}{
			"id":                      template.ID,
			"unified_job_template_id": template.UnifiedJobTemplateID,
			"name":                    template.Name,
			"organization_id":         template.OrganizationID,
		},
		"prompts": map[string]interface{}{
			"inventory":  template.AskInventoryOnLaunch,
			"credential": template.AskCredentialOnLaunch,
			"variables":  template.AskVariablesOnLaunch,
			"limit":      template.AskLimitOnLaunch,
			"survey":     template.SurveyEnabled,
		},
		"defaults": map[string]interface{}{
			"inventory_id":  template.InventoryID,
			"credential_id": template.CredentialID,
			"extra_vars":    rawObject(template.ExtraVars),
			"limit":         template.JobLimit,
		},
		"survey_spec": template.SurveySpec,
		"inventories": inventories,
		"credentials": credentials,
	})
}

// PreviewLaunch POST /api/v1/job-templates/{id}/launch-preview resolves the
// effective launch without creating a job. LaunchJob must resolve and authorize
// the same inputs again because grants can change immediately after this call.
func (rs *TemplatesResource) PreviewLaunch(w http.ResponseWriter, r *http.Request) {
	template, ok := rs.launchableTemplate(w, r)
	if !ok {
		return
	}
	var input launchPromptInput
	if err := decodeStrictJSON(r, &input); err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}
	preview, err := rs.launchResolver().preview(r, template, input)
	if errors.Is(err, errLaunchResourceUnavailable) {
		// Do not distinguish a missing, cross-org, or unauthorized resource.
		render.ErrForbidden(nil).Render(w, r)
		return
	}
	if err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return
	}
	render.JSON(w, r, preview)
}

func (rs *TemplatesResource) launchResolver() launchInputResolver {
	return launchInputResolver{DB: rs.DB, Authorizer: rs.Authorizer}
}

func (rs *TemplatesResource) launchableTemplate(w http.ResponseWriter, r *http.Request) (models.JobTemplate, bool) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		render.ErrInvalidRequest(err).Render(w, r)
		return models.JobTemplate{}, false
	}
	if !rs.authorize(w, r, rbac.JobTemplate, id, actExecute) {
		return models.JobTemplate{}, false
	}
	template, err := rs.store.Get(r.Context(), id)
	if err != nil || template.UnifiedJobTemplateID == nil {
		render.ErrNotFound(nil).Render(w, r)
		return models.JobTemplate{}, false
	}
	return template, true
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid resource id")
	}
	return id, nil
}

func (rs *TemplatesResource) launchInventories(r *http.Request, organizationID int64) ([]launchInventoryRef, error) {
	ids, err := rs.usableIDs(r, rbac.Inventory)
	if err != nil || len(ids) == 0 {
		return []launchInventoryRef{}, err
	}
	query, args, err := sqlx.In(`
		SELECT id, name, kind FROM inventories
		WHERE organization_id = ? AND id IN (?)
		ORDER BY name, id`, organizationID, ids)
	if err != nil {
		return nil, err
	}
	var refs []launchInventoryRef
	err = rs.DB.SelectContext(r.Context(), &refs, rs.DB.Rebind(query), args...)
	return refs, err
}

func (rs *TemplatesResource) launchCredentials(r *http.Request, organizationID int64) ([]launchCredentialRef, error) {
	ids, err := rs.usableIDs(r, rbac.Credential)
	if err != nil || len(ids) == 0 {
		return []launchCredentialRef{}, err
	}
	query, args, err := sqlx.In(`
		SELECT c.id, c.name, c.credential_type_id, ct.name AS credential_type
		FROM credentials c
		JOIN credential_types ct ON ct.id = c.credential_type_id
		WHERE c.organization_id = ? AND c.id IN (?) AND lower(ct.name) = 'machine'
		ORDER BY c.name, c.id`, organizationID, ids)
	if err != nil {
		return nil, err
	}
	var refs []launchCredentialRef
	err = rs.DB.SelectContext(r.Context(), &refs, rs.DB.Rebind(query), args...)
	return refs, err
}

func (rs launchInputResolver) resolve(r *http.Request, template models.JobTemplate, input launchPromptInput) (effectiveLaunchInputs, error) {
	if input.InventoryID != nil && !template.AskInventoryOnLaunch {
		return effectiveLaunchInputs{}, fmt.Errorf("inventory cannot be changed at launch")
	}
	if input.CredentialID != nil && !template.AskCredentialOnLaunch {
		return effectiveLaunchInputs{}, fmt.Errorf("credential cannot be changed at launch")
	}
	if input.Limit != nil && !template.AskLimitOnLaunch {
		return effectiveLaunchInputs{}, fmt.Errorf("limit cannot be changed at launch")
	}
	if !template.SurveyEnabled && !template.AskVariablesOnLaunch && len(input.ExtraVars) > 0 {
		return effectiveLaunchInputs{}, fmt.Errorf("variables cannot be changed at launch")
	}

	inventoryID := template.InventoryID
	if input.InventoryID != nil {
		inventoryID = input.InventoryID
	}
	credentialID := template.CredentialID
	credentialIsOverride := input.CredentialID != nil
	if credentialIsOverride {
		credentialID = input.CredentialID
	}

	inventory, err := rs.resolveLaunchInventory(r, template.OrganizationID, inventoryID)
	if err != nil {
		return effectiveLaunchInputs{}, err
	}
	credential, err := rs.resolveLaunchCredential(r, template.OrganizationID, credentialID, credentialIsOverride)
	if err != nil {
		return effectiveLaunchInputs{}, err
	}

	effectiveVars := rawObject(template.ExtraVars)
	if template.SurveyEnabled {
		answers, err := applySurvey(template.SurveySpec, input.ExtraVars)
		if err != nil {
			return effectiveLaunchInputs{}, err
		}
		mergeObject(effectiveVars, answers)
	} else if template.AskVariablesOnLaunch {
		mergeObject(effectiveVars, input.ExtraVars)
	}

	effectiveLimit := template.JobLimit
	if input.Limit != nil {
		effectiveLimit = strings.TrimSpace(*input.Limit)
	}
	if len(effectiveLimit) > 512 || strings.ContainsAny(effectiveLimit, "\x00\r\n") {
		return effectiveLaunchInputs{}, fmt.Errorf("limit is invalid")
	}

	return effectiveLaunchInputs{
		Inventory:  inventory,
		Credential: credential,
		ExtraVars:  effectiveVars,
		Limit:      effectiveLimit,
	}, nil
}

func (rs launchInputResolver) preview(r *http.Request, template models.JobTemplate, input launchPromptInput) (launchPreview, error) {
	effective, err := rs.resolve(r, template, input)
	if err != nil {
		return launchPreview{}, err
	}
	preview := launchPreview{
		Inventory:               effective.Inventory,
		Credential:              effective.Credential,
		ExtraVars:               effective.ExtraVars,
		Limit:                   effective.Limit,
		InventoryHostSample:     []string{},
		LimitAppliedAtExecution: effective.Limit != "",
	}
	preview.Template.ID = template.ID
	preview.Template.UnifiedJobTemplateID = *template.UnifiedJobTemplateID
	preview.Template.Name = template.Name
	if effective.Inventory != nil {
		if err := rs.DB.GetContext(r.Context(), &preview.InventoryHostCount,
			`SELECT count(*) FROM hosts WHERE inventory_id=$1 AND enabled=true`, effective.Inventory.ID); err != nil {
			return launchPreview{}, err
		}
		if err := rs.DB.SelectContext(r.Context(), &preview.InventoryHostSample,
			`SELECT name FROM hosts WHERE inventory_id=$1 AND enabled=true ORDER BY name LIMIT $2`,
			effective.Inventory.ID, launchPreviewHostSampleLimit); err != nil {
			return launchPreview{}, err
		}
	}
	return preview, nil
}

func (rs launchInputResolver) resolveLaunchInventory(r *http.Request, organizationID int64, id *int64) (*launchInventoryRef, error) {
	if id == nil {
		return nil, nil
	}
	allowed, err := rs.canAuthorize(r, rbac.Inventory, *id, actUse)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errLaunchResourceUnavailable
	}
	var ref launchInventoryRef
	if err := rs.DB.GetContext(r.Context(), &ref,
		`SELECT id, name, kind FROM inventories WHERE id=$1 AND organization_id=$2`,
		*id, organizationID); errors.Is(err, sql.ErrNoRows) {
		return nil, errLaunchResourceUnavailable
	} else if err != nil {
		return nil, err
	}
	return &ref, nil
}

func (rs launchInputResolver) resolveLaunchCredential(r *http.Request, organizationID int64, id *int64, requireMachine bool) (*launchCredentialRef, error) {
	if id == nil {
		return nil, nil
	}
	allowed, err := rs.canAuthorize(r, rbac.Credential, *id, actUse)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errLaunchResourceUnavailable
	}
	query := `
		SELECT c.id, c.name, c.credential_type_id, ct.name AS credential_type
		FROM credentials c JOIN credential_types ct ON ct.id=c.credential_type_id
		WHERE c.id=$1 AND c.organization_id=$2`
	if requireMachine {
		query += ` AND lower(ct.name)='machine'`
	}
	var ref launchCredentialRef
	if err := rs.DB.GetContext(r.Context(), &ref, query, *id, organizationID); errors.Is(err, sql.ErrNoRows) {
		return nil, errLaunchResourceUnavailable
	} else if err != nil {
		return nil, err
	}
	return &ref, nil
}

func rawObject(raw json.RawMessage) map[string]interface{} {
	out := map[string]interface{}{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func mergeObject(dst, src map[string]interface{}) {
	for key, value := range src {
		dst[key] = value
	}
}
