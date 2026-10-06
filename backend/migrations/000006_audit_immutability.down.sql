DROP TRIGGER IF EXISTS trg_phi_audit_logs_truncate ON phi_audit_logs;
DROP FUNCTION IF EXISTS prevent_phi_audit_log_truncate();
DROP TRIGGER IF EXISTS trg_phi_audit_logs_immutable ON phi_audit_logs;
DROP FUNCTION IF EXISTS prevent_phi_audit_log_modification();

-- Sanitize any orphan user_ids created while foreign key was detached
UPDATE phi_audit_logs SET user_id = NULL 
WHERE user_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = phi_audit_logs.user_id);

ALTER TABLE phi_audit_logs
ADD CONSTRAINT phi_audit_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
