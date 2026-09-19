package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

var _ Repository = (*MockRepository)(nil)

// MockRepository provides a thread-safe, customizable mock for unit testing
type MockRepository struct {
	CreatePatientFunc                         func(ctx context.Context, firstName, lastName, email, hashedPassword string) (uuid.UUID, error)
	GetPatientByEmailFunc                     func(ctx context.Context, email string) (models.Patient, error)
	GetPatientByIDFunc                        func(ctx context.Context, id uuid.UUID) (models.Patient, error)
	GetPatientsByDoctorIDFunc                 func(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Patient, error)
	CreateDoctorFunc                          func(ctx context.Context, firstName, lastName, email, hashedPassword, specialty string, experience int) (uuid.UUID, error)
	GetDoctorByEmailFunc                      func(ctx context.Context, email string) (models.Doctor, error)
	GetDoctorByIDFunc                         func(ctx context.Context, id uuid.UUID) (models.Doctor, error)
	GetDoctorsFunc                            func(ctx context.Context, limit, offset int) ([]models.Doctor, error)
	CreateAppointmentFunc                     func(ctx context.Context, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType string) (uuid.UUID, error)
	GetAppointmentsByDoctorIDFunc             func(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetAppointmentsByPatientIDFunc            func(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetAppointmentsForPatientFunc             func(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	UpdateAppointmentAsCompletedForDoctorFunc func(ctx context.Context, appointmentID, doctorID uuid.UUID) (bool, error)
	CreatePrescriptionFunc                    func(ctx context.Context, patientID, doctorID uuid.UUID, medication, notes, fileName string) (uuid.UUID, error)
	GetPrescriptionsByPatientIDFunc           func(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionByFilenameFunc             func(ctx context.Context, patientID uuid.UUID, filename string) (models.Prescription, error)
	GetPrescriptionsForPatientFunc            func(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionByFilenameForDoctorFunc    func(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error)
}

func (m *MockRepository) CreatePatient(ctx context.Context, firstName, lastName, email, hashedPassword string) (uuid.UUID, error) {
	if m.CreatePatientFunc != nil {
		return m.CreatePatientFunc(ctx, firstName, lastName, email, hashedPassword)
	}
	return uuid.New(), nil
}

func (m *MockRepository) GetPatientByEmail(ctx context.Context, email string) (models.Patient, error) {
	if m.GetPatientByEmailFunc != nil {
		return m.GetPatientByEmailFunc(ctx, email)
	}
	return models.Patient{}, sql.ErrNoRows
}

func (m *MockRepository) GetPatientByID(ctx context.Context, id uuid.UUID) (models.Patient, error) {
	if m.GetPatientByIDFunc != nil {
		return m.GetPatientByIDFunc(ctx, id)
	}
	return models.Patient{}, sql.ErrNoRows
}

func (m *MockRepository) GetPatientsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Patient, error) {
	if m.GetPatientsByDoctorIDFunc != nil {
		return m.GetPatientsByDoctorIDFunc(ctx, doctorID, limit, offset)
	}
	return []models.Patient{}, nil
}

func (m *MockRepository) CreateDoctor(ctx context.Context, firstName, lastName, email, hashedPassword, specialty string, experience int) (uuid.UUID, error) {
	if m.CreateDoctorFunc != nil {
		return m.CreateDoctorFunc(ctx, firstName, lastName, email, hashedPassword, specialty, experience)
	}
	return uuid.New(), nil
}

func (m *MockRepository) GetDoctorByEmail(ctx context.Context, email string) (models.Doctor, error) {
	if m.GetDoctorByEmailFunc != nil {
		return m.GetDoctorByEmailFunc(ctx, email)
	}
	return models.Doctor{}, sql.ErrNoRows
}

func (m *MockRepository) GetDoctorByID(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
	if m.GetDoctorByIDFunc != nil {
		return m.GetDoctorByIDFunc(ctx, id)
	}
	return models.Doctor{}, sql.ErrNoRows
}

func (m *MockRepository) GetDoctors(ctx context.Context, limit, offset int) ([]models.Doctor, error) {
	if m.GetDoctorsFunc != nil {
		return m.GetDoctorsFunc(ctx, limit, offset)
	}
	return []models.Doctor{}, nil
}

func (m *MockRepository) CreateAppointment(ctx context.Context, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType string) (uuid.UUID, error) {
	if m.CreateAppointmentFunc != nil {
		return m.CreateAppointmentFunc(ctx, patientID, doctorID, startTime, endTime, apptType)
	}
	return uuid.New(), nil
}

func (m *MockRepository) GetAppointmentsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
	if m.GetAppointmentsByDoctorIDFunc != nil {
		return m.GetAppointmentsByDoctorIDFunc(ctx, doctorID, limit, offset)
	}
	return []models.Appointment{}, nil
}

func (m *MockRepository) GetAppointmentsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
	if m.GetAppointmentsByPatientIDFunc != nil {
		return m.GetAppointmentsByPatientIDFunc(ctx, patientID, limit, offset)
	}
	return []models.Appointment{}, nil
}

func (m *MockRepository) GetAppointmentsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
	if m.GetAppointmentsForPatientFunc != nil {
		return m.GetAppointmentsForPatientFunc(ctx, doctorID, patientID, limit, offset)
	}
	return []models.Appointment{}, nil
}

func (m *MockRepository) UpdateAppointmentAsCompletedForDoctor(ctx context.Context, appointmentID, doctorID uuid.UUID) (bool, error) {
	if m.UpdateAppointmentAsCompletedForDoctorFunc != nil {
		return m.UpdateAppointmentAsCompletedForDoctorFunc(ctx, appointmentID, doctorID)
	}
	return true, nil
}

func (m *MockRepository) CreatePrescription(ctx context.Context, patientID, doctorID uuid.UUID, medication, notes, fileName string) (uuid.UUID, error) {
	if m.CreatePrescriptionFunc != nil {
		return m.CreatePrescriptionFunc(ctx, patientID, doctorID, medication, notes, fileName)
	}
	return uuid.New(), nil
}

func (m *MockRepository) GetPrescriptionsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
	if m.GetPrescriptionsByPatientIDFunc != nil {
		return m.GetPrescriptionsByPatientIDFunc(ctx, patientID, limit, offset)
	}
	return []models.Prescription{}, nil
}

func (m *MockRepository) GetPrescriptionByFilename(ctx context.Context, patientID uuid.UUID, filename string) (models.Prescription, error) {
	if m.GetPrescriptionByFilenameFunc != nil {
		return m.GetPrescriptionByFilenameFunc(ctx, patientID, filename)
	}
	return models.Prescription{}, sql.ErrNoRows
}

func (m *MockRepository) GetPrescriptionsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
	if m.GetPrescriptionsForPatientFunc != nil {
		return m.GetPrescriptionsForPatientFunc(ctx, doctorID, patientID, limit, offset)
	}
	return []models.Prescription{}, nil
}

func (m *MockRepository) GetPrescriptionByFilenameForDoctor(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error) {
	if m.GetPrescriptionByFilenameForDoctorFunc != nil {
		return m.GetPrescriptionByFilenameForDoctorFunc(ctx, doctorID, filename)
	}
	return models.Prescription{}, sql.ErrNoRows
}
