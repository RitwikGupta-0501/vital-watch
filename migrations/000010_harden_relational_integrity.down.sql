-- Migration: 000010_harden_relational_integrity.down.sql

-- 6. Remove Automated updated_at Triggers
DROP TRIGGER IF EXISTS trg_set_updated_at_doctor_profiles ON doctor_profiles;
DROP TRIGGER IF EXISTS trg_set_updated_at_patient_profiles ON patient_profiles;
DROP TRIGGER IF EXISTS trg_set_updated_at_admin_profiles ON admin_profiles;
DROP TRIGGER IF EXISTS trg_set_updated_at_doctor_schedules ON doctor_schedules;
DROP TRIGGER IF EXISTS trg_set_updated_at_prescriptions ON prescriptions;
DROP TRIGGER IF EXISTS trg_set_updated_at_appointments ON appointments;
DROP TRIGGER IF EXISTS trg_set_updated_at_users ON users;

DROP FUNCTION IF EXISTS set_updated_at_timestamp();

-- 5. Remove Timestamps from Profiles
ALTER TABLE doctor_profiles 
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at;

ALTER TABLE patient_profiles 
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at;

-- 4. Remove Partial Index
DROP INDEX IF EXISTS idx_prescriptions_needs_review;

-- 3. Restore Redundant Indexes
CREATE INDEX IF NOT EXISTS idx_prescriptions_status ON prescriptions(status);
CREATE INDEX IF NOT EXISTS idx_prescriptions_source ON prescriptions(source);

-- 2. Restore Appointments Foreign Key Targets to Users
ALTER TABLE appointments
    DROP CONSTRAINT IF EXISTS appointments_doctor_id_fkey,
    DROP CONSTRAINT IF EXISTS appointments_patient_id_fkey,
    ADD CONSTRAINT appointments_doctor_id_fkey 
        FOREIGN KEY (doctor_id) REFERENCES users(id) ON DELETE RESTRICT,
    ADD CONSTRAINT appointments_patient_id_fkey 
        FOREIGN KEY (patient_id) REFERENCES users(id) ON DELETE RESTRICT;

-- 1. Restore Medication Logs Cascade Deletion (Rollback to unsafe state)
ALTER TABLE patient_medication_logs
    DROP CONSTRAINT IF EXISTS patient_medication_logs_patient_id_fkey,
    DROP CONSTRAINT IF EXISTS patient_medication_logs_prescription_item_id_fkey,
    ADD CONSTRAINT patient_medication_logs_patient_id_fkey 
        FOREIGN KEY (patient_id) REFERENCES patient_profiles(user_id) ON DELETE CASCADE,
    ADD CONSTRAINT patient_medication_logs_prescription_item_id_fkey 
        FOREIGN KEY (prescription_item_id) REFERENCES prescription_items(id) ON DELETE CASCADE;
