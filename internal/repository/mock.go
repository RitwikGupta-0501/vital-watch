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
	CreateAppointmentFunc                     func(ctx context.Context, id, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType, meetingLink, meetingID string) (uuid.UUID, error)
	GetAppointmentByIDFunc                    func(ctx context.Context, id uuid.UUID) (models.Appointment, error)
	GetDoctorAppointmentsInRangeFunc          func(ctx context.Context, doctorID uuid.UUID, startTime, endTime time.Time) ([]models.Appointment, error)
	UpsertDoctorScheduleFunc                  func(ctx context.Context, schedule models.DoctorSchedule) (models.DoctorSchedule, error)
	GetDoctorSchedulesFunc                    func(ctx context.Context, doctorID uuid.UUID) ([]models.DoctorSchedule, error)
	GetDoctorScheduleByDayFunc                func(ctx context.Context, doctorID uuid.UUID, dayOfWeek int) (models.DoctorSchedule, error)
	UpsertDoctorSchedulesTxFunc               func(ctx context.Context, doctorID uuid.UUID, schedules []models.DoctorSchedule) ([]models.DoctorSchedule, error)
	DeleteDoctorScheduleByDayFunc             func(ctx context.Context, doctorID uuid.UUID, dayOfWeek int) error
	UpdateAppointmentMeetingRoomFunc          func(ctx context.Context, apptID uuid.UUID, meetingLink, meetingID string) error
	CreatePatientVitalFunc                    func(ctx context.Context, vital models.PatientVital) (uuid.UUID, error)
	GetPatientVitalsFunc                      func(ctx context.Context, patientID uuid.UUID, startDate, endDate *time.Time, limit, offset int) ([]models.PatientVital, error)
	GetLatestPatientVitalFunc                 func(ctx context.Context, patientID uuid.UUID) (models.PatientVital, error)
	GetActivePrescriptionItemsForPatientFunc  func(ctx context.Context, patientID uuid.UUID, targetDate time.Time) ([]models.PrescriptionItem, error)
	UpsertMedicationLogFunc                   func(ctx context.Context, log models.MedicationLog) (models.MedicationLog, error)
	GetMedicationLogsByDateFunc               func(ctx context.Context, patientID uuid.UUID, date time.Time) ([]models.MedicationLog, error)
	VerifyPrescriptionItemOwnershipFunc       func(ctx context.Context, itemID, patientID uuid.UUID) (bool, error)
	HasDoctorPatientRelationshipFunc          func(ctx context.Context, doctorID, patientID uuid.UUID) (bool, error)
	GetAppointmentsByDoctorIDFunc             func(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetAppointmentsByPatientIDFunc            func(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	GetAppointmentsForPatientFunc             func(ctx context.Context, doctorID, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error)
	UpdateAppointmentAsCompletedForDoctorFunc func(ctx context.Context, appointmentID, doctorID uuid.UUID) (bool, error)

	CreateUploadedPrescriptionWithJobFunc  func(ctx context.Context, patientID, doctorID uuid.UUID, fileName, notes string, ocrEnabled bool) (uuid.UUID, string, error)
	CreateDigitalPrescriptionFunc          func(ctx context.Context, patientID, doctorID uuid.UUID, notes string, items []models.PrescriptionItem) (uuid.UUID, error)
	UpdatePrescriptionOCRResultsFunc       func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error
	VerifyPrescriptionFunc                 func(ctx context.Context, prescriptionID, doctorID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error)
	GetPrescriptionsPendingReviewFunc      func(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionsByPatientIDFunc        func(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionByFilenameFunc          func(ctx context.Context, patientID uuid.UUID, filename string) (models.Prescription, error)
	GetPrescriptionsForPatientFunc         func(ctx context.Context, doctorID, patientID uuid.UUID, status string, limit, offset int) ([]models.Prescription, error)
	GetPrescriptionByFilenameForDoctorFunc func(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error)
	GetPrescriptionByIDFunc                func(ctx context.Context, id uuid.UUID) (models.Prescription, error)
	UpdatePrescriptionFileNameFunc         func(ctx context.Context, prescriptionID uuid.UUID, fileName string) error

	CreateRefreshTokenFunc          func(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error)
	GetRefreshTokenByHashFunc       func(ctx context.Context, tokenHash string) (models.RefreshToken, error)
	RevokeRefreshTokenFunc          func(ctx context.Context, id uuid.UUID, replacedByTokenID *uuid.UUID) error
	RevokeAllUserRefreshTokensFunc  func(ctx context.Context, userID uuid.UUID) error
	CreateAuditLogFunc              func(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error)
	GetAuditLogsByPatientIDFunc     func(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.PhiAuditLog, error)
	GetAuditLogsFunc                func(ctx context.Context, limit, offset int) ([]models.PhiAuditLog, error)
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

func (m *MockRepository) CreateAppointment(ctx context.Context, id, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType, meetingLink, meetingID string) (uuid.UUID, error) {
	if m.CreateAppointmentFunc != nil {
		return m.CreateAppointmentFunc(ctx, id, patientID, doctorID, startTime, endTime, apptType, meetingLink, meetingID)
	}
	if id != uuid.Nil {
		return id, nil
	}
	return uuid.New(), nil
}

func (m *MockRepository) GetAppointmentByID(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
	if m.GetAppointmentByIDFunc != nil {
		return m.GetAppointmentByIDFunc(ctx, id)
	}
	return models.Appointment{ID: id}, nil
}

func (m *MockRepository) GetDoctorAppointmentsInRange(ctx context.Context, doctorID uuid.UUID, startTime, endTime time.Time) ([]models.Appointment, error) {
	if m.GetDoctorAppointmentsInRangeFunc != nil {
		return m.GetDoctorAppointmentsInRangeFunc(ctx, doctorID, startTime, endTime)
	}
	return []models.Appointment{}, nil
}

func (m *MockRepository) UpsertDoctorSchedule(ctx context.Context, schedule models.DoctorSchedule) (models.DoctorSchedule, error) {
	if m.UpsertDoctorScheduleFunc != nil {
		return m.UpsertDoctorScheduleFunc(ctx, schedule)
	}
	return schedule, nil
}

func (m *MockRepository) GetDoctorSchedules(ctx context.Context, doctorID uuid.UUID) ([]models.DoctorSchedule, error) {
	if m.GetDoctorSchedulesFunc != nil {
		return m.GetDoctorSchedulesFunc(ctx, doctorID)
	}
	if m.GetDoctorScheduleByDayFunc != nil {
		var list []models.DoctorSchedule
		for day := 0; day <= 6; day++ {
			s, err := m.GetDoctorScheduleByDayFunc(ctx, doctorID, day)
			if err == nil && (s.StartTime != "" || s.IsActive) {
				list = append(list, s)
			}
		}
		return list, nil
	}
	return []models.DoctorSchedule{}, nil
}

func (m *MockRepository) GetDoctorScheduleByDay(ctx context.Context, doctorID uuid.UUID, dayOfWeek int) (models.DoctorSchedule, error) {
	if m.GetDoctorScheduleByDayFunc != nil {
		return m.GetDoctorScheduleByDayFunc(ctx, doctorID, dayOfWeek)
	}
	return models.DoctorSchedule{DoctorID: doctorID, DayOfWeek: dayOfWeek, IsActive: true}, nil
}

func (m *MockRepository) CreatePatientVital(ctx context.Context, vital models.PatientVital) (uuid.UUID, error) {
	if m.CreatePatientVitalFunc != nil {
		return m.CreatePatientVitalFunc(ctx, vital)
	}
	return uuid.New(), nil
}

func (m *MockRepository) GetPatientVitals(ctx context.Context, patientID uuid.UUID, startDate, endDate *time.Time, limit, offset int) ([]models.PatientVital, error) {
	if m.GetPatientVitalsFunc != nil {
		return m.GetPatientVitalsFunc(ctx, patientID, startDate, endDate, limit, offset)
	}
	return []models.PatientVital{}, nil
}

func (m *MockRepository) GetLatestPatientVital(ctx context.Context, patientID uuid.UUID) (models.PatientVital, error) {
	if m.GetLatestPatientVitalFunc != nil {
		return m.GetLatestPatientVitalFunc(ctx, patientID)
	}
	return models.PatientVital{PatientID: patientID}, nil
}

func (m *MockRepository) GetActivePrescriptionItemsForPatient(ctx context.Context, patientID uuid.UUID, targetDate time.Time) ([]models.PrescriptionItem, error) {
	if m.GetActivePrescriptionItemsForPatientFunc != nil {
		return m.GetActivePrescriptionItemsForPatientFunc(ctx, patientID, targetDate)
	}
	return []models.PrescriptionItem{}, nil
}

func (m *MockRepository) UpsertMedicationLog(ctx context.Context, log models.MedicationLog) (models.MedicationLog, error) {
	if m.UpsertMedicationLogFunc != nil {
		return m.UpsertMedicationLogFunc(ctx, log)
	}
	return log, nil
}

func (m *MockRepository) GetMedicationLogsByDate(ctx context.Context, patientID uuid.UUID, date time.Time) ([]models.MedicationLog, error) {
	if m.GetMedicationLogsByDateFunc != nil {
		return m.GetMedicationLogsByDateFunc(ctx, patientID, date)
	}
	return []models.MedicationLog{}, nil
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

func (m *MockRepository) CreateUploadedPrescriptionWithJob(ctx context.Context, patientID, doctorID uuid.UUID, fileName, notes string, ocrEnabled bool) (uuid.UUID, string, error) {
	if m.CreateUploadedPrescriptionWithJobFunc != nil {
		return m.CreateUploadedPrescriptionWithJobFunc(ctx, patientID, doctorID, fileName, notes, ocrEnabled)
	}
	status := "pending_ocr"
	if !ocrEnabled {
		status = "needs_review"
	}
	return uuid.New(), status, nil
}

func (m *MockRepository) CreateDigitalPrescription(ctx context.Context, patientID, doctorID uuid.UUID, notes string, items []models.PrescriptionItem) (uuid.UUID, error) {
	if m.CreateDigitalPrescriptionFunc != nil {
		return m.CreateDigitalPrescriptionFunc(ctx, patientID, doctorID, notes, items)
	}
	return uuid.New(), nil
}

func (m *MockRepository) UpdatePrescriptionOCRResults(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
	if m.UpdatePrescriptionOCRResultsFunc != nil {
		return m.UpdatePrescriptionOCRResultsFunc(ctx, prescriptionID, status, notes, ocrProvider, items)
	}
	return nil
}

func (m *MockRepository) VerifyPrescription(ctx context.Context, prescriptionID, doctorID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
	if m.VerifyPrescriptionFunc != nil {
		return m.VerifyPrescriptionFunc(ctx, prescriptionID, doctorID, status, notes, items)
	}
	return true, nil
}

func (m *MockRepository) GetPrescriptionsPendingReview(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
	if m.GetPrescriptionsPendingReviewFunc != nil {
		return m.GetPrescriptionsPendingReviewFunc(ctx, doctorID, limit, offset)
	}
	return []models.Prescription{}, nil
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
	return models.Prescription{ID: uuid.New(), FileName: filename}, nil
}

func (m *MockRepository) GetPrescriptionsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, status string, limit, offset int) ([]models.Prescription, error) {
	if m.GetPrescriptionsForPatientFunc != nil {
		return m.GetPrescriptionsForPatientFunc(ctx, doctorID, patientID, status, limit, offset)
	}
	return []models.Prescription{}, nil
}

func (m *MockRepository) GetPrescriptionByFilenameForDoctor(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error) {
	if m.GetPrescriptionByFilenameForDoctorFunc != nil {
		return m.GetPrescriptionByFilenameForDoctorFunc(ctx, doctorID, filename)
	}
	return models.Prescription{ID: uuid.New(), FileName: filename}, nil
}

func (m *MockRepository) GetPrescriptionByID(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
	if m.GetPrescriptionByIDFunc != nil {
		return m.GetPrescriptionByIDFunc(ctx, id)
	}
	return models.Prescription{ID: id}, nil
}

func (m *MockRepository) UpdatePrescriptionFileName(ctx context.Context, prescriptionID uuid.UUID, fileName string) error {
	if m.UpdatePrescriptionFileNameFunc != nil {
		return m.UpdatePrescriptionFileNameFunc(ctx, prescriptionID, fileName)
	}
	return nil
}

func (m *MockRepository) VerifyPrescriptionItemOwnership(ctx context.Context, itemID, patientID uuid.UUID) (bool, error) {
	if m.VerifyPrescriptionItemOwnershipFunc != nil {
		return m.VerifyPrescriptionItemOwnershipFunc(ctx, itemID, patientID)
	}
	return true, nil
}

func (m *MockRepository) HasDoctorPatientRelationship(ctx context.Context, doctorID, patientID uuid.UUID) (bool, error) {
	if m.HasDoctorPatientRelationshipFunc != nil {
		return m.HasDoctorPatientRelationshipFunc(ctx, doctorID, patientID)
	}
	return true, nil
}

func (m *MockRepository) UpsertDoctorSchedulesTx(ctx context.Context, doctorID uuid.UUID, schedules []models.DoctorSchedule) ([]models.DoctorSchedule, error) {
	if m.UpsertDoctorSchedulesTxFunc != nil {
		return m.UpsertDoctorSchedulesTxFunc(ctx, doctorID, schedules)
	}
	return schedules, nil
}

func (m *MockRepository) DeleteDoctorScheduleByDay(ctx context.Context, doctorID uuid.UUID, dayOfWeek int) error {
	if m.DeleteDoctorScheduleByDayFunc != nil {
		return m.DeleteDoctorScheduleByDayFunc(ctx, doctorID, dayOfWeek)
	}
	return nil
}

func (m *MockRepository) UpdateAppointmentMeetingRoom(ctx context.Context, apptID uuid.UUID, meetingLink, meetingID string) error {
	if m.UpdateAppointmentMeetingRoomFunc != nil {
		return m.UpdateAppointmentMeetingRoomFunc(ctx, apptID, meetingLink, meetingID)
	}
	return nil
}

func (m *MockRepository) CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error) {
	if m.CreateRefreshTokenFunc != nil {
		return m.CreateRefreshTokenFunc(ctx, userID, tokenHash, expiresAt)
	}
	return models.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}, nil
}

func (m *MockRepository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (models.RefreshToken, error) {
	if m.GetRefreshTokenByHashFunc != nil {
		return m.GetRefreshTokenByHashFunc(ctx, tokenHash)
	}
	return models.RefreshToken{}, sql.ErrNoRows
}

func (m *MockRepository) RevokeRefreshToken(ctx context.Context, id uuid.UUID, replacedByTokenID *uuid.UUID) error {
	if m.RevokeRefreshTokenFunc != nil {
		return m.RevokeRefreshTokenFunc(ctx, id, replacedByTokenID)
	}
	return nil
}

func (m *MockRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	if m.RevokeAllUserRefreshTokensFunc != nil {
		return m.RevokeAllUserRefreshTokensFunc(ctx, userID)
	}
	return nil
}

func (m *MockRepository) CreateAuditLog(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
	if m.CreateAuditLogFunc != nil {
		return m.CreateAuditLogFunc(ctx, log)
	}
	return uuid.New(), nil
}

func (m *MockRepository) GetAuditLogsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.PhiAuditLog, error) {
	if m.GetAuditLogsByPatientIDFunc != nil {
		return m.GetAuditLogsByPatientIDFunc(ctx, patientID, limit, offset)
	}
	return []models.PhiAuditLog{}, nil
}

func (m *MockRepository) GetAuditLogs(ctx context.Context, limit, offset int) ([]models.PhiAuditLog, error) {
	if m.GetAuditLogsFunc != nil {
		return m.GetAuditLogsFunc(ctx, limit, offset)
	}
	return []models.PhiAuditLog{}, nil
}

