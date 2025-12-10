package models

import (
	"encoding/json"
	"time"
)

type Project struct {
	ID             int64     `json:"id" db:"id"`
	OrganizationID int64     `json:"organization_id" db:"organization_id"`
	Name           string    `json:"name" db:"name"`
	Description    *string   `json:"description,omitempty" db:"description"`
	SCMType        string    `json:"scm_type" db:"scm_type"`
	SCMURL         string    `json:"scm_url" db:"scm_url"`
	SCMBranch      *string   `json:"scm_branch,omitempty" db:"scm_branch"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	ModifiedAt     time.Time `json:"modified_at" db:"modified_at"`
}

type Inventory struct {
	ID             int64     `json:"id" db:"id"`
	OrganizationID int64     `json:"organization_id" db:"organization_id"`
	Name           string    `json:"name" db:"name"`
	Description    *string   `json:"description,omitempty" db:"description"`
	Kind           string    `json:"kind" db:"kind"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	ModifiedAt     time.Time `json:"modified_at" db:"modified_at"`
}

type Host struct {
	ID          int64           `json:"id" db:"id"`
	InventoryID int64           `json:"inventory_id" db:"inventory_id"`
	Name        string          `json:"name" db:"name"`
	Description *string         `json:"description,omitempty" db:"description"`
	Variables   json.RawMessage `json:"variables,omitempty" db:"variables"`
	Enabled     bool            `json:"enabled" db:"enabled"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	ModifiedAt  time.Time       `json:"modified_at" db:"modified_at"`
}

type Group struct {
	ID          int64           `json:"id" db:"id"`
	InventoryID int64           `json:"inventory_id" db:"inventory_id"`
	Name        string          `json:"name" db:"name"`
	Description *string         `json:"description,omitempty" db:"description"`
	Variables   json.RawMessage `json:"variables,omitempty" db:"variables"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	ModifiedAt  time.Time       `json:"modified_at" db:"modified_at"`
}

type CredentialType struct {
	ID          int64           `json:"id" db:"id"`
	Name        string          `json:"name" db:"name"`
	Description *string         `json:"description,omitempty" db:"description"`
	Inputs      json.RawMessage `json:"inputs" db:"inputs"`
	Injectors   json.RawMessage `json:"injectors" db:"injectors"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	ModifiedAt  time.Time       `json:"modified_at" db:"modified_at"`
}

type Credential struct {
	ID               int64           `json:"id" db:"id"`
	OrganizationID   int64           `json:"organization_id" db:"organization_id"`
	CredentialTypeID int64           `json:"credential_type_id" db:"credential_type_id"`
	Name             string          `json:"name" db:"name"`
	Description      *string         `json:"description,omitempty" db:"description"`
	Inputs           json.RawMessage `json:"inputs" db:"inputs"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	ModifiedAt       time.Time       `json:"modified_at" db:"modified_at"`
}

type ExecutionEnvironment struct {
	ID             int64     `json:"id" db:"id"`
	OrganizationID *int64    `json:"organization_id,omitempty" db:"organization_id"`
	Name           string    `json:"name" db:"name"`
	Image          string    `json:"image" db:"image"`
	Description    *string   `json:"description,omitempty" db:"description"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	ModifiedAt     time.Time `json:"modified_at" db:"modified_at"`
}

type JobTemplate struct {
	ID                     int64           `json:"id" db:"id"`
	OrganizationID         int64           `json:"organization_id" db:"organization_id"`
	Name                   string          `json:"name" db:"name"`
	Description            *string         `json:"description,omitempty" db:"description"`
	InventoryID            *int64          `json:"inventory_id,omitempty" db:"inventory_id"`
	ProjectID              *int64          `json:"project_id,omitempty" db:"project_id"`
	Playbook               string          `json:"playbook" db:"playbook"`
	PlaybookContent        *string         `json:"playbook_content,omitempty" db:"playbook_content"`
	ExecutionEnvironmentID *int64          `json:"execution_environment_id,omitempty" db:"execution_environment_id"`
	Forks                  int             `json:"forks" db:"forks"`
	JobType                string          `json:"job_type" db:"job_type"`
	Verbosity              int             `json:"verbosity" db:"verbosity"`
	ExtraVars              json.RawMessage `json:"extra_vars,omitempty" db:"extra_vars"`
	CreatedAt              time.Time       `json:"created_at" db:"created_at"`
	ModifiedAt             time.Time       `json:"modified_at" db:"modified_at"`
}
