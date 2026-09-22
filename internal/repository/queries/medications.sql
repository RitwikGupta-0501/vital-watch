-- name: UpsertMedicationLog :one
INSERT INTO patient_medication_logs (
    patient_id, prescription_item_id, scheduled_date, time_of_day, dose_number, meal_timing, status, taken_at, notes
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (patient_id, prescription_item_id, scheduled_date, time_of_day, dose_number)
DO UPDATE SET
    status = EXCLUDED.status,
    taken_at = EXCLUDED.taken_at,
    notes = EXCLUDED.notes,
    meal_timing = COALESCE(EXCLUDED.meal_timing, patient_medication_logs.meal_timing)
RETURNING id, patient_id, prescription_item_id, scheduled_date, time_of_day, dose_number, meal_timing, status, taken_at, notes, created_at;

-- name: GetMedicationLogsByDate :many
SELECT 
    l.id, l.patient_id, l.prescription_item_id, l.scheduled_date, l.time_of_day, l.dose_number, l.meal_timing, l.status, l.taken_at, l.notes, l.created_at,
    pi.medication_name, pi.dosage, pi.timing, pi.instructions
FROM patient_medication_logs l
JOIN prescription_items pi ON l.prescription_item_id = pi.id
WHERE l.patient_id = $1 AND l.scheduled_date = $2
ORDER BY l.created_at ASC;

-- name: GetActivePrescriptionItemsForPatient :many
SELECT 
    pi.id, pi.prescription_id, pi.medication_name, pi.dosage, pi.frequency, pi.duration, pi.timing, pi.instructions, pi.created_at
FROM prescription_items pi
JOIN prescriptions p ON pi.prescription_id = p.id
WHERE p.patient_id = $1 
  AND p.status = 'approved'
  AND p.created_at < (date_trunc('day', sqlc.arg('target_date')::timestamptz) + interval '1 day')
  AND (p.expires_at IS NULL OR p.expires_at >= date_trunc('day', sqlc.arg('target_date')::timestamptz))
ORDER BY pi.created_at DESC;

-- name: VerifyPrescriptionItemOwnership :one
SELECT COUNT(*) > 0 AS is_owner
FROM prescription_items pi
JOIN prescriptions p ON pi.prescription_id = p.id
WHERE pi.id = $1 AND p.patient_id = $2 AND p.status = 'approved';
