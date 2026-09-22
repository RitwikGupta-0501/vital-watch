-- name: CreatePatientVital :one
INSERT INTO patient_vitals (
    patient_id, recorded_by, recorded_at, systolic_bp, diastolic_bp, heart_rate, 
    blood_glucose, oxygen_saturation, temperature, weight_kg, notes
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, patient_id, recorded_by, recorded_at, systolic_bp, diastolic_bp, heart_rate, 
    blood_glucose, oxygen_saturation, temperature, weight_kg, notes, created_at;

-- name: GetPatientVitals :many
SELECT 
    v.id, v.patient_id, v.recorded_by, v.recorded_at, v.systolic_bp, v.diastolic_bp, v.heart_rate, 
    v.blood_glucose, v.oxygen_saturation, v.temperature, v.weight_kg, v.notes, v.created_at,
    u.role AS recorder_role,
    COALESCE(dp.first_name, pp.first_name, '') AS recorder_first_name,
    COALESCE(dp.last_name, pp.last_name, '') AS recorder_last_name
FROM patient_vitals v
JOIN users u ON v.recorded_by = u.id
LEFT JOIN doctor_profiles dp ON u.id = dp.user_id
LEFT JOIN patient_profiles pp ON u.id = pp.user_id
WHERE v.patient_id = $1
  AND (sqlc.narg('start_date')::timestamptz IS NULL OR v.recorded_at >= sqlc.narg('start_date'))
  AND (sqlc.narg('end_date')::timestamptz IS NULL OR v.recorded_at <= sqlc.narg('end_date'))
ORDER BY v.recorded_at DESC
LIMIT $2 OFFSET $3;

-- name: GetLatestPatientVital :one
SELECT 
    v.id, v.patient_id, v.recorded_by, v.recorded_at, v.systolic_bp, v.diastolic_bp, v.heart_rate, 
    v.blood_glucose, v.oxygen_saturation, v.temperature, v.weight_kg, v.notes, v.created_at,
    u.role AS recorder_role,
    COALESCE(dp.first_name, pp.first_name, '') AS recorder_first_name,
    COALESCE(dp.last_name, pp.last_name, '') AS recorder_last_name
FROM patient_vitals v
JOIN users u ON v.recorded_by = u.id
LEFT JOIN doctor_profiles dp ON u.id = dp.user_id
LEFT JOIN patient_profiles pp ON u.id = pp.user_id
WHERE v.patient_id = $1
ORDER BY v.recorded_at DESC
LIMIT 1;
