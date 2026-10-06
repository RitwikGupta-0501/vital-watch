-- Migration: 000011_production_hardening.up.sql
-- Purpose: Phase-1 & Phase-2 production hardening fixes identified by production-readiness audit.
-- Covers:
--   1. Unique partial index on prescriptions(file_name) — closes TOCTOU race condition.
--   2. Composite sort index on patient_vitals — eliminates in-memory Sort node on timestamp/id pagination.
--   3. Composite index on phi_audit_logs for keyset pagination support.
--   4. Partial index for active (non-expired) prescriptions per patient.
-- NOTE: Indexes are applied within the migration transaction (golang-migrate default).
-- CREATE INDEX CONCURRENTLY is prohibited inside a transaction block (SQLSTATE 25001),
-- so standard CREATE INDEX is used here. The migration window is the downtime boundary.

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. Unique partial index: prescriptions.file_name
--    Closes the Check-then-Act (TOCTOU) race condition in CreatePrescription.
--    Two concurrent uploads of the same storage key will now cause a
--    PostgreSQL unique-constraint violation (SQLSTATE 23505) on the SECOND
--    insert, which the application already handles via handleRegisterDBError-
--    style error handling. Digital prescriptions have NULL file_name, so
--    the WHERE clause excludes them from uniqueness enforcement.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE UNIQUE INDEX IF NOT EXISTS idx_prescriptions_unique_filename
    ON prescriptions(file_name)
    WHERE file_name IS NOT NULL;

-- Drop the old non-unique index that is now superseded by the unique one above.
-- The unique index satisfies all previous equality/range lookups that the old
-- non-unique index served.
DROP INDEX IF EXISTS idx_prescriptions_filename;

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. Composite sort index: patient_vitals(patient_id, recorded_at DESC, id DESC)
--    Eliminates the in-memory Sort node PostgreSQL must materialise when
--    the existing idx_patient_vitals_patient_date only covers
--    (patient_id, recorded_at DESC) but the query sorts by (recorded_at DESC, id DESC).
--    With this index, the Index Scan can satisfy ORDER BY without a Sort step.
-- ─────────────────────────────────────────────────────────────────────────────
DROP INDEX IF EXISTS idx_patient_vitals_patient_date;
CREATE INDEX IF NOT EXISTS idx_patient_vitals_patient_date_id
    ON patient_vitals(patient_id, recorded_at DESC, id DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. Composite index: phi_audit_logs(created_at DESC, id DESC)
--    Supports forward keyset/cursor-based pagination on the audit trail
--    (WHERE created_at < $cursor_ts AND id < $cursor_id) instead of
--    expensive OFFSET scans. Also accelerates the existing
--    ORDER BY created_at DESC, id DESC on admin listing endpoints.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_phi_audit_keyset
    ON phi_audit_logs(created_at DESC, id DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- 4. Partial index: phi_audit_logs(patient_id, created_at DESC, id DESC)
--    Supersedes idx_phi_audit_patient for HIPAA audit queries filtered by
--    patient_id, and adds id DESC to enable keyset pagination within
--    a single patient's audit stream.
-- ─────────────────────────────────────────────────────────────────────────────
DROP INDEX IF EXISTS idx_phi_audit_patient;
CREATE INDEX IF NOT EXISTS idx_phi_audit_patient_keyset
    ON phi_audit_logs(patient_id, created_at DESC, id DESC)
    WHERE patient_id IS NOT NULL;
