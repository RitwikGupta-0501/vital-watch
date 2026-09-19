-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- Core Users Table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    hashed_password VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL CHECK (role IN ('patient', 'doctor', 'admin')),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Doctor Profiles Table
CREATE TABLE IF NOT EXISTS doctor_profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    specialty VARCHAR(100) DEFAULT 'General',
    experience_years INT DEFAULT 0,
    available BOOLEAN DEFAULT TRUE
);

-- Patient Profiles Table
CREATE TABLE IF NOT EXISTS patient_profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL
);

-- Appointments Table with Double-Booking Exclusion Constraint & Strict Profile Foreign Keys
CREATE TABLE IF NOT EXISTS appointments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id UUID NOT NULL REFERENCES doctor_profiles(user_id) ON DELETE CASCADE,
    patient_id UUID NOT NULL REFERENCES patient_profiles(user_id) ON DELETE CASCADE,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    status VARCHAR(50) DEFAULT 'upcoming' CHECK (status IN ('upcoming', 'completed', 'cancelled')),
    appointment_type VARCHAR(100) DEFAULT 'in_person' CHECK (appointment_type IN ('in_person', 'virtual')),
    created_at TIMESTAMPTZ DEFAULT now(),

    -- Ensure end_time is strictly after start_time
    CONSTRAINT valid_timespan CHECK (end_time > start_time),

    -- Guarantee no two non-cancelled appointments for the same doctor overlap
    EXCLUDE USING gist (
        doctor_id WITH =,
        tstzrange(start_time, end_time) WITH &&
    ) WHERE (status != 'cancelled')
);

-- Prescriptions Table with Strict Profile Foreign Keys
CREATE TABLE IF NOT EXISTS prescriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id UUID NOT NULL REFERENCES patient_profiles(user_id) ON DELETE CASCADE,
    doctor_id UUID NOT NULL REFERENCES doctor_profiles(user_id) ON DELETE CASCADE,
    medication VARCHAR(255) NOT NULL,
    notes TEXT,
    file_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_appointments_doctor_start ON appointments(doctor_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_appointments_patient_start ON appointments(patient_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_prescriptions_patient_created ON prescriptions(patient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_prescriptions_filename ON prescriptions(file_name);
