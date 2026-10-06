-- name: UpsertDoctorSchedule :one
INSERT INTO doctor_schedules (doctor_id, day_of_week, start_time, end_time, slot_duration, timezone, is_active, updated_at, tenant_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, now(), @tenant_id)
ON CONFLICT (doctor_id, day_of_week) 
DO UPDATE SET
    start_time = EXCLUDED.start_time,
    end_time = EXCLUDED.end_time,
    slot_duration = EXCLUDED.slot_duration,
    timezone = EXCLUDED.timezone,
    is_active = EXCLUDED.is_active,
    updated_at = now()
RETURNING id, doctor_id, day_of_week, start_time, end_time, slot_duration, timezone, is_active, created_at, updated_at;

-- name: GetDoctorSchedules :many
SELECT id, doctor_id, day_of_week, start_time, end_time, slot_duration, timezone, is_active, created_at, updated_at
FROM doctor_schedules
WHERE doctor_id = $1 AND tenant_id = @tenant_id
ORDER BY day_of_week ASC;

-- name: GetDoctorScheduleByDay :one
SELECT id, doctor_id, day_of_week, start_time, end_time, slot_duration, timezone, is_active, created_at, updated_at
FROM doctor_schedules
WHERE doctor_id = $1 AND day_of_week = $2 AND tenant_id = @tenant_id;

-- name: DeleteDoctorScheduleByDay :exec
DELETE FROM doctor_schedules
WHERE doctor_id = $1 AND day_of_week = $2 AND tenant_id = @tenant_id;
