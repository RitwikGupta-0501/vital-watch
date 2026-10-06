-- name: CreateAdminUser :one
INSERT INTO users (email, hashed_password, role, tenant_id)
VALUES ($1, $2, @role, @tenant_id)
RETURNING id;

-- name: CreateAdminProfile :exec
INSERT INTO admin_profiles (user_id, first_name, last_name, department, tenant_id)
VALUES ($1, $2, $3, $4, @tenant_id);

-- name: GetAdminByEmail :one
SELECT u.id, u.email, a.first_name, a.last_name, a.department, u.hashed_password, u.role, u.tenant_id, u.created_at
FROM users u
JOIN admin_profiles a ON u.id = a.user_id
WHERE u.email = $1 AND u.role IN ('tenant_admin', 'platform_admin') AND u.is_active = true AND u.tenant_id = @tenant_id;

-- name: GetAdminByID :one
SELECT u.id, u.email, a.first_name, a.last_name, a.department, u.role, u.tenant_id, u.created_at
FROM users u
JOIN admin_profiles a ON u.id = a.user_id
WHERE u.id = $1 AND u.role IN ('tenant_admin', 'platform_admin') AND u.is_active = true AND u.tenant_id = @tenant_id;

-- name: GetAllUsers :many
SELECT u.id, u.email, u.role, u.is_active, u.created_at
FROM users u
WHERE u.tenant_id = @tenant_id
ORDER BY u.created_at DESC, u.id DESC
LIMIT $1 OFFSET $2;

-- name: UpdateUserActiveStatus :execrows
UPDATE users
SET is_active = $2, updated_at = now()
WHERE id = $1 AND tenant_id = @tenant_id;

-- name: GetUserByIDGlobal :one
SELECT u.id, u.email, u.role, u.tenant_id, u.is_active
FROM users u
WHERE u.id = $1;

