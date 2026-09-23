DROP TRIGGER IF EXISTS trg_phi_audit_logs_truncate ON phi_audit_logs;
DROP FUNCTION IF EXISTS prevent_phi_audit_log_truncate();
DROP TRIGGER IF EXISTS trg_phi_audit_logs_immutable ON phi_audit_logs;
DROP FUNCTION IF EXISTS prevent_phi_audit_log_modification();

ALTER TABLE phi_audit_logs
ADD CONSTRAINT phi_audit_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
