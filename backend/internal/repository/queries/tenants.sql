-- name: CreateTenant :one
INSERT INTO tenants (name, domain, status)
VALUES ($1, $2, 'active')
RETURNING id, name, domain, status, created_at, updated_at;

-- name: CreateTenantInvite :one
INSERT INTO tenant_invites (tenant_id, invite_code, role, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, tenant_id, invite_code, role, expires_at, created_at;

-- name: GetTenantInviteByCode :one
SELECT id, tenant_id, invite_code, role, expires_at, created_at
FROM tenant_invites
WHERE invite_code = $1;

-- name: DeleteTenantInvite :exec
DELETE FROM tenant_invites
WHERE invite_code = $1;
