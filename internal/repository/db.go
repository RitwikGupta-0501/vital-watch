package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

// Repository is the struct that holds our database connection
type Repository struct {
	DB *sql.DB
}

// Patient Related Methods
func (r *Repository) CreatePatient(ctx context.Context, firstName, lastName, email, hashedPassword string) (uuid.UUID, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()

	userQuery := `
		INSERT INTO users (email, hashed_password, role)
		VALUES ($1, $2, 'patient')
		RETURNING id
	`
	var newID uuid.UUID
	err = tx.QueryRowContext(ctx, userQuery, email, hashedPassword).Scan(&newID)
	if err != nil {
		return uuid.Nil, err
	}

	profileQuery := `
		INSERT INTO patient_profiles (user_id, first_name, last_name)
		VALUES ($1, $2, $3)
	`
	_, err = tx.ExecContext(ctx, profileQuery, newID, firstName, lastName)
	if err != nil {
		return uuid.Nil, err
	}

	if err = tx.Commit(); err != nil {
		return uuid.Nil, err
	}

	return newID, nil
}

func (r *Repository) GetPatientByEmail(ctx context.Context, email string) (models.Patient, error) {
	query := `
		SELECT u.id, u.email, p.first_name, p.last_name, u.hashed_password, u.created_at
		FROM users u
		JOIN patient_profiles p ON u.id = p.user_id
		WHERE u.email = $1 AND u.role = 'patient' AND u.is_active = true
	`
	var user models.Patient
	err := r.DB.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.HashedPassword, &user.CreatedAt,
	)
	if err != nil {
		return models.Patient{}, err
	}
	user.Role = "patient"
	return user, nil
}

func (r *Repository) GetPatientByID(ctx context.Context, id uuid.UUID) (models.Patient, error) {
	query := `
		SELECT u.id, u.email, p.first_name, p.last_name, u.hashed_password, u.created_at
		FROM users u
		JOIN patient_profiles p ON u.id = p.user_id
		WHERE u.id = $1 AND u.role = 'patient' AND u.is_active = true
	`
	var user models.Patient
	err := r.DB.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.HashedPassword, &user.CreatedAt,
	)
	if err != nil {
		return models.Patient{}, err
	}
	user.Role = "patient"
	return user, nil
}

func (r *Repository) GetPatientsByDoctorID(ctx context.Context, doctorID uuid.UUID) ([]models.Patient, error) {
	query := `
		SELECT DISTINCT u.id, u.email, p.first_name, p.last_name, u.created_at
		FROM users u
		JOIN patient_profiles p ON u.id = p.user_id
		JOIN appointments a ON u.id = a.patient_id
		WHERE a.doctor_id = $1
		ORDER BY u.created_at DESC
	`
	rows, err := r.DB.QueryContext(ctx, query, doctorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patients []models.Patient
	for rows.Next() {
		var patient models.Patient
		err := rows.Scan(&patient.ID, &patient.Email, &patient.FirstName, &patient.LastName, &patient.CreatedAt)
		if err != nil {
			return nil, err
		}
		patient.Role = "patient"
		patients = append(patients, patient)
	}
	return patients, nil
}

// Doctor Related Methods
func (r *Repository) CreateDoctor(ctx context.Context, firstName, lastName, email, hashedPassword, specialty string, experience int) (uuid.UUID, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()

	userQuery := `
		INSERT INTO users (email, hashed_password, role)
		VALUES ($1, $2, 'doctor')
		RETURNING id
	`
	var newID uuid.UUID
	err = tx.QueryRowContext(ctx, userQuery, email, hashedPassword).Scan(&newID)
	if err != nil {
		return uuid.Nil, err
	}

	profileQuery := `
		INSERT INTO doctor_profiles (user_id, first_name, last_name, specialty, experience_years)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err = tx.ExecContext(ctx, profileQuery, newID, firstName, lastName, specialty, experience)
	if err != nil {
		return uuid.Nil, err
	}

	if err = tx.Commit(); err != nil {
		return uuid.Nil, err
	}

	return newID, nil
}

func (r *Repository) GetDoctorByEmail(ctx context.Context, email string) (models.Doctor, error) {
	query := `
		SELECT u.id, u.email, d.first_name, d.last_name, u.hashed_password, d.specialty, d.experience_years, d.available, u.created_at
		FROM users u
		JOIN doctor_profiles d ON u.id = d.user_id
		WHERE u.email = $1 AND u.role = 'doctor' AND u.is_active = true
	`
	var user models.Doctor
	err := r.DB.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.HashedPassword,
		&user.Specialty, &user.Experience, &user.Available, &user.CreatedAt,
	)
	if err != nil {
		return models.Doctor{}, err
	}
	user.Role = "doctor"
	return user, nil
}

func (r *Repository) GetDoctorByID(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
	query := `
		SELECT u.id, u.email, d.first_name, d.last_name, u.hashed_password, d.specialty, d.experience_years, d.available, u.created_at
		FROM users u
		JOIN doctor_profiles d ON u.id = d.user_id
		WHERE u.id = $1 AND u.role = 'doctor' AND u.is_active = true
	`
	var user models.Doctor
	err := r.DB.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.HashedPassword,
		&user.Specialty, &user.Experience, &user.Available, &user.CreatedAt,
	)
	if err != nil {
		return models.Doctor{}, err
	}
	user.Role = "doctor"
	return user, nil
}

func (r *Repository) GetDoctors(ctx context.Context) ([]models.Doctor, error) {
	query := `
		SELECT u.id, u.email, d.first_name, d.last_name, d.specialty, d.experience_years, d.available, u.created_at
		FROM users u
		JOIN doctor_profiles d ON u.id = d.user_id
		WHERE u.role = 'doctor' AND u.is_active = true
		ORDER BY d.first_name ASC
	`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var doctors []models.Doctor
	for rows.Next() {
		var doc models.Doctor
		err := rows.Scan(
			&doc.ID, &doc.Email, &doc.FirstName, &doc.LastName, &doc.Specialty,
			&doc.Experience, &doc.Available, &doc.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		doc.Role = "doctor"
		doctors = append(doctors, doc)
	}
	return doctors, nil
}

// Appointment Related Methods
func (r *Repository) CreateAppointment(ctx context.Context, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType string) (uuid.UUID, error) {
	query := `
		INSERT INTO appointments (patient_id, doctor_id, start_time, end_time, appointment_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	var newID uuid.UUID
	err := r.DB.QueryRowContext(ctx, query, patientID, doctorID, startTime, endTime, apptType).Scan(&newID)
	if err != nil {
		return uuid.Nil, err
	}
	return newID, nil
}

func (r *Repository) GetAppointmentsByDoctorID(ctx context.Context, doctorID uuid.UUID) ([]models.Appointment, error) {
	query := `
		SELECT 
			a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, a.created_at,
			p.first_name, p.last_name
		FROM appointments a
		JOIN patient_profiles p ON a.patient_id = p.user_id
		WHERE a.doctor_id = $1
		ORDER BY a.start_time DESC
	`
	rows, err := r.DB.QueryContext(ctx, query, doctorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appointments []models.Appointment
	for rows.Next() {
		var appt models.Appointment
		var patientFirstName, patientLastName string

		err := rows.Scan(
			&appt.ID, &appt.PatientID, &appt.DoctorID, &appt.StartTime, &appt.EndTime,
			&appt.Status, &appt.Type, &appt.CreatedAt, &patientFirstName, &patientLastName,
		)
		if err != nil {
			return nil, err
		}

		appt.PatientName = patientFirstName + " " + patientLastName
		appointments = append(appointments, appt)
	}
	return appointments, nil
}

func (r *Repository) GetAppointmentsByPatientID(ctx context.Context, patientID uuid.UUID) ([]models.Appointment, error) {
	query := `
		SELECT 
			a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, a.created_at,
			d.first_name, d.last_name, d.specialty
		FROM appointments a
		JOIN doctor_profiles d ON a.doctor_id = d.user_id
		WHERE a.patient_id = $1
		ORDER BY a.start_time DESC
	`
	rows, err := r.DB.QueryContext(ctx, query, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appointments []models.Appointment
	for rows.Next() {
		var appt models.Appointment
		var docFirstName, docLastName, docSpecialty string

		err := rows.Scan(
			&appt.ID, &appt.PatientID, &appt.DoctorID, &appt.StartTime, &appt.EndTime,
			&appt.Status, &appt.Type, &appt.CreatedAt, &docFirstName, &docLastName, &docSpecialty,
		)
		if err != nil {
			return nil, err
		}

		appt.DoctorName = docFirstName + " " + docLastName
		appt.DoctorSpecialty = docSpecialty
		appointments = append(appointments, appt)
	}
	return appointments, nil
}

func (r *Repository) GetAppointmentsForPatient(ctx context.Context, doctorID, patientID uuid.UUID) ([]models.Appointment, error) {
	query := `
		SELECT 
			a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type, a.created_at,
			d.first_name, d.last_name, d.specialty
		FROM appointments a
		JOIN doctor_profiles d ON a.doctor_id = d.user_id
		WHERE a.patient_id = $1 AND a.doctor_id = $2
		ORDER BY a.start_time DESC
	`
	rows, err := r.DB.QueryContext(ctx, query, patientID, doctorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appointments []models.Appointment
	for rows.Next() {
		var appt models.Appointment
		var docFirstName, docLastName, docSpecialty string

		err := rows.Scan(
			&appt.ID, &appt.PatientID, &appt.DoctorID, &appt.StartTime, &appt.EndTime,
			&appt.Status, &appt.Type, &appt.CreatedAt, &docFirstName, &docLastName, &docSpecialty,
		)
		if err != nil {
			return nil, err
		}

		appt.DoctorName = docFirstName + " " + docLastName
		appt.DoctorSpecialty = docSpecialty
		appointments = append(appointments, appt)
	}
	return appointments, nil
}

func (r *Repository) UpdateAppointmentAsCompletedForDoctor(ctx context.Context, appointmentID, doctorID uuid.UUID) (bool, error) {
	query := `UPDATE appointments SET status = 'completed' WHERE id = $1 AND doctor_id = $2`
	result, err := r.DB.ExecContext(ctx, query, appointmentID, doctorID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// Prescription Related Methods
func (r *Repository) CreatePrescription(ctx context.Context, patientID, doctorID uuid.UUID, medication, notes, fileName string) (uuid.UUID, error) {
	query := `
		INSERT INTO prescriptions (patient_id, doctor_id, medication, notes, file_name)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	var newID uuid.UUID
	err := r.DB.QueryRowContext(ctx, query, patientID, doctorID, medication, notes, fileName).Scan(&newID)
	if err != nil {
		return uuid.Nil, err
	}
	return newID, nil
}

func (r *Repository) GetPrescriptionsByPatientID(ctx context.Context, patientID uuid.UUID) ([]models.Prescription, error) {
	query := `
		SELECT p.id, p.patient_id, p.doctor_id, p.medication, p.notes, p.file_name, p.created_at, d.first_name, d.last_name
		FROM prescriptions p
		JOIN doctor_profiles d ON p.doctor_id = d.user_id
		WHERE p.patient_id = $1
		ORDER BY p.created_at DESC
	`
	rows, err := r.DB.QueryContext(ctx, query, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prescriptions []models.Prescription
	for rows.Next() {
		var pres models.Prescription
		var docFirstName, docLastName string
		err := rows.Scan(
			&pres.ID, &pres.PatientID, &pres.DoctorID, &pres.Medication, &pres.Notes,
			&pres.FileName, &pres.CreatedAt, &docFirstName, &docLastName,
		)
		if err != nil {
			return nil, err
		}
		pres.DoctorName = docFirstName + " " + docLastName
		prescriptions = append(prescriptions, pres)
	}
	return prescriptions, nil
}

func (r *Repository) GetPrescriptionByFilename(ctx context.Context, patientID uuid.UUID, filename string) (models.Prescription, error) {
	query := `SELECT id FROM prescriptions WHERE patient_id = $1 AND file_name = $2`
	var pres models.Prescription
	err := r.DB.QueryRowContext(ctx, query, patientID, filename).Scan(&pres.ID)
	return pres, err
}

func (r *Repository) GetPrescriptionsForPatient(ctx context.Context, doctorID, patientID uuid.UUID) ([]models.Prescription, error) {
	query := `
		SELECT 
			p.id, p.patient_id, p.doctor_id, p.medication, p.notes, p.file_name, p.created_at, 
			d.first_name, d.last_name
		FROM prescriptions p
		JOIN doctor_profiles d ON p.doctor_id = d.user_id
		WHERE p.patient_id = $1 AND EXISTS (
			SELECT 1 FROM appointments WHERE patient_id = $1 AND doctor_id = $2
		)
		ORDER BY p.created_at DESC
	`
	rows, err := r.DB.QueryContext(ctx, query, patientID, doctorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prescriptions []models.Prescription
	for rows.Next() {
		var pres models.Prescription
		var docFirstName, docLastName string
		err := rows.Scan(
			&pres.ID, &pres.PatientID, &pres.DoctorID, &pres.Medication, &pres.Notes,
			&pres.FileName, &pres.CreatedAt, &docFirstName, &docLastName,
		)
		if err != nil {
			return nil, err
		}
		pres.DoctorName = docFirstName + " " + docLastName
		prescriptions = append(prescriptions, pres)
	}
	return prescriptions, nil
}

func (r *Repository) GetPrescriptionByFilenameForDoctor(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error) {
	query := `
		SELECT p.id 
		FROM prescriptions p
		LEFT JOIN appointments a ON p.patient_id = a.patient_id AND a.doctor_id = $2
		WHERE p.file_name = $1 AND (p.doctor_id = $2 OR a.doctor_id = $2)
		LIMIT 1
	`
	var pres models.Prescription
	err := r.DB.QueryRowContext(ctx, query, filename, doctorID).Scan(&pres.ID)
	return pres, err
}
