-- name: CreateDoctorUser :one
INSERT INTO users (email, hashed_password, role)
VALUES ($1, $2, 'doctor')
RETURNING id;

-- name: CreateDoctorProfile :exec
INSERT INTO doctor_profiles (user_id, first_name, last_name, specialty, experience_years)
VALUES ($1, $2, $3, $4, $5);

-- name: GetDoctorByEmail :one
SELECT u.id, u.email, d.first_name, d.last_name, d.specialty, d.experience_years, d.available, u.hashed_password, u.created_at
FROM users u
JOIN doctor_profiles d ON u.id = d.user_id
WHERE u.email = $1 AND u.role = 'doctor' AND u.is_active = true;

-- name: GetDoctorByID :one
SELECT u.id, u.email, d.first_name, d.last_name, d.specialty, d.experience_years, d.available, u.created_at
FROM users u
JOIN doctor_profiles d ON u.id = d.user_id
WHERE u.id = $1 AND u.role = 'doctor' AND u.is_active = true;

-- name: GetDoctors :many
SELECT u.id, u.email, d.first_name, d.last_name, d.specialty, d.experience_years, d.available, u.created_at
FROM users u
JOIN doctor_profiles d ON u.id = d.user_id
WHERE u.role = 'doctor' AND u.is_active = true
ORDER BY d.first_name ASC
LIMIT $1 OFFSET $2;
