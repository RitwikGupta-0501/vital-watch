-- name: CreateAdminUser :one
INSERT INTO users (email, hashed_password, role)
VALUES ($1, $2, 'admin')
RETURNING id;

-- name: CreateAdminProfile :exec
INSERT INTO admin_profiles (user_id, first_name, last_name, department)
VALUES ($1, $2, $3, $4);

-- name: GetAdminByEmail :one
SELECT u.id, u.email, a.first_name, a.last_name, a.department, u.hashed_password, u.role, u.created_at
FROM users u
JOIN admin_profiles a ON u.id = a.user_id
WHERE u.email = $1 AND u.role = 'admin' AND u.is_active = true;

-- name: GetAdminByID :one
SELECT u.id, u.email, a.first_name, a.last_name, a.department, u.role, u.created_at
FROM users u
JOIN admin_profiles a ON u.id = a.user_id
WHERE u.id = $1 AND u.role = 'admin' AND u.is_active = true;

-- name: GetAllUsers :many
SELECT u.id, u.email, u.role, u.is_active, u.created_at
FROM users u
ORDER BY u.created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateUserActiveStatus :execrows
UPDATE users
SET is_active = $2, updated_at = now()
WHERE id = $1;
