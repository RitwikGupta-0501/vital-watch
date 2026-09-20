-- name: CreatePrescription :one
INSERT INTO prescriptions (patient_id, doctor_id, source, status, file_name, notes)
VALUES ($1, $2, 'uploaded', $3, $4, $5)
RETURNING id;

-- name: CreateDigitalPrescription :one
INSERT INTO prescriptions (patient_id, doctor_id, source, status, notes)
VALUES ($1, $2, 'digital', 'approved', $3)
RETURNING id;

-- name: InsertPrescriptionItem :one
INSERT INTO prescription_items (prescription_id, medication_name, dosage, frequency, duration, timing, instructions)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, prescription_id, medication_name, dosage, frequency, duration, timing, instructions, created_at;

-- name: GetPrescriptionItemsByPrescriptionID :many
SELECT id, prescription_id, medication_name, dosage, frequency, duration, timing, instructions, created_at
FROM prescription_items
WHERE prescription_id = $1
ORDER BY created_at ASC;

-- name: GetPrescriptionItemsByPrescriptionIDs :many
SELECT id, prescription_id, medication_name, dosage, frequency, duration, timing, instructions, created_at
FROM prescription_items
WHERE prescription_id = ANY(@prescription_ids::uuid[])
ORDER BY created_at ASC;

-- name: DeletePrescriptionItems :exec
DELETE FROM prescription_items
WHERE prescription_id = $1;

-- name: UpdatePrescriptionOCRStatus :execrows
UPDATE prescriptions
SET status = $2,
    notes = CASE
        WHEN $3 IS NOT NULL AND notes IS NOT NULL AND notes <> '' THEN notes || E'\n' || $3
        WHEN $3 IS NOT NULL THEN $3
        ELSE notes
    END,
    ocr_provider = $4,
    updated_at = now()
WHERE id = $1
  AND status = 'pending_ocr';

-- name: VerifyPrescription :execrows
UPDATE prescriptions p
SET status = $2,
    notes = CASE
        WHEN $3 IS NOT NULL THEN $3
        WHEN $2 = 'approved' THEN TRIM(regexp_replace(COALESCE(notes, ''), E'\s*\[(OCR|Storage)[^\]]+\]', '', 'g'))
        ELSE notes
    END,
    updated_at = now()
WHERE p.id = $1
  AND p.status = 'needs_review'
  AND (
    p.doctor_id = $4
    OR EXISTS (
      SELECT 1 FROM appointments a
      WHERE a.patient_id = p.patient_id AND a.doctor_id = $4
    )
  );

-- name: GetPrescriptionsPendingReview :many
SELECT
    p.id, p.patient_id, p.doctor_id, p.source, p.status, p.file_name, p.notes, p.ocr_provider, p.created_at, p.updated_at,
    d.first_name AS doctor_first_name, d.last_name AS doctor_last_name,
    pat.first_name AS patient_first_name, pat.last_name AS patient_last_name
FROM prescriptions p
JOIN doctor_profiles d ON p.doctor_id = d.user_id
JOIN patient_profiles pat ON p.patient_id = pat.user_id
WHERE p.status = 'needs_review'
  AND (
    p.doctor_id = $1
    OR EXISTS (
      SELECT 1 FROM appointments a
      WHERE a.patient_id = p.patient_id AND a.doctor_id = $1
    )
  )
ORDER BY p.created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetPrescriptionsByPatientID :many
SELECT p.id, p.patient_id, p.doctor_id, p.source, p.status, p.file_name, p.notes, p.ocr_provider, p.created_at, p.updated_at, d.first_name, d.last_name
FROM prescriptions p
JOIN doctor_profiles d ON p.doctor_id = d.user_id
WHERE p.patient_id = $1
  AND p.status = 'approved'
ORDER BY p.created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetPrescriptionByFilename :one
SELECT id, status FROM prescriptions WHERE patient_id = $1 AND file_name = $2 AND status = 'approved';

-- name: GetPrescriptionsForPatient :many
SELECT
    p.id, p.patient_id, p.doctor_id, p.source, p.status, p.file_name, p.notes, p.ocr_provider, p.created_at, p.updated_at,
    d.first_name, d.last_name
FROM prescriptions p
JOIN doctor_profiles d ON p.doctor_id = d.user_id
WHERE p.patient_id = $1
  AND (sqlc.narg('status')::varchar IS NULL OR sqlc.narg('status') = '' OR p.status = sqlc.narg('status'))
  AND (
    p.doctor_id = $2
    OR EXISTS (
      SELECT 1 FROM appointments a WHERE a.patient_id = $1 AND a.doctor_id = $2
    )
  )
ORDER BY p.created_at DESC
LIMIT $3 OFFSET $4;

-- name: GetPrescriptionByFilenameForDoctor :one
SELECT p.id, p.status
FROM prescriptions p
LEFT JOIN appointments a ON p.patient_id = a.patient_id AND a.doctor_id = $2
WHERE p.file_name = $1 AND (p.doctor_id = $2 OR a.doctor_id = $2)
LIMIT 1;

-- name: GetPrescriptionByID :one
SELECT
    p.id, p.patient_id, p.doctor_id, p.source, p.status, p.file_name, p.notes, p.ocr_provider, p.created_at, p.updated_at,
    d.first_name AS doctor_first_name, d.last_name AS doctor_last_name,
    pat.first_name AS patient_first_name, pat.last_name AS patient_last_name
FROM prescriptions p
JOIN doctor_profiles d ON p.doctor_id = d.user_id
JOIN patient_profiles pat ON p.patient_id = pat.user_id
WHERE p.id = $1;
