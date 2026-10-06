-- Doctor Working Schedules
CREATE TABLE IF NOT EXISTS doctor_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id UUID NOT NULL REFERENCES doctor_profiles(user_id) ON DELETE CASCADE,
    day_of_week INT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    slot_duration INT NOT NULL DEFAULT 30 CHECK (slot_duration IN (15, 20, 30, 45, 60)),
    timezone VARCHAR(100) NOT NULL DEFAULT 'UTC',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    CONSTRAINT valid_schedule_times CHECK (end_time > start_time),
    CONSTRAINT uq_doctor_day UNIQUE (doctor_id, day_of_week)
);

-- Telehealth Virtual Appointment Extensions
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS meeting_link TEXT;
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS meeting_id VARCHAR(255);

-- Prescription Expiry & Active Status Extension
ALTER TABLE prescriptions ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- Longitudinal Patient Vitals
CREATE TABLE IF NOT EXISTS patient_vitals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id UUID NOT NULL REFERENCES patient_profiles(user_id) ON DELETE CASCADE,
    recorded_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    systolic_bp INT CHECK (systolic_bp BETWEEN 50 AND 300),
    diastolic_bp INT CHECK (diastolic_bp BETWEEN 30 AND 200),
    heart_rate INT CHECK (heart_rate BETWEEN 30 AND 250),
    blood_glucose NUMERIC(6,2) CHECK (blood_glucose BETWEEN 10 AND 1000),
    oxygen_saturation NUMERIC(4,1) CHECK (oxygen_saturation BETWEEN 50.0 AND 100.0),
    temperature NUMERIC(4,1) CHECK (temperature BETWEEN 30.0 AND 45.0),
    weight_kg NUMERIC(5,2) CHECK (weight_kg BETWEEN 1.0 AND 500.0),
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    CONSTRAINT valid_blood_pressure CHECK (systolic_bp IS NULL OR diastolic_bp IS NULL OR systolic_bp > diastolic_bp),
    CONSTRAINT at_least_one_vital_metric CHECK (
        systolic_bp IS NOT NULL OR diastolic_bp IS NOT NULL OR heart_rate IS NOT NULL OR
        blood_glucose IS NOT NULL OR oxygen_saturation IS NOT NULL OR temperature IS NOT NULL OR
        weight_kg IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_patient_vitals_patient_date ON patient_vitals(patient_id, recorded_at DESC);

-- Patient Daily Medication Adherence Logs
CREATE TABLE IF NOT EXISTS patient_medication_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id UUID NOT NULL REFERENCES patient_profiles(user_id) ON DELETE CASCADE,
    prescription_item_id UUID NOT NULL REFERENCES prescription_items(id) ON DELETE CASCADE,
    scheduled_date DATE NOT NULL,
    time_of_day VARCHAR(50) NOT NULL CHECK (time_of_day IN ('morning', 'afternoon', 'evening', 'bedtime', 'as_needed')),
    dose_number INT NOT NULL DEFAULT 1 CHECK (dose_number >= 1),
    meal_timing VARCHAR(100),
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'taken', 'skipped')),
    taken_at TIMESTAMPTZ,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    CONSTRAINT uq_patient_med_slot UNIQUE (patient_id, prescription_item_id, scheduled_date, time_of_day, dose_number)
);

CREATE INDEX IF NOT EXISTS idx_patient_med_logs_patient_date ON patient_medication_logs(patient_id, scheduled_date DESC);
