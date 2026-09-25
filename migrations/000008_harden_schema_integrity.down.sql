-- Migration: 000008_harden_schema_integrity.down.sql
-- Goal: Clean rollback restoring previous cascade and index definitions.

-- 1. Drop Added Performance Indexes
DROP INDEX IF EXISTS idx_prescriptions_patient_active;
DROP INDEX IF EXISTS idx_appointments_patient_doctor_status;
DROP INDEX IF EXISTS idx_patient_med_logs_item;
DROP INDEX IF EXISTS idx_patient_vitals_recorded_by;

-- 2. Restore Duplicate Indexes
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_admin_profiles_user_id ON admin_profiles(user_id);

-- 3. Revert Constraints and Column Definitions
ALTER TABLE doctor_profiles DROP CONSTRAINT IF EXISTS chk_doctor_experience_positive;

ALTER TABLE appointments DROP COLUMN IF EXISTS updated_at;
ALTER TABLE appointments ALTER COLUMN appointment_type DROP NOT NULL;
ALTER TABLE appointments ALTER COLUMN status DROP NOT NULL;
ALTER TABLE users ALTER COLUMN is_active DROP NOT NULL;

-- 4. Revert Foreign Keys to CASCADE
ALTER TABLE prescriptions 
    DROP CONSTRAINT IF EXISTS prescriptions_doctor_id_fkey,
    DROP CONSTRAINT IF EXISTS prescriptions_patient_id_fkey,
    ADD CONSTRAINT prescriptions_doctor_id_fkey FOREIGN KEY (doctor_id) REFERENCES doctor_profiles(user_id) ON DELETE CASCADE,
    ADD CONSTRAINT prescriptions_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES patient_profiles(user_id) ON DELETE CASCADE;

ALTER TABLE appointments 
    DROP CONSTRAINT IF EXISTS appointments_doctor_id_fkey,
    DROP CONSTRAINT IF EXISTS appointments_patient_id_fkey,
    ADD CONSTRAINT appointments_doctor_id_fkey FOREIGN KEY (doctor_id) REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT appointments_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE patient_vitals 
    DROP CONSTRAINT IF EXISTS patient_vitals_patient_id_fkey,
    DROP CONSTRAINT IF EXISTS patient_vitals_recorded_by_fkey,
    ADD CONSTRAINT patient_vitals_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES patient_profiles(user_id) ON DELETE CASCADE,
    ADD CONSTRAINT patient_vitals_recorded_by_fkey FOREIGN KEY (recorded_by) REFERENCES users(id) ON DELETE CASCADE;
