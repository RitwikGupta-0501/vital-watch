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

	CreateAppointment(ctx context.Context, id, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType, meetingLink, meetingID string) (uuid.UUID, error)
	GetAppointmentByID(ctx context.Context, id uuid.UUID) (models.Appointment, error)
	GetAppointmentsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetAppointmentsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetAppointmentsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetDoctorAppointmentsInRange(ctx context.Context, doctorID uuid.UUID, startTime, endTime time.Time) ([]models.Appointment, error)
	UpdateAppointmentAsCompletedForDoctor(ctx context.Context, appointmentID, doctorID uuid.UUID) (bool, error)
	UpdateAppointmentMeetingRoom(ctx context.Context, apptID uuid.UUID, meetingLink, meetingID string) error

	// Phase 4: Doctor Working Schedules
	UpsertDoctorSchedule(ctx context.Context, schedule models.DoctorSchedule) (models.DoctorSchedule, error)
	UpsertDoctorSchedulesTx(ctx context.Context, doctorID uuid.UUID, schedules []models.DoctorSchedule) ([]models.DoctorSchedule, error)
	GetDoctorSchedules(ctx context.Context, doctorID uuid.UUID) ([]models.DoctorSchedule, error)
	GetDoctorScheduleByDay(ctx context.Context, doctorID uuid.UUID, dayOfWeek int) (models.DoctorSchedule, error)
	DeleteDoctorScheduleByDay(ctx context.Context, doctorID uuid.UUID, dayOfWeek int) error

	// Phase 4: Longitudinal Vitals
	CreatePatientVital(ctx context.Context, vital models.PatientVital) (uuid.UUID, error)
	GetPatientVitals(ctx context.Context, patientID uuid.UUID, startDate, endDate *time.Time, limit, offset int) ([]models.PatientVital, error)
	GetLatestPatientVital(ctx context.Context, patientID uuid.UUID) (models.PatientVital, error)

	// Phase 4: Medication Adherence & Schedules
	GetActivePrescriptionItemsForPatient(ctx context.Context, patientID uuid.UUID, targetDate time.Time) ([]models.PrescriptionItem, error)
	UpsertMedicationLog(ctx context.Context, log models.MedicationLog) (models.MedicationLog, error)
	GetMedicationLogsByDate(ctx context.Context, patientID uuid.UUID, date time.Time) ([]models.MedicationLog, error)
	VerifyPrescriptionItemOwnership(ctx context.Context, itemID, patientID uuid.UUID) (bool, error)
	HasDoctorPatientRelationship(ctx context.Context, doctorID, patientID uuid.UUID) (bool, error)

	// Prescriptions: Dual-Mode, Atomic Enqueue, and Review
	CreateUploadedPrescriptionWithJob(ctx context.Context, patientID, doctorID uuid.UUID, fileName, notes string, ocrEnabled bool) (uuid.UUID, string, error)
	CreateDigitalPrescription(ctx context.Context, patientID, doctorID uuid.UUID, notes string, items []models.PrescriptionItem) (uuid.UUID, error)
	UpdatePrescriptionOCRResults(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error
	VerifyPrescription(ctx context.Context, prescriptionID, doctorID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error)
	GetPrescriptionsPendingReview(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionByFilename(ctx context.Context, patientID uuid.UUID, filename string) (models.Prescription, error)
	GetPrescriptionsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, status string, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionByFilenameForDoctor(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error)
	GetPrescriptionByID(ctx context.Context, id uuid.UUID) (models.Prescription, error)
	UpdatePrescriptionFileName(ctx context.Context, prescriptionID uuid.UUID, fileName string) error

	// Phase 5: Refresh Tokens & Session Management
	CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error)
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (models.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID, replacedByTokenID *uuid.UUID) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error

	// Phase 5: HIPAA ePHI Audit Logging
	CreateAuditLog(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error)
	GetAuditLogsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.PhiAuditLog, error)
	GetAuditLogs(ctx context.Context, limit, offset int) ([]models.PhiAuditLog, error)
}
