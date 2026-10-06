-- Create tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Create tenant invites table
CREATE TABLE IF NOT EXISTS tenant_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    invite_code VARCHAR(255) UNIQUE NOT NULL,
    role VARCHAR(50) NOT NULL CHECK (role IN ('tenant_admin', 'doctor')),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- We need a System Default Tenant for platform admins and backward compatibility
INSERT INTO tenants (id, name, domain) 
VALUES ('00000000-0000-0000-0000-000000000000'::uuid, 'System Default', 'system.local') 
ON CONFLICT (id) DO NOTHING;

-- Update users role constraint
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('patient', 'doctor', 'platform_admin', 'tenant_admin'));

-- Migrate any existing 'admin' to 'platform_admin'
UPDATE users SET role = 'platform_admin' WHERE role = 'admin';

-- Add tenant_id to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;

-- Add tenant_id to other core tables
ALTER TABLE doctor_profiles ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE patient_profiles ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE prescriptions ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE prescription_items ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE doctor_schedules ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE patient_vitals ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE patient_medication_logs ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE phi_audit_logs ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;

-- Update existing tenant_settings and ocr configs to reference the new table
ALTER TABLE ocr_provider_configs DROP CONSTRAINT IF EXISTS ocr_provider_configs_tenant_id_fkey;
ALTER TABLE ocr_provider_configs ADD CONSTRAINT ocr_provider_configs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE tenant_settings DROP CONSTRAINT IF EXISTS tenant_settings_tenant_id_fkey;
ALTER TABLE tenant_settings ADD CONSTRAINT tenant_settings_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;


-- Enable RLS on core tables
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE doctor_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE patient_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE appointments ENABLE ROW LEVEL SECURITY;
ALTER TABLE prescriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE prescription_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE doctor_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE patient_vitals ENABLE ROW LEVEL SECURITY;
ALTER TABLE patient_medication_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE phi_audit_logs ENABLE ROW LEVEL SECURITY;

-- Create a shared policy for tenant isolation with Platform Admin bypass
-- The bypass uses `current_setting('app.is_platform_admin', true) = 'true'`
CREATE OR REPLACE FUNCTION current_tenant_id() RETURNS UUID AS $$
BEGIN
    RETURN current_setting('app.current_tenant', true)::UUID;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION is_platform_admin() RETURNS BOOLEAN AS $$
BEGIN
    RETURN current_setting('app.is_platform_admin', true) = 'true';
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql STABLE;

-- RLS Policy definition (allow if row tenant_id matches session tenant_id OR user is platform admin)
DO $$ 
DECLARE 
    tname text;
BEGIN
    FOR tname IN SELECT unnest(ARRAY['users', 'doctor_profiles', 'patient_profiles', 'appointments', 'prescriptions', 'prescription_items', 'doctor_schedules', 'patient_vitals', 'patient_medication_logs', 'phi_audit_logs'])
    LOOP
        EXECUTE format('CREATE POLICY tenant_isolation_policy ON %I FOR ALL USING (tenant_id = current_tenant_id() OR is_platform_admin());', tname);
    END LOOP;
END $$;
