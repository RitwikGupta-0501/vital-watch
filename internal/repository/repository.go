package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

// Repository defines all database operations needed by the application handlers
type Repository interface {
	CreatePatient(ctx context.Context, firstName, lastName, email, hashedPassword string) (uuid.UUID, error)
	GetPatientByEmail(ctx context.Context, email string) (models.Patient, error)
	GetPatientByID(ctx context.Context, id uuid.UUID) (models.Patient, error)
	GetPatientsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Patient, error)

	CreateDoctor(ctx context.Context, firstName, lastName, email, hashedPassword, specialty string, experience int) (uuid.UUID, error)
	GetDoctorByEmail(ctx context.Context, email string) (models.Doctor, error)
	GetDoctorByID(ctx context.Context, id uuid.UUID) (models.Doctor, error)
	GetDoctors(ctx context.Context, limit, offset int) ([]models.Doctor, error)

	CreateAppointment(ctx context.Context, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType string) (uuid.UUID, error)
	GetAppointmentsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetAppointmentsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetAppointmentsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	UpdateAppointmentAsCompletedForDoctor(ctx context.Context, appointmentID, doctorID uuid.UUID) (bool, error)

	CreatePrescription(ctx context.Context, patientID, doctorID uuid.UUID, medication, notes, fileName string) (uuid.UUID, error)
	GetPrescriptionsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionByFilename(ctx context.Context, patientID uuid.UUID, filename string) (models.Prescription, error)
	GetPrescriptionsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionByFilenameForDoctor(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error)
}
