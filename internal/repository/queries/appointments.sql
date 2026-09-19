-- name: CreateAppointment :one
INSERT INTO appointments (patient_id, doctor_id, start_time, end_time, appointment_type)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: GetAppointmentsByDoctorID :many
SELECT 
    a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, a.created_at,
    p.first_name, p.last_name
FROM appointments a
JOIN patient_profiles p ON a.patient_id = p.user_id
WHERE a.doctor_id = $1
ORDER BY a.start_time DESC
LIMIT $2 OFFSET $3;

-- name: GetAppointmentsByPatientID :many
SELECT 
    a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, a.created_at,
    d.first_name, d.last_name, d.specialty
FROM appointments a
JOIN doctor_profiles d ON a.doctor_id = d.user_id
WHERE a.patient_id = $1
ORDER BY a.start_time DESC
LIMIT $2 OFFSET $3;

-- name: GetAppointmentsForPatient :many
SELECT 
    a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, a.created_at,
    d.first_name, d.last_name, d.specialty
FROM appointments a
JOIN doctor_profiles d ON a.doctor_id = d.user_id
WHERE a.patient_id = $1 AND a.doctor_id = $2
ORDER BY a.start_time DESC
LIMIT $3 OFFSET $4;

-- name: UpdateAppointmentAsCompletedForDoctor :execrows
UPDATE appointments 
SET status = 'completed' 
WHERE id = $1 AND doctor_id = $2;
