-- Migration: 000010_harden_relational_integrity.up.sql

-- 1. Protect Medication Logs from Cascade Deletion (HIPAA § 164.312)
ALTER TABLE patient_medication_logs
    DROP CONSTRAINT IF EXISTS patient_medication_logs_patient_id_fkey,
    DROP CONSTRAINT IF EXISTS patient_medication_logs_prescription_item_id_fkey,
    ADD CONSTRAINT patient_medication_logs_patient_id_fkey 
        FOREIGN KEY (patient_id) REFERENCES patient_profiles(user_id) ON DELETE RESTRICT,
    ADD CONSTRAINT patient_medication_logs_prescription_item_id_fkey 
        FOREIGN KEY (prescription_item_id) REFERENCES prescription_items(id) ON DELETE RESTRICT;

-- 2. Correct Appointments Foreign Key Targets (Re-align with Profiles)
ALTER TABLE appointments
    DROP CONSTRAINT IF EXISTS appointments_doctor_id_fkey,
    DROP CONSTRAINT IF EXISTS appointments_patient_id_fkey,
    ADD CONSTRAINT appointments_doctor_id_fkey 
        FOREIGN KEY (doctor_id) REFERENCES doctor_profiles(user_id) ON DELETE RESTRICT,
    ADD CONSTRAINT appointments_patient_id_fkey 
        FOREIGN KEY (patient_id) REFERENCES patient_profiles(user_id) ON DELETE RESTRICT;

-- 3. Drop Redundant / Low-Cardinality Indexes
DROP INDEX IF EXISTS idx_prescriptions_source;
DROP INDEX IF EXISTS idx_prescriptions_status;

-- 4. Add Partial Index for Doctor Review Inbox
CREATE INDEX IF NOT EXISTS idx_prescriptions_needs_review 
ON prescriptions(doctor_id, created_at DESC) 
WHERE status = 'needs_review';

-- 5. Add Missing Timestamps to Profiles
ALTER TABLE patient_profiles 
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE doctor_profiles 
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- 6. Add Automated updated_at Trigger
CREATE OR REPLACE FUNCTION set_updated_at_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_set_updated_at_users ON users;
CREATE TRIGGER trg_set_updated_at_users
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at_timestamp();

DROP TRIGGER IF EXISTS trg_set_updated_at_appointments ON appointments;
CREATE TRIGGER trg_set_updated_at_appointments
BEFORE UPDATE ON appointments
FOR EACH ROW EXECUTE FUNCTION set_updated_at_timestamp();

DROP TRIGGER IF EXISTS trg_set_updated_at_prescriptions ON prescriptions;
CREATE TRIGGER trg_set_updated_at_prescriptions
BEFORE UPDATE ON prescriptions
FOR EACH ROW EXECUTE FUNCTION set_updated_at_timestamp();

DROP TRIGGER IF EXISTS trg_set_updated_at_doctor_schedules ON doctor_schedules;
CREATE TRIGGER trg_set_updated_at_doctor_schedules
BEFORE UPDATE ON doctor_schedules
FOR EACH ROW EXECUTE FUNCTION set_updated_at_timestamp();

DROP TRIGGER IF EXISTS trg_set_updated_at_admin_profiles ON admin_profiles;
CREATE TRIGGER trg_set_updated_at_admin_profiles
BEFORE UPDATE ON admin_profiles
FOR EACH ROW EXECUTE FUNCTION set_updated_at_timestamp();

DROP TRIGGER IF EXISTS trg_set_updated_at_patient_profiles ON patient_profiles;
CREATE TRIGGER trg_set_updated_at_patient_profiles
BEFORE UPDATE ON patient_profiles
FOR EACH ROW EXECUTE FUNCTION set_updated_at_timestamp();

DROP TRIGGER IF EXISTS trg_set_updated_at_doctor_profiles ON doctor_profiles;
CREATE TRIGGER trg_set_updated_at_doctor_profiles
BEFORE UPDATE ON doctor_profiles
FOR EACH ROW EXECUTE FUNCTION set_updated_at_timestamp();
