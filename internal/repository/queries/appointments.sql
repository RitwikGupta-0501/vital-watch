-- name: CreateAppointment :one
INSERT INTO appointments (id, patient_id, doctor_id, start_time, end_time, appointment_type, meeting_link, meeting_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: GetAppointmentsByDoctorID :many
SELECT 
    a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, 
    a.meeting_link, a.meeting_id, a.created_at,
    p.first_name, p.last_name
FROM appointments a
JOIN patient_profiles p ON a.patient_id = p.user_id
WHERE a.doctor_id = $1
ORDER BY a.start_time DESC
LIMIT $2 OFFSET $3;

-- name: GetAppointmentsByPatientID :many
SELECT 
    a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, 
    a.meeting_link, a.meeting_id, a.created_at,
    d.first_name, d.last_name, d.specialty
FROM appointments a
JOIN doctor_profiles d ON a.doctor_id = d.user_id
WHERE a.patient_id = $1
ORDER BY a.start_time DESC
LIMIT $2 OFFSET $3;

-- name: GetAppointmentsForPatient :many
SELECT 
    a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, 
    a.meeting_link, a.meeting_id, a.created_at,
    d.first_name, d.last_name, d.specialty
FROM appointments a
JOIN doctor_profiles d ON a.doctor_id = d.user_id
WHERE a.patient_id = $1 AND a.doctor_id = $2
ORDER BY a.start_time DESC
LIMIT $3 OFFSET $4;

-- name: GetAppointmentByID :one
SELECT 
    a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, 
    a.meeting_link, a.meeting_id, a.created_at,
    d.first_name AS doctor_first_name, d.last_name AS doctor_last_name, d.specialty AS doctor_specialty,
    p.first_name AS patient_first_name, p.last_name AS patient_last_name
FROM appointments a
JOIN doctor_profiles d ON a.doctor_id = d.user_id
JOIN patient_profiles p ON a.patient_id = p.user_id
WHERE a.id = $1;

-- name: GetDoctorAppointmentsInRange :many
SELECT a.id, a.doctor_id, a.start_time, a.end_time, a.status
FROM appointments a
WHERE a.doctor_id = $1 
  AND a.status != 'cancelled'
  AND a.start_time < sqlc.arg('range_end')::timestamptz 
  AND a.end_time > sqlc.arg('range_start')::timestamptz
ORDER BY a.start_time ASC;

-- name: UpdateAppointmentAsCompletedForDoctor :execrows
UPDATE appointments 
SET status = 'completed' 
WHERE id = $1 AND doctor_id = $2;

-- name: UpdateAppointmentMeetingRoom :exec
UPDATE appointments 
SET meeting_link = $1, meeting_id = $2
WHERE id = $3 AND (meeting_link IS NULL OR meeting_link = '');
