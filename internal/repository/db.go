package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository/dbgen"
)

// DBRepository is the concrete implementation of the Repository interface wrapping sqlc generated queries
type DBRepository struct {
	DB      *sql.DB
	queries *dbgen.Queries
}

var _ Repository = (*DBRepository)(nil)

// New creates a new DBRepository instance
func New(db *sql.DB) *DBRepository {
	return &DBRepository{
		DB:      db,
		queries: dbgen.New(db),
	}
}

func clampPagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// Patient Related Methods
func (r *DBRepository) CreatePatient(ctx context.Context, firstName, lastName, email, hashedPassword string) (uuid.UUID, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()

	qtx := r.queries.WithTx(tx)

	newID, err := qtx.CreatePatientUser(ctx, dbgen.CreatePatientUserParams{
		Email:          email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		return uuid.Nil, err
	}

	err = qtx.CreatePatientProfile(ctx, dbgen.CreatePatientProfileParams{
		UserID:    newID,
		FirstName: firstName,
		LastName:  lastName,
	})
	if err != nil {
		return uuid.Nil, err
	}

	if err = tx.Commit(); err != nil {
		return uuid.Nil, err
	}

	return newID, nil
}

func (r *DBRepository) GetPatientByEmail(ctx context.Context, email string) (models.Patient, error) {
	row, err := r.queries.GetPatientByEmail(ctx, email)
	if err != nil {
		return models.Patient{}, err
	}
	return models.Patient{
		ID:             row.ID,
		Email:          row.Email,
		FirstName:      row.FirstName,
		LastName:       row.LastName,
		HashedPassword: row.HashedPassword,
		Role:           "patient",
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetPatientByID(ctx context.Context, id uuid.UUID) (models.Patient, error) {
	row, err := r.queries.GetPatientByID(ctx, id)
	if err != nil {
		return models.Patient{}, err
	}
	return models.Patient{
		ID:        row.ID,
		Email:     row.Email,
		FirstName: row.FirstName,
		LastName:  row.LastName,
		Role:      "patient",
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetPatientsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Patient, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetPatientsByDoctorID(ctx, dbgen.GetPatientsByDoctorIDParams{
		DoctorID: doctorID,
		Limit:    int32(lim),
		Offset:   int32(off),
	})
	if err != nil {
		return nil, err
	}

	patients := make([]models.Patient, 0, len(rows))
	for _, row := range rows {
		patients = append(patients, models.Patient{
			ID:        row.ID,
			Email:     row.Email,
			FirstName: row.FirstName,
			LastName:  row.LastName,
			Role:      "patient",
			CreatedAt: row.CreatedAt.Time,
		})
	}
	return patients, nil
}

// Doctor Related Methods
func (r *DBRepository) CreateDoctor(ctx context.Context, firstName, lastName, email, hashedPassword, specialty string, experience int) (uuid.UUID, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()

	qtx := r.queries.WithTx(tx)

	newID, err := qtx.CreateDoctorUser(ctx, dbgen.CreateDoctorUserParams{
		Email:          email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		return uuid.Nil, err
	}

	err = qtx.CreateDoctorProfile(ctx, dbgen.CreateDoctorProfileParams{
		UserID:          newID,
		FirstName:       firstName,
		LastName:        lastName,
		Specialty:       sql.NullString{String: specialty, Valid: specialty != ""},
		ExperienceYears: sql.NullInt32{Int32: int32(experience), Valid: true},
	})
	if err != nil {
		return uuid.Nil, err
	}

	if err = tx.Commit(); err != nil {
		return uuid.Nil, err
	}

	return newID, nil
}

func (r *DBRepository) GetDoctorByEmail(ctx context.Context, email string) (models.Doctor, error) {
	row, err := r.queries.GetDoctorByEmail(ctx, email)
	if err != nil {
		return models.Doctor{}, err
	}
	return models.Doctor{
		ID:             row.ID,
		Email:          row.Email,
		FirstName:      row.FirstName,
		LastName:       row.LastName,
		Specialty:      row.Specialty.String,
		Experience:     int(row.ExperienceYears.Int32),
		Available:      row.Available.Bool,
		HashedPassword: row.HashedPassword,
		Role:           "doctor",
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetDoctorByID(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
	row, err := r.queries.GetDoctorByID(ctx, id)
	if err != nil {
		return models.Doctor{}, err
	}
	return models.Doctor{
		ID:         row.ID,
		Email:      row.Email,
		FirstName:  row.FirstName,
		LastName:   row.LastName,
		Specialty:  row.Specialty.String,
		Experience: int(row.ExperienceYears.Int32),
		Available:  row.Available.Bool,
		Role:       "doctor",
		CreatedAt:  row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetDoctors(ctx context.Context, limit, offset int) ([]models.Doctor, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetDoctors(ctx, dbgen.GetDoctorsParams{
		Limit:  int32(lim),
		Offset: int32(off),
	})
	if err != nil {
		return nil, err
	}

	doctors := make([]models.Doctor, 0, len(rows))
	for _, row := range rows {
		doctors = append(doctors, models.Doctor{
			ID:         row.ID,
			Email:      row.Email,
			FirstName:  row.FirstName,
			LastName:   row.LastName,
			Specialty:  row.Specialty.String,
			Experience: int(row.ExperienceYears.Int32),
			Available:  row.Available.Bool,
			Role:       "doctor",
			CreatedAt:  row.CreatedAt.Time,
		})
	}
	return doctors, nil
}

// Appointment Related Methods
func (r *DBRepository) CreateAppointment(ctx context.Context, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType string) (uuid.UUID, error) {
	return r.queries.CreateAppointment(ctx, dbgen.CreateAppointmentParams{
		PatientID:       patientID,
		DoctorID:        doctorID,
		StartTime:       startTime,
		EndTime:         endTime,
		AppointmentType: sql.NullString{String: apptType, Valid: apptType != ""},
	})
}

func (r *DBRepository) GetAppointmentsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetAppointmentsByDoctorID(ctx, dbgen.GetAppointmentsByDoctorIDParams{
		DoctorID: doctorID,
		Limit:    int32(lim),
		Offset:   int32(off),
	})
	if err != nil {
		return nil, err
	}

	appointments := make([]models.Appointment, 0, len(rows))
	for _, row := range rows {
		appointments = append(appointments, models.Appointment{
			ID:          row.ID,
			PatientID:   row.PatientID,
			DoctorID:    row.DoctorID,
			StartTime:   row.StartTime,
			EndTime:     row.EndTime,
			Status:      row.Status.String,
			Type:        row.AppointmentType.String,
			PatientName: row.FirstName + " " + row.LastName,
			CreatedAt:   row.CreatedAt.Time,
		})
	}
	return appointments, nil
}

func (r *DBRepository) GetAppointmentsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetAppointmentsByPatientID(ctx, dbgen.GetAppointmentsByPatientIDParams{
		PatientID: patientID,
		Limit:     int32(lim),
		Offset:    int32(off),
	})
	if err != nil {
		return nil, err
	}

	appointments := make([]models.Appointment, 0, len(rows))
	for _, row := range rows {
		appointments = append(appointments, models.Appointment{
			ID:              row.ID,
			PatientID:       row.PatientID,
			DoctorID:        row.DoctorID,
			StartTime:       row.StartTime,
			EndTime:         row.EndTime,
			Status:          row.Status.String,
			Type:            row.AppointmentType.String,
			DoctorName:      row.FirstName + " " + row.LastName,
			DoctorSpecialty: row.Specialty.String,
			CreatedAt:       row.CreatedAt.Time,
		})
	}
	return appointments, nil
}

func (r *DBRepository) GetAppointmentsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetAppointmentsForPatient(ctx, dbgen.GetAppointmentsForPatientParams{
		PatientID: patientID,
		DoctorID:  doctorID,
		Limit:     int32(lim),
		Offset:    int32(off),
	})
	if err != nil {
		return nil, err
	}

	appointments := make([]models.Appointment, 0, len(rows))
	for _, row := range rows {
		appointments = append(appointments, models.Appointment{
			ID:              row.ID,
			PatientID:       row.PatientID,
			DoctorID:        row.DoctorID,
			StartTime:       row.StartTime,
			EndTime:         row.EndTime,
			Status:          row.Status.String,
			Type:            row.AppointmentType.String,
			DoctorName:      row.FirstName + " " + row.LastName,
			DoctorSpecialty: row.Specialty.String,
			CreatedAt:       row.CreatedAt.Time,
		})
	}
	return appointments, nil
}

func (r *DBRepository) UpdateAppointmentAsCompletedForDoctor(ctx context.Context, appointmentID, doctorID uuid.UUID) (bool, error) {
	rowsAffected, err := r.queries.UpdateAppointmentAsCompletedForDoctor(ctx, dbgen.UpdateAppointmentAsCompletedForDoctorParams{
		ID:       appointmentID,
		DoctorID: doctorID,
	})
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

// Prescription Related Methods
func (r *DBRepository) CreatePrescription(ctx context.Context, patientID, doctorID uuid.UUID, medication, notes, fileName string) (uuid.UUID, error) {
	return r.queries.CreatePrescription(ctx, dbgen.CreatePrescriptionParams{
		PatientID:  patientID,
		DoctorID:   doctorID,
		Medication: medication,
		Notes:      sql.NullString{String: notes, Valid: notes != ""},
		FileName:   fileName,
	})
}

func (r *DBRepository) GetPrescriptionsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetPrescriptionsByPatientID(ctx, dbgen.GetPrescriptionsByPatientIDParams{
		PatientID: patientID,
		Limit:     int32(lim),
		Offset:    int32(off),
	})
	if err != nil {
		return nil, err
	}

	prescriptions := make([]models.Prescription, 0, len(rows))
	for _, row := range rows {
		prescriptions = append(prescriptions, models.Prescription{
			ID:         row.ID,
			PatientID:  row.PatientID,
			DoctorID:   row.DoctorID,
			Medication: row.Medication,
			Notes:      row.Notes.String,
			FileName:   row.FileName,
			DoctorName: row.FirstName + " " + row.LastName,
			CreatedAt:  row.CreatedAt.Time,
		})
	}
	return prescriptions, nil
}

func (r *DBRepository) GetPrescriptionByFilename(ctx context.Context, patientID uuid.UUID, filename string) (models.Prescription, error) {
	id, err := r.queries.GetPrescriptionByFilename(ctx, dbgen.GetPrescriptionByFilenameParams{
		PatientID: patientID,
		FileName:  filename,
	})
	if err != nil {
		return models.Prescription{}, err
	}
	return models.Prescription{ID: id}, nil
}

func (r *DBRepository) GetPrescriptionsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetPrescriptionsForPatient(ctx, dbgen.GetPrescriptionsForPatientParams{
		PatientID: patientID,
		DoctorID:  doctorID,
		Limit:     int32(lim),
		Offset:    int32(off),
	})
	if err != nil {
		return nil, err
	}

	prescriptions := make([]models.Prescription, 0, len(rows))
	for _, row := range rows {
		prescriptions = append(prescriptions, models.Prescription{
			ID:         row.ID,
			PatientID:  row.PatientID,
			DoctorID:   row.DoctorID,
			Medication: row.Medication,
			Notes:      row.Notes.String,
			FileName:   row.FileName,
			DoctorName: row.FirstName + " " + row.LastName,
			CreatedAt:  row.CreatedAt.Time,
		})
	}
	return prescriptions, nil
}

func (r *DBRepository) GetPrescriptionByFilenameForDoctor(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error) {
	id, err := r.queries.GetPrescriptionByFilenameForDoctor(ctx, dbgen.GetPrescriptionByFilenameForDoctorParams{
		FileName: filename,
		DoctorID: doctorID,
	})
	if err != nil {
		return models.Prescription{}, err
	}
	return models.Prescription{ID: id}, nil
}
