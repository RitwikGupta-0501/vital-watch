DROP INDEX IF EXISTS idx_prescription_items_prescription;
DROP INDEX IF EXISTS idx_prescriptions_doctor_status;
DROP INDEX IF EXISTS idx_prescriptions_source;
DROP INDEX IF EXISTS idx_prescriptions_status;

DROP TABLE IF EXISTS prescription_items;

ALTER TABLE prescriptions
    DROP COLUMN IF EXISTS ocr_provider,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS source;

-- Restore NOT NULL on file_name and medication after cleaning any NULL values from digital records
UPDATE prescriptions SET file_name = 'legacy-unknown' WHERE file_name IS NULL;
UPDATE prescriptions SET medication = 'legacy-unknown' WHERE medication IS NULL;
ALTER TABLE prescriptions
    ALTER COLUMN file_name SET NOT NULL,
    ALTER COLUMN medication SET NOT NULL;
