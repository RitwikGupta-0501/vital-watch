-- Migration: 000008_harden_schema_integrity.up.sql
-- Goal: Harden schema invariants, prevent clinical record cascade deletions (HIPAA compliance),
--       add performance indexes, and drop redundant duplicate indexes.

-- 1. Prevent Cascade Deletion on Clinical Records (HIPAA § 164.312 & Medical Retention Laws)
ALTER TABLE patient_vitals 
    DROP CONSTRAINT IF EXISTS patient_vitals_recorded_by_fkey,
    ADD CONSTRAINT patient_vitals_recorded_by_fkey FOREIGN KEY (recorded_by) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE patient_vitals 
    DROP CONSTRAINT IF EXISTS patient_vitals_patient_id_fkey,
    ADD CONSTRAINT patient_vitals_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES patient_profiles(user_id) ON DELETE RESTRICT;

ALTER TABLE appointments 
    DROP CONSTRAINT IF EXISTS appointments_doctor_id_fkey,
    DROP CONSTRAINT IF EXISTS appointments_patient_id_fkey,
    ADD CONSTRAINT appointments_doctor_id_fkey FOREIGN KEY (doctor_id) REFERENCES users(id) ON DELETE RESTRICT,
    ADD CONSTRAINT appointments_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE prescriptions 
    DROP CONSTRAINT IF EXISTS prescriptions_doctor_id_fkey,
    DROP CONSTRAINT IF EXISTS prescriptions_patient_id_fkey,
    ADD CONSTRAINT prescriptions_doctor_id_fkey FOREIGN KEY (doctor_id) REFERENCES doctor_profiles(user_id) ON DELETE RESTRICT,
    ADD CONSTRAINT prescriptions_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES patient_profiles(user_id) ON DELETE RESTRICT;

-- 2. Enforce NOT NULL Invariants, Defaults, and Timestamps
UPDATE appointments SET status = 'upcoming' WHERE status IS NULL;
ALTER TABLE appointments ALTER COLUMN status SET NOT NULL;
ALTER TABLE appointments ALTER COLUMN status SET DEFAULT 'upcoming';

UPDATE appointments SET appointment_type = 'in_person' WHERE appointment_type IS NULL;
ALTER TABLE appointments ALTER COLUMN appointment_type SET NOT NULL;
ALTER TABLE appointments ALTER COLUMN appointment_type SET DEFAULT 'in_person';

ALTER TABLE appointments ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE users SET is_active = TRUE WHERE is_active IS NULL;
ALTER TABLE users ALTER COLUMN is_active SET NOT NULL;
ALTER TABLE users ALTER COLUMN is_active SET DEFAULT TRUE;

ALTER TABLE doctor_profiles ADD CONSTRAINT chk_doctor_experience_positive CHECK (experience_years >= 0);

-- 3. Drop Redundant Duplicate Indexes
DROP INDEX IF EXISTS idx_refresh_tokens_hash;
DROP INDEX IF EXISTS idx_admin_profiles_user_id;

-- 4. Add Missing Performance Indexes for Foreign Keys and High-Frequency Lookups
CREATE INDEX IF NOT EXISTS idx_patient_vitals_recorded_by ON patient_vitals(recorded_by);
CREATE INDEX IF NOT EXISTS idx_patient_med_logs_item ON patient_medication_logs(prescription_item_id);
CREATE INDEX IF NOT EXISTS idx_appointments_patient_doctor_status ON appointments(patient_id, doctor_id, status);
CREATE INDEX IF NOT EXISTS idx_prescriptions_patient_active ON prescriptions(patient_id, status, expires_at);
