-- Evolve prescriptions table to dual-mode (uploaded + digital e-prescribing)
ALTER TABLE prescriptions
    ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'uploaded' CHECK (source IN ('uploaded', 'digital')),
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'approved' CHECK (status IN ('pending_ocr', 'needs_review', 'approved', 'rejected')),
    ADD COLUMN IF NOT EXISTS ocr_provider VARCHAR(50),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT now(),
    ALTER COLUMN file_name DROP NOT NULL,
    ALTER COLUMN medication DROP NOT NULL;

-- Multi-Medication Relational Support
CREATE TABLE IF NOT EXISTS prescription_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prescription_id UUID NOT NULL REFERENCES prescriptions(id) ON DELETE CASCADE,
    medication_name VARCHAR(255) NOT NULL,
    dosage VARCHAR(100),
    frequency VARCHAR(100),
    duration VARCHAR(100),
    timing VARCHAR(100),
    instructions TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Additional Performance Indexes
CREATE INDEX IF NOT EXISTS idx_prescriptions_status ON prescriptions(status);
CREATE INDEX IF NOT EXISTS idx_prescriptions_source ON prescriptions(source);
CREATE INDEX IF NOT EXISTS idx_prescriptions_doctor_status ON prescriptions(doctor_id, status);
CREATE INDEX IF NOT EXISTS idx_prescription_items_prescription ON prescription_items(prescription_id);

-- Backfill legacy medication data into prescription_items
INSERT INTO prescription_items (prescription_id, medication_name, created_at)
SELECT id, medication, created_at
FROM prescriptions p
WHERE medication IS NOT NULL AND TRIM(medication) <> ''
  AND NOT EXISTS (
      SELECT 1 FROM prescription_items pi WHERE pi.prescription_id = p.id
  );
