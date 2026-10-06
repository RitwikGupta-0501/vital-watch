-- Add tenant_id to admin_profiles
ALTER TABLE admin_profiles ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid REFERENCES tenants(id) ON DELETE CASCADE;

-- Backfill admin_profiles tenant_id from users
UPDATE admin_profiles a
SET tenant_id = u.tenant_id
FROM users u
WHERE a.user_id = u.id;

-- Ensure users_role_check constraint enforces allowed roles
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('patient', 'doctor', 'platform_admin', 'tenant_admin'));
