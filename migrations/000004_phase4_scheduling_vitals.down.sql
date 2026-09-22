DROP TABLE IF EXISTS patient_medication_logs;
DROP TABLE IF EXISTS patient_vitals;
ALTER TABLE prescriptions DROP COLUMN IF EXISTS expires_at;
ALTER TABLE appointments DROP COLUMN IF EXISTS meeting_id;
ALTER TABLE appointments DROP COLUMN IF EXISTS meeting_link;
DROP TABLE IF EXISTS doctor_schedules;
