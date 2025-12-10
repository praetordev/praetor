package models

import (
	"time"
)

type InstanceGroup struct {
	ID         int64     `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	ModifiedAt time.Time `json:"modified_at" db:"modified_at"`
}

type Instance struct {
	ID         int64     `json:"id" db:"id"`
	Hostname   string    `json:"hostname" db:"hostname"`
	Version    *string   `json:"version,omitempty" db:"version"`
	Capacity   int       `json:"capacity" db:"capacity"`
	Enabled    bool      `json:"enabled" db:"enabled"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	ModifiedAt time.Time `json:"modified_at" db:"modified_at"`
}
