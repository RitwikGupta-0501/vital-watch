-- Stores individual provider configurations (The Tenant's Keys)
-- Note: '00000000-0000-0000-0000-000000000000' is the System Default Tenant ID used prior to full multi-tenancy.
CREATE TABLE IF NOT EXISTS ocr_provider_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid,
    provider_name VARCHAR(50) NOT NULL,
    encrypted_key BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Stores the Tenant Admin's settings (The Tenant's Chain)
-- Note: '00000000-0000-0000-0000-000000000000' is the System Default Tenant ID.
CREATE TABLE IF NOT EXISTS tenant_settings (
    tenant_id UUID PRIMARY KEY DEFAULT '00000000-0000-0000-0000-000000000000'::uuid,
    ocr_fallback_chain UUID[] NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ DEFAULT now()
);
