package repository

import (
	"database/sql"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

// Repository is the struct that holds our database connection
type Repository struct {
	DB *sql.DB
}

// Patient Related Methods
func (r *Repository) CreatePatient(firstName, lastName, email, hashedPassword string) (int, error) {
	query := `
		INSERT INTO patients (firstName, lastName, email, hashedPassword)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	var newID int

	// Pass all 4 arguments
	err := r.DB.QueryRow(query, firstName, lastName, email, hashedPassword).Scan(&newID)
	if err != nil {
		return 0, err
	}

	return newID, nil
}

func (r *Repository) GetPatientByEmail(email string) (models.Patient, error) {
	query := `SELECT id, firstName, lastName, email, hashedPassword FROM patients WHERE email=$1`

	var user models.Patient

	err := r.DB.QueryRow(query, email).Scan(&user.ID, &user.FirstName, &user.LastName, &user.Email, &user.HashedPassword)
	if err != nil {
		return models.Patient{}, err
	}

	return user, nil
}

func (r *Repository) GetPatientByID(id int) (models.Patient, error) {
	query := `SELECT id, firstName, lastName, email, hashedPassword FROM patients WHERE id=$1`

	var user models.Patient
	err := r.DB.QueryRow(query, id).Scan(&user.ID, &user.FirstName, &user.LastName, &user.Email, &user.HashedPassword)
	if err != nil {
		return models.Patient{}, err
	}

	return user, nil
}

func (r *Repository) GetPatientsByDoctorID(doctorID int) ([]models.Patient, error) {
	query := `
		SELECT DISTINCT p.id, p.firstName, p.lastName, p.email, p.createdAt
		FROM patients p
		JOIN appointments a ON p.id = a.patient_id
		WHERE a.doctor_id = $1`

	rows, err := r.DB.Query(query, doctorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patients []models.Patient
	for rows.Next() {
		var patient models.Patient
		err := rows.Scan(&patient.ID, &patient.FirstName, &patient.LastName, &patient.Email, &patient.CreatedAt)
		if err != nil {
			return nil, err
		}
		patients = append(patients, patient)
	}

	return patients, nil
}

// Doctor Related Methods
func (r *Repository) CreateDoctor(firstName, lastName, email, hashedPassword, specialty string, experience int) (int, error) {
	query := `
		INSERT INTO doctors (firstName, lastName, email, hashedPassword, specialty, experience)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	var newID int

	err := r.DB.QueryRow(query, firstName, lastName, email, hashedPassword, specialty, experience).Scan(&newID)
	if err != nil {
		return 0, err
	}

	return newID, nil
}

func (r *Repository) GetDoctorByEmail(email string) (models.Doctor, error) {
	query := `SELECT id, firstName, lastName, email, hashedPassword FROM doctors WHERE email=$1`

	var user models.Doctor
	err := r.DB.QueryRow(query, email).Scan(&user.ID, &user.FirstName, &user.LastName, &user.Email, &user.HashedPassword)
	if err != nil {
		return models.Doctor{}, err
	}

	return user, nil
}

func (r *Repository) GetDoctorByID(id int) (models.Doctor, error) {
	query := `SELECT id, firstName, lastName, email, hashedPassword FROM doctors WHERE id=$1`

	var user models.Doctor
	err := r.DB.QueryRow(query, id).Scan(&user.ID, &user.FirstName, &user.LastName, &user.Email, &user.HashedPassword)
	if err != nil {
		return models.Doctor{}, err
	}

	return user, nil
}

func (r *Repository) GetDoctors() ([]models.Doctor, error) {
	query := `SELECT id, firstName, lastName, email, specialty, experience, available FROM doctors`

	rows, err := r.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var doctors []models.Doctor
	for rows.Next() {
		var doc models.Doctor
		err := rows.Scan(&doc.ID, &doc.FirstName, &doc.LastName, &doc.Email, &doc.Specialty, &doc.Experience, &doc.Available)
		if err != nil {
			return nil, err
		}
		doctors = append(doctors, doc)
	}
	return doctors, nil
}

// Appointment Related Methods
func (r *Repository) CreateAppointment(patientID int, doctorID int, startTime time.Time, endTime time.Time, apptType string) (int, error) {
	query := `
		INSERT INTO appointments (patient_id, doctor_id, start_time, end_time, appointment_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	var newID int
	err := r.DB.QueryRow(query, patientID, doctorID, startTime, endTime, apptType).Scan(&newID)
	if err != nil {
		return 0, err
	}
	return newID, nil
}

func (r *Repository) GetAppointmentsByDoctorID(doctorID int) ([]models.Appointment, error) {
	query := `
		SELECT 
			a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type,
			p.firstName, p.lastName
		FROM appointments a
		JOIN patients p ON a.patient_id = p.id
		WHERE a.doctor_id = $1
		ORDER BY a.start_time DESC`

	rows, err := r.DB.Query(query, doctorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appointments []models.Appointment
	for rows.Next() {
		var appt models.Appointment
		var patientFirstName, patientLastName string

		err := rows.Scan(
			&appt.ID, &appt.PatientID, &appt.DoctorID, &appt.StartTime, &appt.EndTime, &appt.Status, &appt.Type,
			&patientFirstName, &patientLastName,
		)
		if err != nil {
			return nil, err
		}

		appt.PatientName = patientFirstName + " " + patientLastName

		appointments = append(appointments, appt)
	}
	return appointments, nil
}

func (r *Repository) GetAppointmentsByPatientID(patientID int) ([]models.Appointment, error) {
	query := `
		SELECT 
			a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type,
			d.firstName, d.lastName, d.specialty
		FROM appointments a
		JOIN doctors d ON a.doctor_id = d.id
		WHERE a.patient_id = $1
		ORDER BY a.start_time DESC
	`

	rows, err := r.DB.Query(query, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appointments []models.Appointment
	for rows.Next() {
		var appt models.Appointment
		var docFirstName, docLastName, docSpecialty string

		err := rows.Scan(
			&appt.ID, &appt.PatientID, &appt.DoctorID, &appt.StartTime, &appt.EndTime, &appt.Status, &appt.Type,
			&docFirstName, &docLastName, &docSpecialty,
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

func (r *Repository) GetAppointmentsForPatient(doctorID int, patientID int) ([]models.Appointment, error) {
	query := `
		SELECT 
			a.id, a.patient_id, a.doctor_id, a.start_time, a.end_time, a.status, a.appointment_type,
			d.firstName, d.lastName, d.specialty
		FROM appointments a
		JOIN doctors d ON a.doctor_id = d.id
		WHERE a.patient_id = $1 AND a.doctor_id IN (
			SELECT doctor_id FROM appointments WHERE patient_id = $1 AND doctor_id = $2
		)
		ORDER BY a.start_time DESC
	`
	rows, err := r.DB.Query(query, patientID, doctorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appointments []models.Appointment
	for rows.Next() {
		var appt models.Appointment
		var docFirstName, docLastName, docSpecialty string

		err := rows.Scan(
			&appt.ID, &appt.PatientID, &appt.DoctorID, &appt.StartTime, &appt.EndTime, &appt.Status, &appt.Type,
			&docFirstName, &docLastName, &docSpecialty,
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

func (r *Repository) UpdateAppointmentAsCompleted(appointmentID int) error {
	query := `UPDATE appointments SET status = 'completed' WHERE id = $1`
	_, err := r.DB.Exec(query, appointmentID)
	return err
}

func (r *Repository) UpdateAppointmentAsCompletedForDoctor(appointmentID int, doctorID int) (bool, error) {
	query := `UPDATE appointments SET status = 'completed' WHERE id = $1 AND doctor_id = $2`
	result, err := r.DB.Exec(query, appointmentID, doctorID)
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
func (r *Repository) CreatePrescription(patientID int, doctorID int, medication, notes, fileName string) (int, error) {
	query := `
		INSERT INTO prescriptions (patient_id, doctor_id, medication, notes, file_name)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	var newID int
	err := r.DB.QueryRow(query, patientID, doctorID, medication, notes, fileName).Scan(&newID)
	if err != nil {
		return 0, err
	}
	return newID, nil
}

func (r *Repository) GetPrescriptionsByPatientID(patientID int) ([]models.Prescription, error) {
	query := `
		SELECT p.id, p.patient_id, p.doctor_id, p.medication, p.notes, p.file_name, p.created_at, d.firstName, d.lastName
		FROM prescriptions p
		JOIN doctors d ON p.doctor_id = d.id
		WHERE p.patient_id = $1
		ORDER BY p.created_at DESC
	`
	rows, err := r.DB.Query(query, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prescriptions []models.Prescription
	for rows.Next() {
		var pres models.Prescription
		var docFirstName, docLastName string
		err := rows.Scan(&pres.ID, &pres.PatientID, &pres.DoctorID, &pres.Medication, &pres.Notes, &pres.FileName, &pres.CreatedAt, &docFirstName, &docLastName)
		if err != nil {
			return nil, err
		}
		pres.DoctorName = docFirstName + " " + docLastName
		prescriptions = append(prescriptions, pres)
	}
	return prescriptions, nil
}

func (r *Repository) GetPrescriptionByFilename(patientID int, filename string) (models.Prescription, error) {
	query := `SELECT id FROM prescriptions WHERE patient_id = $1 AND file_name = $2`

	var pres models.Prescription
	err := r.DB.QueryRow(query, patientID, filename).Scan(&pres.ID)

	// This will correctly return sql.ErrNoRows if not found/not owned
	return pres, err
}

func (r *Repository) GetPrescriptionsForPatient(doctorID int, patientID int) ([]models.Prescription, error) {
	query := `
		SELECT 
			p.id, p.patient_id, p.doctor_id, p.medication, p.notes, p.file_name, p.created_at, 
			d.firstName, d.lastName
		FROM prescriptions p
		JOIN doctors d ON p.doctor_id = d.id
		WHERE p.patient_id = $1 AND p.patient_id IN (
			SELECT patient_id FROM appointments WHERE patient_id = $1 AND doctor_id = $2
		)
		ORDER BY p.created_at DESC
	`
	rows, err := r.DB.Query(query, patientID, doctorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prescriptions []models.Prescription
	for rows.Next() {
		var pres models.Prescription
		var docFirstName, docLastName string
		err := rows.Scan(&pres.ID, &pres.PatientID, &pres.DoctorID, &pres.Medication, &pres.Notes, &pres.FileName, &pres.CreatedAt, &docFirstName, &docLastName)
		if err != nil {
			return nil, err
		}
		pres.DoctorName = docFirstName + " " + docLastName
		prescriptions = append(prescriptions, pres)
	}
	return prescriptions, nil
}

func (r *Repository) GetPrescriptionByFilenameForDoctor(doctorID int, filename string) (models.Prescription, error) {
	query := `
		SELECT p.id 
		FROM prescriptions p
		JOIN appointments a ON p.patient_id = a.patient_id
		WHERE p.file_name = $1 AND a.doctor_id = $2
		LIMIT 1
	`
	var pres models.Prescription
	err := r.DB.QueryRow(query, filename, doctorID).Scan(&pres.ID)

	return pres, err
}
