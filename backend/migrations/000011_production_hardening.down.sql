-- Migration: 000011_production_hardening.down.sql
-- Reverses all changes from 000011_production_hardening.up.sql

-- Restore the original non-unique filename index
DROP INDEX IF EXISTS idx_prescriptions_unique_filename;
CREATE INDEX IF NOT EXISTS idx_prescriptions_filename ON prescriptions(file_name);

-- Restore the original patient_vitals sort index (without id column)
DROP INDEX IF EXISTS idx_patient_vitals_patient_date_id;
CREATE INDEX IF NOT EXISTS idx_patient_vitals_patient_date ON patient_vitals(patient_id, recorded_at DESC);

-- Drop the keyset pagination indexes for audit logs
DROP INDEX IF EXISTS idx_phi_audit_keyset;
DROP INDEX IF EXISTS idx_phi_audit_patient_keyset;
CREATE INDEX IF NOT EXISTS idx_phi_audit_patient ON phi_audit_logs(patient_id, created_at DESC);
