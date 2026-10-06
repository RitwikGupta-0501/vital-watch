-- name: CreatePatientUser :one
INSERT INTO users (email, hashed_password, role, tenant_id)
VALUES ($1, $2, 'patient', @tenant_id)
RETURNING id;

-- name: CreatePatientProfile :exec
INSERT INTO patient_profiles (user_id, first_name, last_name, tenant_id)
VALUES ($1, $2, $3, @tenant_id);

-- name: GetPatientByEmail :one
SELECT u.id, u.email, p.first_name, p.last_name, u.hashed_password, u.created_at
FROM users u
JOIN patient_profiles p ON u.id = p.user_id
WHERE u.email = $1 AND u.role = 'patient' AND u.is_active = true AND u.tenant_id = @tenant_id;

-- name: GetPatientByID :one
SELECT u.id, u.email, p.first_name, p.last_name, u.created_at
FROM users u
JOIN patient_profiles p ON u.id = p.user_id
WHERE u.id = $1 AND u.role = 'patient' AND u.is_active = true AND u.tenant_id = @tenant_id;

-- name: GetPatientsByDoctorID :many
-- Uses an EXISTS semi-join instead of SELECT DISTINCT to avoid a full
-- hash-aggregate/deduplication pass over all appointment rows.
-- The EXISTS subquery short-circuits at the first matching appointment
-- per patient, allowing PostgreSQL to use an index scan with pagination.
SELECT u.id, u.email, p.first_name, p.last_name, u.created_at
FROM users u
JOIN patient_profiles p ON u.id = p.user_id
WHERE u.role = 'patient'
  AND u.is_active = true
  AND u.tenant_id = @tenant_id
  AND EXISTS (
      SELECT 1 FROM appointments a
      WHERE a.patient_id = u.id AND a.doctor_id = $1
  )
ORDER BY u.created_at DESC, u.id DESC
LIMIT $2 OFFSET $3;

-- name: HasDoctorPatientRelationship :one
SELECT (
    EXISTS (SELECT 1 FROM appointments a WHERE a.doctor_id = $1 AND a.patient_id = $2 AND a.status != 'cancelled' AND a.tenant_id = @tenant_id)
    OR
    EXISTS (SELECT 1 FROM prescriptions p WHERE p.doctor_id = $1 AND p.patient_id = $2 AND p.tenant_id = @tenant_id)
) AS has_relationship;
