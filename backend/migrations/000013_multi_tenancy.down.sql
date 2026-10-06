DO $$ 
DECLARE 
    tname text;
BEGIN
    FOR tname IN SELECT unnest(ARRAY['users', 'doctor_profiles', 'patient_profiles', 'appointments', 'prescriptions', 'prescription_items', 'doctor_schedules', 'patient_vitals', 'patient_medication_logs', 'phi_audit_logs'])
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_policy ON %I;', tname);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY;', tname);
    END LOOP;
END $$;
DROP FUNCTION IF EXISTS current_tenant_id();
DROP FUNCTION IF EXISTS is_platform_admin();
ALTER TABLE ocr_provider_configs DROP CONSTRAINT IF EXISTS ocr_provider_configs_tenant_id_fkey;
ALTER TABLE tenant_settings DROP CONSTRAINT IF EXISTS tenant_settings_tenant_id_fkey;

ALTER TABLE phi_audit_logs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE patient_medication_logs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE patient_vitals DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE doctor_schedules DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE prescription_items DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE prescriptions DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE appointments DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE patient_profiles DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE doctor_profiles DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE users DROP COLUMN IF EXISTS tenant_id;

UPDATE users SET role = 'admin' WHERE role = 'platform_admin' OR role = 'tenant_admin';

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('patient', 'doctor', 'admin'));

DROP TABLE IF EXISTS tenant_invites CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;
