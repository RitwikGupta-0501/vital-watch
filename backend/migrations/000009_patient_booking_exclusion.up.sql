-- Add GiST exclusion constraint on patient_id to prevent concurrent patient double-booking across different doctors
ALTER TABLE appointments ADD CONSTRAINT uq_patient_no_overlap
EXCLUDE USING gist (
    patient_id WITH =,
    tstzrange(start_time, end_time) WITH &&
) WHERE (status != 'cancelled');
