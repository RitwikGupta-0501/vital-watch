package models

import "github.com/google/uuid"

var (
	// SystemDefaultTenantID is the default tenant ID used for single-tenant deployments
	// or system-level configuration before full multi-tenancy is introduced.
	SystemDefaultTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000000")
)
