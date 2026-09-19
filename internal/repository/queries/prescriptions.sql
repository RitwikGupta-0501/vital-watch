-- name: CreatePrescription :one
INSERT INTO prescriptions (patient_id, doctor_id, medication, notes, file_name)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: GetPrescriptionsByPatientID :many
SELECT p.id, p.patient_id, p.doctor_id, p.medication, p.notes, p.file_name, p.created_at, d.first_name, d.last_name
FROM prescriptions p
JOIN doctor_profiles d ON p.doctor_id = d.user_id
WHERE p.patient_id = $1
ORDER BY p.created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetPrescriptionByFilename :one
SELECT id FROM prescriptions WHERE patient_id = $1 AND file_name = $2;

-- name: GetPrescriptionsForPatient :many
SELECT 
    p.id, p.patient_id, p.doctor_id, p.medication, p.notes, p.file_name, p.created_at, 
    d.first_name, d.last_name
FROM prescriptions p
JOIN doctor_profiles d ON p.doctor_id = d.user_id
WHERE p.patient_id = $1 AND EXISTS (
    SELECT 1 FROM appointments a WHERE a.patient_id = $1 AND a.doctor_id = $2
)
ORDER BY p.created_at DESC
LIMIT $3 OFFSET $4;

-- name: GetPrescriptionByFilenameForDoctor :one
SELECT p.id 
FROM prescriptions p
LEFT JOIN appointments a ON p.patient_id = a.patient_id AND a.doctor_id = $2
WHERE p.file_name = $1 AND (p.doctor_id = $2 OR a.doctor_id = $2)
LIMIT 1;
