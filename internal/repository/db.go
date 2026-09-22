package repository

import (
	"context"
	"fmt"
	"strings"
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
func (r *DBRepository) CreateAppointment(ctx context.Context, id, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType, meetingLink, meetingID string) (uuid.UUID, error) {
	if id == uuid.Nil {
		id = uuid.New()
	}
	return r.queries.CreateAppointment(ctx, dbgen.CreateAppointmentParams{
		ID:              id,
		PatientID:       patientID,
		DoctorID:        doctorID,
		StartTime:       pgtype.Timestamptz{Time: startTime, Valid: true},
		EndTime:         pgtype.Timestamptz{Time: endTime, Valid: true},
		AppointmentType: pgtype.Text{String: apptType, Valid: apptType != ""},
		MeetingLink:     pgtype.Text{String: meetingLink, Valid: meetingLink != ""},
		MeetingID:       pgtype.Text{String: meetingID, Valid: meetingID != ""},
	})
}

func (r *DBRepository) UpdateAppointmentMeetingRoom(ctx context.Context, apptID uuid.UUID, meetingLink, meetingID string) error {
	return r.queries.UpdateAppointmentMeetingRoom(ctx, dbgen.UpdateAppointmentMeetingRoomParams{
		MeetingLink: pgtype.Text{String: meetingLink, Valid: meetingLink != ""},
		MeetingID:   pgtype.Text{String: meetingID, Valid: meetingID != ""},
		ID:          apptID,
	})
}

func (r *DBRepository) GetAppointmentByID(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
	row, err := r.queries.GetAppointmentByID(ctx, id)
	if err != nil {
		return models.Appointment{}, err
	}
	return models.Appointment{
		ID:              row.ID,
		PatientID:       row.PatientID,
		DoctorID:        row.DoctorID,
		StartTime:       row.StartTime.Time,
		EndTime:         row.EndTime.Time,
		Status:          row.Status.String,
		Type:            row.AppointmentType.String,
		MeetingLink:     row.MeetingLink.String,
		MeetingID:       row.MeetingID.String,
		DoctorName:      row.DoctorFirstName + " " + row.DoctorLastName,
		DoctorSpecialty: row.DoctorSpecialty.String,
		PatientName:     row.PatientFirstName + " " + row.PatientLastName,
		CreatedAt:       row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetDoctorAppointmentsInRange(ctx context.Context, doctorID uuid.UUID, startTime, endTime time.Time) ([]models.Appointment, error) {
	rows, err := r.queries.GetDoctorAppointmentsInRange(ctx, dbgen.GetDoctorAppointmentsInRangeParams{
		DoctorID:   doctorID,
		RangeStart: pgtype.Timestamptz{Time: startTime, Valid: true},
		RangeEnd:   pgtype.Timestamptz{Time: endTime, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	appts := make([]models.Appointment, 0, len(rows))
	for _, row := range rows {
		appts = append(appts, models.Appointment{
			ID:        row.ID,
			DoctorID:  row.DoctorID,
			StartTime: row.StartTime.Time,
			EndTime:   row.EndTime.Time,
			Status:    row.Status.String,
		})
	}
	return appts, nil
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
			MeetingLink: row.MeetingLink.String,
			MeetingID:   row.MeetingID.String,
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
			MeetingLink:     row.MeetingLink.String,
			MeetingID:       row.MeetingID.String,
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
			MeetingLink:     row.MeetingLink.String,
			MeetingID:       row.MeetingID.String,
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


func parseDurationString(s string) (time.Duration, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, false
	}
	var num int
	var unit string
	_, err := fmt.Sscanf(s, "%d %s", &num, &unit)
	if err != nil || num <= 0 {
		num = 0
		unit = ""
		for i, r := range s {
			if r >= '0' && r <= '9' {
				num = num*10 + int(r-'0')
			} else {
				unit = strings.TrimSpace(s[i:])
				break
			}
		}
	}
	if num <= 0 {
		return 0, false
	}
	unit = strings.TrimSpace(unit)
	switch {
	case strings.HasPrefix(unit, "day") || unit == "d":
		if num > 3650 {
			num = 3650
		}
		return time.Duration(num) * 24 * time.Hour, true
	case strings.HasPrefix(unit, "week") || unit == "w" || unit == "wk" || unit == "wks":
		if num > 520 {
			num = 520
		}
		return time.Duration(num) * 7 * 24 * time.Hour, true
	case strings.HasPrefix(unit, "month") || unit == "m" || unit == "mo" || unit == "mos":
		if num > 120 {
			num = 120
		}
		return time.Duration(num) * 30 * 24 * time.Hour, true
	case strings.HasPrefix(unit, "year") || unit == "y" || unit == "yr" || unit == "yrs":
		if num > 10 {
			num = 10
		}
		return time.Duration(num) * 365 * 24 * time.Hour, true
	}
	return 0, false
}

func calculatePrescriptionExpiry(items []models.PrescriptionItem) time.Time {
	maxDuration := 30 * 24 * time.Hour
	foundValid := false
	for _, item := range items {
		if d, ok := parseDurationString(item.Duration); ok {
			if !foundValid || d > maxDuration {
				maxDuration = d
			}
			foundValid = true
		}
	}
	return time.Now().Add(maxDuration)
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

	if len(items) > 0 {
		exp := calculatePrescriptionExpiry(items)
		if _, err := tx.Exec(ctx, "UPDATE prescriptions SET expires_at = $1 WHERE id = $2", exp, newID); err != nil {
			return uuid.Nil, fmt.Errorf("failed to update prescription expiry: %w", err)
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
		if status == "approved" && len(items) > 0 {
			exp := calculatePrescriptionExpiry(items)
			if _, err := tx.Exec(ctx, "UPDATE prescriptions SET expires_at = $1 WHERE id = $2", exp, prescriptionID); err != nil {
				return false, fmt.Errorf("failed to update prescription expiry: %w", err)
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

func (r *DBRepository) UpdatePrescriptionFileName(ctx context.Context, prescriptionID uuid.UUID, fileName string) error {
	tag, err := r.pool.Exec(ctx, "UPDATE prescriptions SET file_name = $1, updated_at = now() WHERE id = $2", fileName, prescriptionID)
	if err != nil {
		return fmt.Errorf("failed to update prescription file_name: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("prescription %s not found", prescriptionID)
	}
	return nil
}

// Helpers for Phase 4 conversions
func timeToPgTime(s string) (pgtype.Time, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return pgtype.Time{}, err
	}
	micros := int64(t.Hour())*3600*1e6 + int64(t.Minute())*60*1e6
	return pgtype.Time{Microseconds: micros, Valid: true}, nil
}

func pgTimeToStr(t pgtype.Time) string {
	if !t.Valid {
		return ""
	}
	totalSec := t.Microseconds / 1e6
	hours := totalSec / 3600
	mins := (totalSec % 3600) / 60
	return fmt.Sprintf("%02d:%02d", hours, mins)
}

func floatToNumeric(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	var num pgtype.Numeric
	_ = num.Scan(fmt.Sprintf("%.2f", *f))
	return num
}

func numericToFloat(num pgtype.Numeric) *float64 {
	if !num.Valid {
		return nil
	}
	val, err := num.Float64Value()
	if err != nil || !val.Valid {
		return nil
	}
	f := val.Float64
	return &f
}

// Doctor Schedules
func (r *DBRepository) UpsertDoctorSchedule(ctx context.Context, schedule models.DoctorSchedule) (models.DoctorSchedule, error) {
	startTime, err := timeToPgTime(schedule.StartTime)
	if err != nil {
		return models.DoctorSchedule{}, fmt.Errorf("invalid start_time format (expected HH:MM): %w", err)
	}
	endTime, err := timeToPgTime(schedule.EndTime)
	if err != nil {
		return models.DoctorSchedule{}, fmt.Errorf("invalid end_time format (expected HH:MM): %w", err)
	}

	tz := schedule.Timezone
	if tz == "" {
		tz = "UTC"
	}
	slotDur := schedule.SlotDuration
	if slotDur == 0 {
		slotDur = 30
	}

	row, err := r.queries.UpsertDoctorSchedule(ctx, dbgen.UpsertDoctorScheduleParams{
		DoctorID:     schedule.DoctorID,
		DayOfWeek:    int32(schedule.DayOfWeek),
		StartTime:    startTime,
		EndTime:      endTime,
		SlotDuration: int32(slotDur),
		Timezone:     tz,
		IsActive:     pgtype.Bool{Bool: schedule.IsActive, Valid: true},
	})
	if err != nil {
		return models.DoctorSchedule{}, err
	}

	return models.DoctorSchedule{
		ID:           row.ID,
		DoctorID:     row.DoctorID,
		DayOfWeek:    int(row.DayOfWeek),
		StartTime:    pgTimeToStr(row.StartTime),
		EndTime:      pgTimeToStr(row.EndTime),
		SlotDuration: int(row.SlotDuration),
		Timezone:     row.Timezone,
		IsActive:     row.IsActive.Bool,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}, nil
}

func (r *DBRepository) GetDoctorSchedules(ctx context.Context, doctorID uuid.UUID) ([]models.DoctorSchedule, error) {
	rows, err := r.queries.GetDoctorSchedules(ctx, doctorID)
	if err != nil {
		return nil, err
	}
	schedules := make([]models.DoctorSchedule, 0, len(rows))
	for _, row := range rows {
		schedules = append(schedules, models.DoctorSchedule{
			ID:           row.ID,
			DoctorID:     row.DoctorID,
			DayOfWeek:    int(row.DayOfWeek),
			StartTime:    pgTimeToStr(row.StartTime),
			EndTime:      pgTimeToStr(row.EndTime),
			SlotDuration: int(row.SlotDuration),
			Timezone:     row.Timezone,
			IsActive:     row.IsActive.Bool,
			CreatedAt:    row.CreatedAt.Time,
			UpdatedAt:    row.UpdatedAt.Time,
		})
	}
	return schedules, nil
}

func (r *DBRepository) GetDoctorScheduleByDay(ctx context.Context, doctorID uuid.UUID, dayOfWeek int) (models.DoctorSchedule, error) {
	row, err := r.queries.GetDoctorScheduleByDay(ctx, dbgen.GetDoctorScheduleByDayParams{
		DoctorID:  doctorID,
		DayOfWeek: int32(dayOfWeek),
	})
	if err != nil {
		return models.DoctorSchedule{}, err
	}
	return models.DoctorSchedule{
		ID:           row.ID,
		DoctorID:     row.DoctorID,
		DayOfWeek:    int(row.DayOfWeek),
		StartTime:    pgTimeToStr(row.StartTime),
		EndTime:      pgTimeToStr(row.EndTime),
		SlotDuration: int(row.SlotDuration),
		Timezone:     row.Timezone,
		IsActive:     row.IsActive.Bool,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}, nil
}

func (r *DBRepository) UpsertDoctorSchedulesTx(ctx context.Context, doctorID uuid.UUID, schedules []models.DoctorSchedule) ([]models.DoctorSchedule, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin schedule transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)
	saved := make([]models.DoctorSchedule, 0, len(schedules))

	for _, schedule := range schedules {
		startTime, err := timeToPgTime(schedule.StartTime)
		if err != nil {
			return nil, fmt.Errorf("invalid start_time format (expected HH:MM): %w", err)
		}
		endTime, err := timeToPgTime(schedule.EndTime)
		if err != nil {
			return nil, fmt.Errorf("invalid end_time format (expected HH:MM): %w", err)
		}

		tz := schedule.Timezone
		if tz == "" {
			tz = "UTC"
		}
		slotDur := schedule.SlotDuration
		if slotDur == 0 {
			slotDur = 30
		}

		row, err := qtx.UpsertDoctorSchedule(ctx, dbgen.UpsertDoctorScheduleParams{
			DoctorID:     doctorID,
			DayOfWeek:    int32(schedule.DayOfWeek),
			StartTime:    startTime,
			EndTime:      endTime,
			SlotDuration: int32(slotDur),
			Timezone:     tz,
			IsActive:     pgtype.Bool{Bool: schedule.IsActive, Valid: true},
		})
		if err != nil {
			return nil, err
		}

		saved = append(saved, models.DoctorSchedule{
			ID:           row.ID,
			DoctorID:     row.DoctorID,
			DayOfWeek:    int(row.DayOfWeek),
			StartTime:    pgTimeToStr(row.StartTime),
			EndTime:      pgTimeToStr(row.EndTime),
			SlotDuration: int(row.SlotDuration),
			Timezone:     row.Timezone,
			IsActive:     row.IsActive.Bool,
			CreatedAt:    row.CreatedAt.Time,
			UpdatedAt:    row.UpdatedAt.Time,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit schedule transaction: %w", err)
	}

	return saved, nil
}

func (r *DBRepository) DeleteDoctorScheduleByDay(ctx context.Context, doctorID uuid.UUID, dayOfWeek int) error {
	return r.queries.DeleteDoctorScheduleByDay(ctx, dbgen.DeleteDoctorScheduleByDayParams{
		DoctorID:  doctorID,
		DayOfWeek: int32(dayOfWeek),
	})
}

// Patient Vitals
func (r *DBRepository) CreatePatientVital(ctx context.Context, vital models.PatientVital) (uuid.UUID, error) {
	var sys, dia, hr pgtype.Int4
	if vital.SystolicBP != nil {
		sys = pgtype.Int4{Int32: int32(*vital.SystolicBP), Valid: true}
	}
	if vital.DiastolicBP != nil {
		dia = pgtype.Int4{Int32: int32(*vital.DiastolicBP), Valid: true}
	}
	if vital.HeartRate != nil {
		hr = pgtype.Int4{Int32: int32(*vital.HeartRate), Valid: true}
	}

	recAt := vital.RecordedAt
	if recAt.IsZero() {
		recAt = time.Now()
	}

	row, err := r.queries.CreatePatientVital(ctx, dbgen.CreatePatientVitalParams{
		PatientID:        vital.PatientID,
		RecordedBy:       vital.RecordedBy,
		RecordedAt:       pgtype.Timestamptz{Time: recAt, Valid: true},
		SystolicBp:       sys,
		DiastolicBp:      dia,
		HeartRate:        hr,
		BloodGlucose:     floatToNumeric(vital.BloodGlucose),
		OxygenSaturation: floatToNumeric(vital.OxygenSaturation),
		Temperature:      floatToNumeric(vital.Temperature),
		WeightKg:         floatToNumeric(vital.WeightKg),
		Notes:            pgtype.Text{String: vital.Notes, Valid: vital.Notes != ""},
	})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

func (r *DBRepository) GetPatientVitals(ctx context.Context, patientID uuid.UUID, startDate, endDate *time.Time, limit, offset int) ([]models.PatientVital, error) {
	lim, off := clampPagination(limit, offset)
	var startPg, endPg pgtype.Timestamptz
	if startDate != nil {
		startPg = pgtype.Timestamptz{Time: *startDate, Valid: true}
	}
	if endDate != nil {
		endPg = pgtype.Timestamptz{Time: *endDate, Valid: true}
	}

	rows, err := r.queries.GetPatientVitals(ctx, dbgen.GetPatientVitalsParams{
		PatientID: patientID,
		StartDate: startPg,
		EndDate:   endPg,
		Limit:     int32(lim),
		Offset:    int32(off),
	})
	if err != nil {
		return nil, err
	}

	vitals := make([]models.PatientVital, 0, len(rows))
	for _, row := range rows {
		var sys, dia, hr *int
		if row.SystolicBp.Valid {
			s := int(row.SystolicBp.Int32)
			sys = &s
		}
		if row.DiastolicBp.Valid {
			d := int(row.DiastolicBp.Int32)
			dia = &d
		}
		if row.HeartRate.Valid {
			h := int(row.HeartRate.Int32)
			hr = &h
		}

		recorderName := strings.TrimSpace(fmt.Sprintf("%s %s", row.RecorderFirstName, row.RecorderLastName))

		vitals = append(vitals, models.PatientVital{
			ID:               row.ID,
			PatientID:        row.PatientID,
			RecordedBy:       row.RecordedBy,
			RecordedAt:       row.RecordedAt.Time,
			SystolicBP:       sys,
			DiastolicBP:      dia,
			HeartRate:        hr,
			BloodGlucose:     numericToFloat(row.BloodGlucose),
			OxygenSaturation: numericToFloat(row.OxygenSaturation),
			Temperature:      numericToFloat(row.Temperature),
			WeightKg:         numericToFloat(row.WeightKg),
			Notes:            row.Notes.String,
			CreatedAt:        row.CreatedAt.Time,
			RecorderName:     recorderName,
			RecorderRole:     row.RecorderRole,
		})
	}
	return vitals, nil
}

func (r *DBRepository) GetLatestPatientVital(ctx context.Context, patientID uuid.UUID) (models.PatientVital, error) {
	row, err := r.queries.GetLatestPatientVital(ctx, patientID)
	if err != nil {
		return models.PatientVital{}, err
	}

	var sys, dia, hr *int
	if row.SystolicBp.Valid {
		s := int(row.SystolicBp.Int32)
		sys = &s
	}
	if row.DiastolicBp.Valid {
		d := int(row.DiastolicBp.Int32)
		dia = &d
	}
	if row.HeartRate.Valid {
		h := int(row.HeartRate.Int32)
		hr = &h
	}
	recorderName := strings.TrimSpace(fmt.Sprintf("%s %s", row.RecorderFirstName, row.RecorderLastName))

	return models.PatientVital{
		ID:               row.ID,
		PatientID:        row.PatientID,
		RecordedBy:       row.RecordedBy,
		RecordedAt:       row.RecordedAt.Time,
		SystolicBP:       sys,
		DiastolicBP:      dia,
		HeartRate:        hr,
		BloodGlucose:     numericToFloat(row.BloodGlucose),
		OxygenSaturation: numericToFloat(row.OxygenSaturation),
		Temperature:      numericToFloat(row.Temperature),
		WeightKg:         numericToFloat(row.WeightKg),
		Notes:            row.Notes.String,
		CreatedAt:        row.CreatedAt.Time,
		RecorderName:     recorderName,
		RecorderRole:     row.RecorderRole,
	}, nil
}

// Medication Schedules & Adherence
func (r *DBRepository) GetActivePrescriptionItemsForPatient(ctx context.Context, patientID uuid.UUID, targetDate time.Time) ([]models.PrescriptionItem, error) {
	if targetDate.IsZero() {
		targetDate = time.Now()
	}
	rows, err := r.queries.GetActivePrescriptionItemsForPatient(ctx, dbgen.GetActivePrescriptionItemsForPatientParams{
		PatientID:  patientID,
		TargetDate: pgtype.Timestamptz{Time: targetDate, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	items := make([]models.PrescriptionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, models.PrescriptionItem{
			ID:             row.ID,
			PrescriptionID: row.PrescriptionID,
			MedicationName: row.MedicationName,
			Dosage:         row.Dosage.String,
			Frequency:      row.Frequency.String,
			Duration:       row.Duration.String,
			Timing:         row.Timing.String,
			Instructions:   row.Instructions.String,
			CreatedAt:      row.CreatedAt.Time,
		})
	}
	return items, nil
}

func (r *DBRepository) UpsertMedicationLog(ctx context.Context, log models.MedicationLog) (models.MedicationLog, error) {
	var takenAt pgtype.Timestamptz
	if log.TakenAt != nil {
		takenAt = pgtype.Timestamptz{Time: *log.TakenAt, Valid: true}
	}

	datePg := pgtype.Date{
		Time:  time.Date(log.ScheduledDate.Year(), log.ScheduledDate.Month(), log.ScheduledDate.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}

	doseNum := int32(log.DoseNumber)
	if doseNum <= 0 {
		doseNum = 1
	}

	row, err := r.queries.UpsertMedicationLog(ctx, dbgen.UpsertMedicationLogParams{
		PatientID:          log.PatientID,
		PrescriptionItemID: log.PrescriptionItemID,
		ScheduledDate:      datePg,
		TimeOfDay:          log.TimeOfDay,
		DoseNumber:         doseNum,
		MealTiming:         pgtype.Text{String: log.MealTiming, Valid: log.MealTiming != ""},
		Status:             log.Status,
		TakenAt:            takenAt,
		Notes:              pgtype.Text{String: log.Notes, Valid: log.Notes != ""},
	})
	if err != nil {
		return models.MedicationLog{}, err
	}

	var resTakenAt *time.Time
	if row.TakenAt.Valid {
		t := row.TakenAt.Time
		resTakenAt = &t
	}

	return models.MedicationLog{
		ID:                 row.ID,
		PatientID:          row.PatientID,
		PrescriptionItemID: row.PrescriptionItemID,
		ScheduledDate:      row.ScheduledDate.Time,
		TimeOfDay:          row.TimeOfDay,
		DoseNumber:         int(row.DoseNumber),
		MealTiming:         row.MealTiming.String,
		Status:             row.Status,
		TakenAt:            resTakenAt,
		Notes:              row.Notes.String,
		CreatedAt:          row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetMedicationLogsByDate(ctx context.Context, patientID uuid.UUID, date time.Time) ([]models.MedicationLog, error) {
	datePg := pgtype.Date{
		Time:  time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
	rows, err := r.queries.GetMedicationLogsByDate(ctx, dbgen.GetMedicationLogsByDateParams{
		PatientID:     patientID,
		ScheduledDate: datePg,
	})
	if err != nil {
		return nil, err
	}
	logs := make([]models.MedicationLog, 0, len(rows))
	for _, row := range rows {
		var takenAt *time.Time
		if row.TakenAt.Valid {
			t := row.TakenAt.Time
			takenAt = &t
		}
		logs = append(logs, models.MedicationLog{
			ID:                 row.ID,
			PatientID:          row.PatientID,
			PrescriptionItemID: row.PrescriptionItemID,
			ScheduledDate:      row.ScheduledDate.Time,
			TimeOfDay:          row.TimeOfDay,
			DoseNumber:         int(row.DoseNumber),
			MealTiming:         row.MealTiming.String,
			Status:             row.Status,
			TakenAt:            takenAt,
			Notes:              row.Notes.String,
			MedicationName:     row.MedicationName,
			Dosage:             row.Dosage.String,
			Timing:             row.Timing.String,
			Instructions:       row.Instructions.String,
			CreatedAt:          row.CreatedAt.Time,
		})
	}
	return logs, nil
}

func (r *DBRepository) VerifyPrescriptionItemOwnership(ctx context.Context, itemID, patientID uuid.UUID) (bool, error) {
	return r.queries.VerifyPrescriptionItemOwnership(ctx, dbgen.VerifyPrescriptionItemOwnershipParams{
		ID:        itemID,
		PatientID: patientID,
	})
}

func (r *DBRepository) HasDoctorPatientRelationship(ctx context.Context, doctorID, patientID uuid.UUID) (bool, error) {
	res, err := r.queries.HasDoctorPatientRelationship(ctx, dbgen.HasDoctorPatientRelationshipParams{
		DoctorID:  doctorID,
		PatientID: patientID,
	})
	if err != nil {
		return false, err
	}
	return res.Bool, nil
}

// ==========================================
// PHASE 5: REFRESH TOKENS & SESSION MGMT
// ==========================================

func (r *DBRepository) CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error) {
	row, err := r.queries.CreateRefreshToken(ctx, dbgen.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return models.RefreshToken{}, err
	}
	var revokedAt *time.Time
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		revokedAt = &t
	}
	var replacedBy *uuid.UUID
	if row.ReplacedByTokenID.Valid {
		u := uuid.UUID(row.ReplacedByTokenID.Bytes)
		replacedBy = &u
	}
	return models.RefreshToken{
		ID:                row.ID,
		UserID:            row.UserID,
		TokenHash:         row.TokenHash,
		ExpiresAt:         row.ExpiresAt.Time,
		RevokedAt:         revokedAt,
		ReplacedByTokenID: replacedBy,
		CreatedAt:         row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (models.RefreshToken, error) {
	row, err := r.queries.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return models.RefreshToken{}, err
	}
	var revokedAt *time.Time
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		revokedAt = &t
	}
	var replacedBy *uuid.UUID
	if row.ReplacedByTokenID.Valid {
		u := uuid.UUID(row.ReplacedByTokenID.Bytes)
		replacedBy = &u
	}
	return models.RefreshToken{
		ID:                row.ID,
		UserID:            row.UserID,
		TokenHash:         row.TokenHash,
		ExpiresAt:         row.ExpiresAt.Time,
		RevokedAt:         revokedAt,
		ReplacedByTokenID: replacedBy,
		CreatedAt:         row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) RevokeRefreshToken(ctx context.Context, id uuid.UUID, replacedByTokenID *uuid.UUID) error {
	var replaced pgtype.UUID
	if replacedByTokenID != nil {
		replaced = pgtype.UUID{Bytes: *replacedByTokenID, Valid: true}
	}
	return r.queries.RevokeRefreshToken(ctx, dbgen.RevokeRefreshTokenParams{
		ID:                id,
		ReplacedByTokenID: replaced,
	})
}

func (r *DBRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return r.queries.RevokeAllUserRefreshTokens(ctx, userID)
}

// ==========================================
// PHASE 5: HIPAA ePHI AUDIT LOGGING
// ==========================================

func (r *DBRepository) CreateAuditLog(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
	var uid, rid, pid, reqID pgtype.UUID
	if log.UserID != nil {
		uid = pgtype.UUID{Bytes: *log.UserID, Valid: true}
	}
	if log.ResourceID != nil {
		rid = pgtype.UUID{Bytes: *log.ResourceID, Valid: true}
	}
	if log.PatientID != nil {
		pid = pgtype.UUID{Bytes: *log.PatientID, Valid: true}
	}
	if log.RequestID != nil {
		reqID = pgtype.UUID{Bytes: *log.RequestID, Valid: true}
	}

	metaBytes := []byte(log.Metadata)
	if len(metaBytes) == 0 {
		metaBytes = []byte("{}")
	}

	row, err := r.queries.CreateAuditLog(ctx, dbgen.CreateAuditLogParams{
		UserID:       uid,
		UserRole:     pgtype.Text{String: log.UserRole, Valid: log.UserRole != ""},
		Action:       log.Action,
		ResourceType: log.ResourceType,
		ResourceID:   rid,
		PatientID:    pid,
		IpAddress:    pgtype.Text{String: log.IPAddress, Valid: log.IPAddress != ""},
		UserAgent:    pgtype.Text{String: log.UserAgent, Valid: log.UserAgent != ""},
		RequestID:    reqID,
		StatusCode:   pgtype.Int4{Int32: int32(log.StatusCode), Valid: log.StatusCode > 0},
		Metadata:     metaBytes,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

func (r *DBRepository) GetAuditLogsByPatientID(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.PhiAuditLog, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetAuditLogsByPatientID(ctx, dbgen.GetAuditLogsByPatientIDParams{
		PatientID: pgtype.UUID{Bytes: patientID, Valid: true},
		Limit:     int32(lim),
		Offset:    int32(off),
	})
	if err != nil {
		return nil, err
	}
	logs := make([]models.PhiAuditLog, 0, len(rows))
	for _, row := range rows {
		var uid, rid, pid, reqID *uuid.UUID
		if row.UserID.Valid {
			u := uuid.UUID(row.UserID.Bytes)
			uid = &u
		}
		if row.ResourceID.Valid {
			u := uuid.UUID(row.ResourceID.Bytes)
			rid = &u
		}
		if row.PatientID.Valid {
			u := uuid.UUID(row.PatientID.Bytes)
			pid = &u
		}
		if row.RequestID.Valid {
			u := uuid.UUID(row.RequestID.Bytes)
			reqID = &u
		}
		logs = append(logs, models.PhiAuditLog{
			ID:           row.ID,
			UserID:       uid,
			UserRole:     row.UserRole.String,
			Action:       row.Action,
			ResourceType: row.ResourceType,
			ResourceID:   rid,
			PatientID:    pid,
			IPAddress:    row.IpAddress.String,
			UserAgent:    row.UserAgent.String,
			RequestID:    reqID,
			StatusCode:   int(row.StatusCode.Int32),
			Metadata:     string(row.Metadata),
			CreatedAt:    row.CreatedAt.Time,
		})
	}
	return logs, nil
}

func (r *DBRepository) GetAuditLogs(ctx context.Context, limit, offset int) ([]models.PhiAuditLog, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetAuditLogs(ctx, dbgen.GetAuditLogsParams{
		Limit:  int32(lim),
		Offset: int32(off),
	})
	if err != nil {
		return nil, err
	}
	logs := make([]models.PhiAuditLog, 0, len(rows))
	for _, row := range rows {
		var uid, rid, pid, reqID *uuid.UUID
		if row.UserID.Valid {
			u := uuid.UUID(row.UserID.Bytes)
			uid = &u
		}
		if row.ResourceID.Valid {
			u := uuid.UUID(row.ResourceID.Bytes)
			rid = &u
		}
		if row.PatientID.Valid {
			u := uuid.UUID(row.PatientID.Bytes)
			pid = &u
		}
		if row.RequestID.Valid {
			u := uuid.UUID(row.RequestID.Bytes)
			reqID = &u
		}
		logs = append(logs, models.PhiAuditLog{
			ID:           row.ID,
			UserID:       uid,
			UserRole:     row.UserRole.String,
			Action:       row.Action,
			ResourceType: row.ResourceType,
			ResourceID:   rid,
			PatientID:    pid,
			IPAddress:    row.IpAddress.String,
			UserAgent:    row.UserAgent.String,
			RequestID:    reqID,
			StatusCode:   int(row.StatusCode.Int32),
			Metadata:     string(row.Metadata),
			CreatedAt:    row.CreatedAt.Time,
		})
	}
	return logs, nil
}

