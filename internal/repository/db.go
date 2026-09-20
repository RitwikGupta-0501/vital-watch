package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
		"github.com/RitwikGupta-0501/vital-watch/internal/repository/dbgen"
)

// DBRepository is the concrete implementation of the Repository interface wrapping sqlc generated queries
type DBRepository struct {
	pool        *pgxpool.Pool
	queries     *dbgen.Queries
	riverClient *river.Client[pgx.Tx]
}

var _ Repository = (*DBRepository)(nil)

// New creates a new DBRepository instance
func New(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *DBRepository {
	return &DBRepository{
		pool:        pool,
		queries:     dbgen.New(pool),
		riverClient: riverClient,
	}
}

// SetRiverClient attaches a river client after initialization
func (r *DBRepository) SetRiverClient(riverClient *river.Client[pgx.Tx]) {
	r.riverClient = riverClient
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

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

	if err = tx.Commit(ctx); err != nil {
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

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
		Specialty:       pgtype.Text{String: specialty, Valid: specialty != ""},
		ExperienceYears: pgtype.Int4{Int32: int32(experience), Valid: true},
	})
	if err != nil {
		return uuid.Nil, err
	}

	if err = tx.Commit(ctx); err != nil {
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
		StartTime:       pgtype.Timestamptz{Time: startTime, Valid: true},
		EndTime:         pgtype.Timestamptz{Time: endTime, Valid: true},
		AppointmentType: pgtype.Text{String: apptType, Valid: apptType != ""},
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
			StartTime:   row.StartTime.Time,
			EndTime:     row.EndTime.Time,
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
			StartTime:       row.StartTime.Time,
			EndTime:         row.EndTime.Time,
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
			StartTime:       row.StartTime.Time,
			EndTime:         row.EndTime.Time,
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

// Prescription Methods: Dual-Mode, Atomic Enqueue, and Review

func (r *DBRepository) CreateUploadedPrescriptionWithJob(ctx context.Context, patientID, doctorID uuid.UUID, fileName, notes string, ocrEnabled bool) (uuid.UUID, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", err
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	status := "pending_ocr"
	if !ocrEnabled || r.riverClient == nil {
		status = "needs_review"
		if notes == "" {
			notes = "[OCR disabled: manual clinician entry required]"
		} else {
			notes = notes + " [OCR disabled: manual clinician entry required]"
		}
	}

	newID, err := qtx.CreatePrescription(ctx, dbgen.CreatePrescriptionParams{
		PatientID: patientID,
		DoctorID:  doctorID,
		Status:    status,
		FileName:  pgtype.Text{String: fileName, Valid: fileName != ""},
		Notes:     pgtype.Text{String: notes, Valid: notes != ""},
	})
	if err != nil {
		return uuid.Nil, "", err
	}

	if status == "pending_ocr" && r.riverClient != nil {
		_, err = r.riverClient.InsertTx(ctx, tx, models.PrescriptionOCRArgs{
			PrescriptionID: newID,
			StorageKey:     fileName,
		}, nil)
		if err != nil {
			return uuid.Nil, "", fmt.Errorf("failed to enqueue prescription OCR job atomically: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, "", err
	}

	return newID, status, nil
}

func (r *DBRepository) CreateDigitalPrescription(ctx context.Context, patientID, doctorID uuid.UUID, notes string, items []models.PrescriptionItem) (uuid.UUID, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	newID, err := qtx.CreateDigitalPrescription(ctx, dbgen.CreateDigitalPrescriptionParams{
		PatientID: patientID,
		DoctorID:  doctorID,
		Notes:     pgtype.Text{String: notes, Valid: notes != ""},
	})
	if err != nil {
		return uuid.Nil, err
	}

	for _, item := range items {
		_, err := qtx.InsertPrescriptionItem(ctx, dbgen.InsertPrescriptionItemParams{
			PrescriptionID: newID,
			MedicationName: item.MedicationName,
			Dosage:         pgtype.Text{String: item.Dosage, Valid: item.Dosage != ""},
			Frequency:      pgtype.Text{String: item.Frequency, Valid: item.Frequency != ""},
			Duration:       pgtype.Text{String: item.Duration, Valid: item.Duration != ""},
			Timing:         pgtype.Text{String: item.Timing, Valid: item.Timing != ""},
			Instructions:   pgtype.Text{String: item.Instructions, Valid: item.Instructions != ""},
		})
		if err != nil {
			return uuid.Nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}

	return newID, nil
}

func (r *DBRepository) UpdatePrescriptionOCRResults(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	rowsAffected, err := qtx.UpdatePrescriptionOCRStatus(ctx, dbgen.UpdatePrescriptionOCRStatusParams{
		ID:          prescriptionID,
		Status:      status,
		Notes:       pgtype.Text{String: notes, Valid: notes != ""},
		OcrProvider: pgtype.Text{String: ocrProvider, Valid: ocrProvider != ""},
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		// Prescription is no longer in pending_ocr (e.g. cancelled or already handled)
		return nil
	}

	if len(items) > 0 {
		if err := qtx.DeletePrescriptionItems(ctx, prescriptionID); err != nil {
			return err
		}
		for _, item := range items {
			_, err := qtx.InsertPrescriptionItem(ctx, dbgen.InsertPrescriptionItemParams{
				PrescriptionID: prescriptionID,
				MedicationName: item.MedicationName,
				Dosage:         pgtype.Text{String: item.Dosage, Valid: item.Dosage != ""},
				Frequency:      pgtype.Text{String: item.Frequency, Valid: item.Frequency != ""},
				Duration:       pgtype.Text{String: item.Duration, Valid: item.Duration != ""},
				Timing:         pgtype.Text{String: item.Timing, Valid: item.Timing != ""},
				Instructions:   pgtype.Text{String: item.Instructions, Valid: item.Instructions != ""},
			})
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

func (r *DBRepository) VerifyPrescription(ctx context.Context, prescriptionID, doctorID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	rowsAffected, err := qtx.VerifyPrescription(ctx, dbgen.VerifyPrescriptionParams{
		ID:       prescriptionID,
		Status:   status,
		Notes:    pgtype.Text{String: notes, Valid: notes != ""},
		DoctorID: doctorID,
	})
	if err != nil {
		return false, err
	}
	if rowsAffected == 0 {
		return false, nil
	}

	if items != nil {
		if err := qtx.DeletePrescriptionItems(ctx, prescriptionID); err != nil {
			return false, err
		}
		for _, item := range items {
			_, err := qtx.InsertPrescriptionItem(ctx, dbgen.InsertPrescriptionItemParams{
				PrescriptionID: prescriptionID,
				MedicationName: item.MedicationName,
				Dosage:         pgtype.Text{String: item.Dosage, Valid: item.Dosage != ""},
				Frequency:      pgtype.Text{String: item.Frequency, Valid: item.Frequency != ""},
				Duration:       pgtype.Text{String: item.Duration, Valid: item.Duration != ""},
				Timing:         pgtype.Text{String: item.Timing, Valid: item.Timing != ""},
				Instructions:   pgtype.Text{String: item.Instructions, Valid: item.Instructions != ""},
			})
			if err != nil {
				return false, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	return true, nil
}

func (r *DBRepository) fetchPrescriptionItems(ctx context.Context, prescriptionID uuid.UUID) ([]models.PrescriptionItem, error) {
	items, err := r.queries.GetPrescriptionItemsByPrescriptionID(ctx, prescriptionID)
	if err != nil {
		return nil, err
	}
	res := make([]models.PrescriptionItem, 0, len(items))
	for _, it := range items {
		res = append(res, models.PrescriptionItem{
			ID:             it.ID,
			PrescriptionID: it.PrescriptionID,
			MedicationName: it.MedicationName,
			Dosage:         it.Dosage.String,
			Frequency:      it.Frequency.String,
			Duration:       it.Duration.String,
			Timing:         it.Timing.String,
			Instructions:   it.Instructions.String,
			CreatedAt:      it.CreatedAt.Time,
		})
	}
	return res, nil
}

func (r *DBRepository) fetchPrescriptionItemsBatch(ctx context.Context, prescriptionIDs []uuid.UUID) (map[uuid.UUID][]models.PrescriptionItem, error) {
	itemsMap := make(map[uuid.UUID][]models.PrescriptionItem, len(prescriptionIDs))
	for _, id := range prescriptionIDs {
		itemsMap[id] = []models.PrescriptionItem{}
	}
	if len(prescriptionIDs) == 0 {
		return itemsMap, nil
	}

	rows, err := r.queries.GetPrescriptionItemsByPrescriptionIDs(ctx, prescriptionIDs)
	if err != nil {
		return nil, err
	}

	for _, it := range rows {
		itemsMap[it.PrescriptionID] = append(itemsMap[it.PrescriptionID], models.PrescriptionItem{
			ID:             it.ID,
			PrescriptionID: it.PrescriptionID,
			MedicationName: it.MedicationName,
			Dosage:         it.Dosage.String,
			Frequency:      it.Frequency.String,
			Duration:       it.Duration.String,
			Timing:         it.Timing.String,
			Instructions:   it.Instructions.String,
			CreatedAt:      it.CreatedAt.Time,
		})
	}
	return itemsMap, nil
}

func (r *DBRepository) GetPrescriptionsPendingReview(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetPrescriptionsPendingReview(ctx, dbgen.GetPrescriptionsPendingReviewParams{
		DoctorID: doctorID,
		Limit:    int32(lim),
		Offset:   int32(off),
	})
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	itemsMap, err := r.fetchPrescriptionItemsBatch(ctx, ids)
	if err != nil {
		return nil, err
	}

	prescriptions := make([]models.Prescription, 0, len(rows))
	for _, row := range rows {
		prescriptions = append(prescriptions, models.Prescription{
			ID:          row.ID,
			PatientID:   row.PatientID,
			DoctorID:    row.DoctorID,
			Source:      row.Source,
			Status:      row.Status,
			FileName:    row.FileName.String,
			Notes:       row.Notes.String,
			OCRProvider: row.OcrProvider.String,
			Items:       itemsMap[row.ID],
			CreatedAt:   row.CreatedAt.Time,
			UpdatedAt:   row.UpdatedAt.Time,
			PatientName: row.PatientFirstName + " " + row.PatientLastName,
			DoctorName:  row.DoctorFirstName + " " + row.DoctorLastName,
		})
	}
	return prescriptions, nil
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

	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	itemsMap, err := r.fetchPrescriptionItemsBatch(ctx, ids)
	if err != nil {
		return nil, err
	}

	prescriptions := make([]models.Prescription, 0, len(rows))
	for _, row := range rows {
		prescriptions = append(prescriptions, models.Prescription{
			ID:          row.ID,
			PatientID:   row.PatientID,
			DoctorID:    row.DoctorID,
			Source:      row.Source,
			Status:      row.Status,
			FileName:    row.FileName.String,
			Notes:       row.Notes.String,
			OCRProvider: row.OcrProvider.String,
			Items:       itemsMap[row.ID],
			CreatedAt:   row.CreatedAt.Time,
			UpdatedAt:   row.UpdatedAt.Time,
			DoctorName:  row.FirstName + " " + row.LastName,
		})
	}
	return prescriptions, nil
}

func (r *DBRepository) GetPrescriptionByFilename(ctx context.Context, patientID uuid.UUID, filename string) (models.Prescription, error) {
	row, err := r.queries.GetPrescriptionByFilename(ctx, dbgen.GetPrescriptionByFilenameParams{
		PatientID: patientID,
		FileName:  pgtype.Text{String: filename, Valid: true},
	})
	if err != nil {
		return models.Prescription{}, err
	}
	return models.Prescription{
		ID:     row.ID,
		Status: row.Status,
	}, nil
}

func (r *DBRepository) GetPrescriptionsForPatient(ctx context.Context, doctorID, patientID uuid.UUID, status string, limit, offset int) ([]models.Prescription, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetPrescriptionsForPatient(ctx, dbgen.GetPrescriptionsForPatientParams{
		PatientID: patientID,
		Status:    pgtype.Text{String: status, Valid: status != ""},
		DoctorID:  doctorID,
		Limit:     int32(lim),
		Offset:    int32(off),
	})
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	itemsMap, err := r.fetchPrescriptionItemsBatch(ctx, ids)
	if err != nil {
		return nil, err
	}

	prescriptions := make([]models.Prescription, 0, len(rows))
	for _, row := range rows {
		prescriptions = append(prescriptions, models.Prescription{
			ID:          row.ID,
			PatientID:   row.PatientID,
			DoctorID:    row.DoctorID,
			Source:      row.Source,
			Status:      row.Status,
			FileName:    row.FileName.String,
			Notes:       row.Notes.String,
			OCRProvider: row.OcrProvider.String,
			Items:       itemsMap[row.ID],
			CreatedAt:   row.CreatedAt.Time,
			UpdatedAt:   row.UpdatedAt.Time,
			DoctorName:  row.FirstName + " " + row.LastName,
		})
	}
	return prescriptions, nil
}

func (r *DBRepository) GetPrescriptionByFilenameForDoctor(ctx context.Context, doctorID uuid.UUID, filename string) (models.Prescription, error) {
	row, err := r.queries.GetPrescriptionByFilenameForDoctor(ctx, dbgen.GetPrescriptionByFilenameForDoctorParams{
		FileName: pgtype.Text{String: filename, Valid: true},
		DoctorID: doctorID,
	})
	if err != nil {
		return models.Prescription{}, err
	}
	return models.Prescription{
		ID:     row.ID,
		Status: row.Status,
	}, nil
}

func (r *DBRepository) GetPrescriptionByID(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
	row, err := r.queries.GetPrescriptionByID(ctx, id)
	if err != nil {
		return models.Prescription{}, err
	}
	items, err := r.fetchPrescriptionItems(ctx, row.ID)
	if err != nil {
		return models.Prescription{}, err
	}

	return models.Prescription{
		ID:          row.ID,
		PatientID:   row.PatientID,
		DoctorID:    row.DoctorID,
		Source:      row.Source,
		Status:      row.Status,
		FileName:    row.FileName.String,
		Notes:       row.Notes.String,
		OCRProvider: row.OcrProvider.String,
		Items:       items,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
		DoctorName:  row.DoctorFirstName + " " + row.DoctorLastName,
		PatientName: row.PatientFirstName + " " + row.PatientLastName,
	}, nil
}
