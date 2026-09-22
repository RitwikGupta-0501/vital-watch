-- name: CreatePatientUser :one
INSERT INTO users (email, hashed_password, role)
VALUES ($1, $2, 'patient')
RETURNING id;

-- name: CreatePatientProfile :exec
INSERT INTO patient_profiles (user_id, first_name, last_name)
VALUES ($1, $2, $3);

-- name: GetPatientByEmail :one
SELECT u.id, u.email, p.first_name, p.last_name, u.hashed_password, u.created_at
FROM users u
JOIN patient_profiles p ON u.id = p.user_id
WHERE u.email = $1 AND u.role = 'patient' AND u.is_active = true;

-- name: GetPatientByID :one
SELECT u.id, u.email, p.first_name, p.last_name, u.created_at
FROM users u
JOIN patient_profiles p ON u.id = p.user_id
WHERE u.id = $1 AND u.role = 'patient' AND u.is_active = true;

-- name: GetPatientsByDoctorID :many
SELECT DISTINCT u.id, u.email, p.first_name, p.last_name, u.created_at
FROM users u
JOIN patient_profiles p ON u.id = p.user_id
JOIN appointments a ON u.id = a.patient_id
WHERE a.doctor_id = $1
ORDER BY u.created_at DESC
LIMIT $2 OFFSET $3;

-- name: HasDoctorPatientRelationship :one
SELECT (
    EXISTS (SELECT 1 FROM appointments a WHERE a.doctor_id = $1 AND a.patient_id = $2 AND a.status != 'cancelled')
    OR
    EXISTS (SELECT 1 FROM prescriptions p WHERE p.doctor_id = $1 AND p.patient_id = $2)
) AS has_relationship;
