-- Add new columns to doctors table
ALTER TABLE doctors
ADD COLUMN specialty VARCHAR(100) DEFAULT 'General',
ADD COLUMN experience INT DEFAULT 0,
ADD COLUMN available BOOLEAN DEFAULT TRUE;

-- Create the appointments table
CREATE TABLE IF NOT EXISTS appointments (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    patient_id INT NOT NULL REFERENCES patients(id),
    doctor_id INT NOT NULL REFERENCES doctors(id),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    status VARCHAR(50) DEFAULT 'upcoming', -- e.g., 'upcoming', 'completed'
    appointment_type VARCHAR(100) -- e.g., 'virtual', 'in-person'
);

-- Create the prescriptions table
CREATE TABLE IF NOT EXISTS prescriptions (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    patient_id INT NOT NULL REFERENCES patients(id),
    doctor_id INT NOT NULL REFERENCES doctors(id),
    medication VARCHAR(255) NOT NULL,
    notes TEXT,
    file_name VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT now()
);
