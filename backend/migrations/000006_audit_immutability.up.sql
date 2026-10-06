-- HIPAA § 164.312(b): Audit trails must be tamper-evident and immutable.
-- 1. Detach ON DELETE SET NULL foreign key constraint so user deletions never
--    trigger cascade updates against immutable audit logs or erase historical user IDs.
ALTER TABLE phi_audit_logs DROP CONSTRAINT IF EXISTS phi_audit_logs_user_id_fkey;

-- 2. Prevent UPDATE and DELETE statements on phi_audit_logs.
CREATE OR REPLACE FUNCTION prevent_phi_audit_log_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Audit logs are immutable and cannot be updated or deleted under HIPAA compliance (45 CFR § 164.312(b))';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_phi_audit_logs_immutable ON phi_audit_logs;
CREATE TRIGGER trg_phi_audit_logs_immutable
BEFORE UPDATE OR DELETE ON phi_audit_logs
FOR EACH ROW
EXECUTE FUNCTION prevent_phi_audit_log_modification();

-- 3. Prevent TRUNCATE statements on phi_audit_logs.
CREATE OR REPLACE FUNCTION prevent_phi_audit_log_truncate()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Audit logs are immutable and cannot be truncated under HIPAA compliance (45 CFR § 164.312(b))';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_phi_audit_logs_truncate ON phi_audit_logs;
CREATE TRIGGER trg_phi_audit_logs_truncate
BEFORE TRUNCATE ON phi_audit_logs
FOR EACH STATEMENT
EXECUTE FUNCTION prevent_phi_audit_log_truncate();
