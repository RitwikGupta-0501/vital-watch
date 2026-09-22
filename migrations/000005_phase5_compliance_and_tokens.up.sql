-- Refresh Tokens for Token Rotation & Session Management
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    replaced_by_token_id UUID REFERENCES refresh_tokens(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id, expires_at DESC);

-- HIPAA ePHI Access Audit Logs
CREATE TABLE IF NOT EXISTS phi_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    user_role VARCHAR(50),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID,
    patient_id UUID,
    ip_address VARCHAR(45),
    user_agent TEXT,
    request_id UUID,
    status_code INT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_phi_audit_patient ON phi_audit_logs(patient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_phi_audit_user ON phi_audit_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_phi_audit_created ON phi_audit_logs(created_at DESC);
