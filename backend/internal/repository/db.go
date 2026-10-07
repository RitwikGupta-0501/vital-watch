package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"golang.org/x/sync/singleflight"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository/dbgen"
)

// DBRepository is the concrete implementation of the Repository interface wrapping sqlc generated queries
type DBRepository struct {
	pool        *pgxpool.Pool
	queries     *dbgen.Queries
	riverClient *river.Client[pgx.Tx]
	roomGroup   singleflight.Group
}

var _ Repository = (*DBRepository)(nil)

func getTenantID(ctx context.Context) uuid.UUID {
	val := ctx.Value("tenant_id")
	if val == nil {
		return uuid.Nil
	}
	if str, ok := val.(string); ok {
		u, _ := uuid.Parse(str)
		return u
	}
	if u, ok := val.(uuid.UUID); ok {
		return u
	}
	return uuid.Nil
}

func (r *DBRepository) beginTxWithRLS(ctx context.Context) (pgx.Tx, *dbgen.Queries, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}

	tenantID := getTenantID(ctx)
	if tenantID != uuid.Nil {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant', $1, true)", tenantID.String()); err != nil {
			_ = tx.Rollback(ctx)
			return nil, nil, fmt.Errorf("failed to set app.current_tenant for RLS: %w", err)
		}
	}

	isPlatformAdmin := false
	if rVal := ctx.Value("role"); rVal != nil {
		if rStr, ok := rVal.(string); ok && rStr == "platform_admin" {
			isPlatformAdmin = true
		}
	}

	adminFlag := "false"
	if isPlatformAdmin {
		adminFlag = "true"
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.is_platform_admin', $1, true)", adminFlag); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("failed to set app.is_platform_admin for RLS: %w", err)
	}

	return tx, r.queries.WithTx(tx), nil
}


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

func (r *DBRepository) Queries() dbgen.Querier {
	return r.queries
}

// Ping checks if the underlying PostgreSQL connection pool is alive and reachable.
func (r *DBRepository) Ping(ctx context.Context) error {
	if r.pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	return r.pool.Ping(ctx)
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
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	newID, err := qtx.CreatePatientUser(ctx, dbgen.CreatePatientUserParams{
		TenantID: getTenantID(ctx),
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
	row, err := r.queries.GetPatientByEmail(ctx, dbgen.GetPatientByEmailParams{
		Email: email,
		TenantID: getTenantID(ctx),
	})
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
		TenantID:       row.TenantID,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetPatientByID(ctx context.Context, id uuid.UUID) (models.Patient, error) {
	row, err := r.queries.GetPatientByID(ctx, dbgen.GetPatientByIDParams{
		ID: id,
		TenantID: getTenantID(ctx),
	})
	if err != nil {
		return models.Patient{}, err
	}
	return models.Patient{
		ID:        row.ID,
		Email:     row.Email,
		FirstName: row.FirstName,
		LastName:  row.LastName,
		Role:      "patient",
		TenantID:  row.TenantID,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetPatientsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Patient, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetPatientsByDoctorID(ctx, dbgen.GetPatientsByDoctorIDParams{
		TenantID: getTenantID(ctx),
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
			TenantID:  row.TenantID,
			CreatedAt: row.CreatedAt.Time,
		})
	}
	return patients, nil
}

// Admin Related Methods
func (r *DBRepository) CreateAdmin(ctx context.Context, firstName, lastName, email, hashedPassword, department, role string) (uuid.UUID, error) {
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	tenantID := getTenantID(ctx)
	if role == "" {
		role = "tenant_admin"
	}

	newID, err := qtx.CreateAdminUser(ctx, dbgen.CreateAdminUserParams{
		Email:          email,
		HashedPassword: hashedPassword,
		Role:           role,
		TenantID:       tenantID,
	})
	if err != nil {
		return uuid.Nil, err
	}

	if department == "" {
		department = "Operations"
	}

	err = qtx.CreateAdminProfile(ctx, dbgen.CreateAdminProfileParams{
		UserID:     newID,
		FirstName:  firstName,
		LastName:   lastName,
		Department: pgtype.Text{String: department, Valid: department != ""},
		TenantID:   tenantID,
	})
	if err != nil {
		return uuid.Nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}

	return newID, nil
}

func (r *DBRepository) GetAdminByEmail(ctx context.Context, email string) (models.Admin, error) {
	row, err := r.queries.GetAdminByEmail(ctx, dbgen.GetAdminByEmailParams{
		Email: email,
		TenantID: getTenantID(ctx),
	})
	if err != nil {
		return models.Admin{}, err
	}
	return models.Admin{
		ID:             row.ID,
		Email:          row.Email,
		FirstName:      row.FirstName,
		LastName:       row.LastName,
		Department:     row.Department.String,
		HashedPassword: row.HashedPassword,
		Role:           row.Role,
		TenantID:       row.TenantID,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetAdminByID(ctx context.Context, id uuid.UUID) (models.Admin, error) {
	row, err := r.queries.GetAdminByID(ctx, dbgen.GetAdminByIDParams{
		ID: id,
		TenantID: getTenantID(ctx),
	})
	if err != nil {
		return models.Admin{}, err
	}
	return models.Admin{
		ID:         row.ID,
		Email:      row.Email,
		FirstName:  row.FirstName,
		LastName:   row.LastName,
		Department: row.Department.String,
		Role:       row.Role,
		TenantID:   row.TenantID,
		CreatedAt:  row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetAllUsers(ctx context.Context, limit, offset int) ([]models.User, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetAllUsers(ctx, dbgen.GetAllUsersParams{
		TenantID: getTenantID(ctx),
		Limit:  int32(lim),
		Offset: int32(off),
	})
	if err != nil {
		return nil, err
	}

	users := make([]models.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, models.User{
			ID:        row.ID,
			Email:     row.Email,
			Role:      row.Role,
			IsActive:  row.IsActive,
			CreatedAt: row.CreatedAt.Time,
		})
	}
	return users, nil
}

func (r *DBRepository) UpdateUserActiveStatus(ctx context.Context, id uuid.UUID, isActive bool) error {
	rowsAffected, err := r.queries.UpdateUserActiveStatus(ctx, dbgen.UpdateUserActiveStatusParams{
		TenantID: getTenantID(ctx),
		ID:       id,
		IsActive: isActive,
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Doctor Related Methods
func (r *DBRepository) CreateDoctor(ctx context.Context, firstName, lastName, email, hashedPassword, specialty string, experience int) (uuid.UUID, error) {
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	newID, err := qtx.CreateDoctorUser(ctx, dbgen.CreateDoctorUserParams{
		TenantID: getTenantID(ctx),
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
	row, err := r.queries.GetDoctorByEmail(ctx, dbgen.GetDoctorByEmailParams{
		Email: email,
		TenantID: getTenantID(ctx),
	})
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
		Role:           row.Role,
		TenantID:       row.TenantID,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetDoctorByID(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
	row, err := r.queries.GetDoctorByID(ctx, dbgen.GetDoctorByIDParams{
		ID: id,
		TenantID: getTenantID(ctx),
	})
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
		Role:       row.Role,
		TenantID:   row.TenantID,
		CreatedAt:  row.CreatedAt.Time,
	}, nil
}

func (r *DBRepository) GetDoctors(ctx context.Context, limit, offset int) ([]models.Doctor, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetDoctors(ctx, dbgen.GetDoctorsParams{
		TenantID: getTenantID(ctx),
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
			Role:       row.Role,
			TenantID:   row.TenantID,
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
	resID, err := r.queries.CreateAppointment(ctx, dbgen.CreateAppointmentParams{
		TenantID: getTenantID(ctx),
		ID:              id,
		PatientID:       patientID,
		DoctorID:        doctorID,
		StartTime:       pgtype.Timestamptz{Time: startTime, Valid: true},
		EndTime:         pgtype.Timestamptz{Time: endTime, Valid: true},
		AppointmentType: models.AppointmentType(apptType),
		MeetingLink:     pgtype.Text{String: meetingLink, Valid: meetingLink != ""},
		MeetingID:       pgtype.Text{String: meetingID, Valid: meetingID != ""},
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return uuid.Nil, ErrDoubleBooking
		}
		return uuid.Nil, err
	}
	return resID, nil
}

func (r *DBRepository) UpdateAppointmentMeetingRoom(ctx context.Context, apptID uuid.UUID, meetingLink, meetingID string) error {
	_, err := r.queries.UpdateAppointmentMeetingRoom(ctx, dbgen.UpdateAppointmentMeetingRoomParams{
		TenantID: getTenantID(ctx),
		MeetingLink: pgtype.Text{String: meetingLink, Valid: meetingLink != ""},
		MeetingID:   pgtype.Text{String: meetingID, Valid: meetingID != ""},
		ID:          apptID,
	})
	return err
}

func (r *DBRepository) GetOrGenerateAppointmentMeetingRoom(ctx context.Context, apptID uuid.UUID, generator func() (meetingLink string, meetingID string, err error)) (models.Appointment, error) {
	appt, err := r.GetAppointmentByID(ctx, apptID)
	if err != nil {
		return models.Appointment{}, err
	}

	// If meeting link is already populated, or no generator provided, return immediately
	if appt.MeetingLink != "" || generator == nil {
		return appt, nil
	}

	type roomResult struct {
		link string
		id   string
	}

	res, genErr, _ := r.roomGroup.Do(apptID.String(), func() (any, error) {
		// Re-check DB in case another process/goroutine already created it
		curAppt, err := r.GetAppointmentByID(ctx, apptID)
		if err == nil && curAppt.MeetingLink != "" {
			return roomResult{link: curAppt.MeetingLink, id: curAppt.MeetingID}, nil
		}

		// Generate external meeting room link OUTSIDE any database transaction or row lock
		newLink, newID, err := generator()
		if err != nil {
			return roomResult{}, err
		}

		if newLink != "" {
			// Atomic conditional update: only update if still unpopulated
			rowsAffected, err := r.queries.UpdateAppointmentMeetingRoom(ctx, dbgen.UpdateAppointmentMeetingRoomParams{
		TenantID: getTenantID(ctx),
				MeetingLink: pgtype.Text{String: newLink, Valid: true},
				MeetingID:   pgtype.Text{String: newID, Valid: newID != ""},
				ID:          apptID,
			})
			if err != nil {
				return roomResult{}, err
			}
			if rowsAffected == 0 {
				curAppt, err := r.GetAppointmentByID(ctx, apptID)
				if err == nil && curAppt.MeetingLink != "" {
					return roomResult{link: curAppt.MeetingLink, id: curAppt.MeetingID}, nil
				}
			}
		}

		return roomResult{link: newLink, id: newID}, nil
	})

	if genErr != nil {
		return appt, genErr
	}

	rRes := res.(roomResult)
	if rRes.link != "" {
		appt.MeetingLink = rRes.link
		appt.MeetingID = rRes.id
	}

	return appt, nil
}

func (r *DBRepository) GetAppointmentByID(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
	row, err := r.queries.GetAppointmentByID(ctx, dbgen.GetAppointmentByIDParams{
		ID: id,
		TenantID: getTenantID(ctx),
	})
	if err != nil {
		return models.Appointment{}, err
	}
	return models.Appointment{
		ID:              row.ID,
		PatientID:       row.PatientID,
		DoctorID:        row.DoctorID,
		StartTime:       row.StartTime.Time,
		EndTime:         row.EndTime.Time,
		Status:          row.Status,
		Type:            row.AppointmentType,
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
		TenantID: getTenantID(ctx),
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
			Status:    row.Status,
		})
	}
	return appts, nil
}

func (r *DBRepository) GetAppointmentsByDoctorID(ctx context.Context, doctorID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetAppointmentsByDoctorID(ctx, dbgen.GetAppointmentsByDoctorIDParams{
		TenantID: getTenantID(ctx),
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
			Status:      row.Status,
			Type:        row.AppointmentType,
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
		TenantID: getTenantID(ctx),
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
			Status:          row.Status,
			Type:            row.AppointmentType,
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
		TenantID: getTenantID(ctx),
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
			Status:          row.Status,
			Type:            row.AppointmentType,
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
		TenantID: getTenantID(ctx),
		ID:       appointmentID,
		DoctorID: doctorID,
	})
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func (r *DBRepository) CancelAppointmentByParticipant(ctx context.Context, appointmentID, participantID uuid.UUID) (bool, error) {
	rowsAffected, err := r.queries.CancelAppointmentByParticipant(ctx, dbgen.CancelAppointmentByParticipantParams{
		TenantID: getTenantID(ctx),
		ID:        appointmentID,
		PatientID: participantID,
	})
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

// Prescription Methods: Dual-Mode, Atomic Enqueue, and Review

func (r *DBRepository) CreateUploadedPrescriptionWithJob(ctx context.Context, patientID, doctorID uuid.UUID, fileName, notes string, ocrEnabled bool) (uuid.UUID, string, error) {
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return uuid.Nil, "", err
	}
	defer tx.Rollback(ctx)

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
		TenantID: getTenantID(ctx),
		PatientID: patientID,
		DoctorID:  doctorID,
		Status:    models.PrescriptionStatus(status),
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
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var exp pgtype.Timestamptz
	if len(items) > 0 {
		expTime := calculatePrescriptionExpiry(items)
		exp = pgtype.Timestamptz{Time: expTime, Valid: !expTime.IsZero()}
	}

	newID, err := qtx.CreateDigitalPrescription(ctx, dbgen.CreateDigitalPrescriptionParams{
		TenantID: getTenantID(ctx),
		PatientID: patientID,
		DoctorID:  doctorID,
		Notes:     pgtype.Text{String: notes, Valid: notes != ""},
		ExpiresAt: exp,
	})
	if err != nil {
		return uuid.Nil, err
	}

	for _, item := range items {
		_, err := qtx.InsertPrescriptionItem(ctx, dbgen.InsertPrescriptionItemParams{
		TenantID: getTenantID(ctx),
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
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rowsAffected, err := qtx.UpdatePrescriptionOCRStatus(ctx, dbgen.UpdatePrescriptionOCRStatusParams{
		TenantID: getTenantID(ctx),
		ID:          prescriptionID,
		Status:      models.PrescriptionStatus(status),
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
		if err := qtx.DeletePrescriptionItems(ctx, dbgen.DeletePrescriptionItemsParams{
		PrescriptionID: prescriptionID,
		TenantID: getTenantID(ctx),
	}); err != nil {
			return err
		}
		for _, item := range items {
			_, err := qtx.InsertPrescriptionItem(ctx, dbgen.InsertPrescriptionItemParams{
		TenantID: getTenantID(ctx),
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
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	rowsAffected, err := qtx.VerifyPrescription(ctx, dbgen.VerifyPrescriptionParams{
		TenantID: getTenantID(ctx),
		ID:       prescriptionID,
		Status:   models.PrescriptionStatus(status),
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
		if err := qtx.DeletePrescriptionItems(ctx, dbgen.DeletePrescriptionItemsParams{
		PrescriptionID: prescriptionID,
		TenantID: getTenantID(ctx),
	}); err != nil {
			return false, err
		}
		for _, item := range items {
			_, err := qtx.InsertPrescriptionItem(ctx, dbgen.InsertPrescriptionItemParams{
		TenantID: getTenantID(ctx),
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

	if status == "approved" {
		var itemsForExpiry []models.PrescriptionItem
		if items != nil {
			itemsForExpiry = items
		} else {
			existingItems, err := qtx.GetPrescriptionItemsByPrescriptionID(ctx, dbgen.GetPrescriptionItemsByPrescriptionIDParams{
		PrescriptionID: prescriptionID,
		TenantID: getTenantID(ctx),
	})
			if err == nil && len(existingItems) > 0 {
				itemsForExpiry = make([]models.PrescriptionItem, len(existingItems))
				for i, it := range existingItems {
					itemsForExpiry[i] = models.PrescriptionItem{
						Duration: it.Duration.String,
					}
				}
			}
		}
		var exp time.Time
		if len(itemsForExpiry) > 0 {
			exp = calculatePrescriptionExpiry(itemsForExpiry)
		}
		if exp.IsZero() {
			exp = time.Now().Add(30 * 24 * time.Hour)
		}
		if err := qtx.UpdatePrescriptionExpiry(ctx, dbgen.UpdatePrescriptionExpiryParams{
		TenantID: getTenantID(ctx),
			ExpiresAt: pgtype.Timestamptz{Time: exp, Valid: true},
			ID:        prescriptionID,
		}); err != nil {
			return false, fmt.Errorf("failed to update prescription expiry: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	return true, nil
}

func (r *DBRepository) fetchPrescriptionItems(ctx context.Context, prescriptionID uuid.UUID) ([]models.PrescriptionItem, error) {
	items, err := r.queries.GetPrescriptionItemsByPrescriptionID(ctx, dbgen.GetPrescriptionItemsByPrescriptionIDParams{
		PrescriptionID: prescriptionID,
		TenantID: getTenantID(ctx),
	})
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

	rows, err := r.queries.GetPrescriptionItemsByPrescriptionIDs(ctx, dbgen.GetPrescriptionItemsByPrescriptionIDsParams{
		PrescriptionIds: prescriptionIDs,
		TenantID: getTenantID(ctx),
	})
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
		FileName: pgtype.Text{String: filename, Valid: true},
		DoctorID: doctorID,
	})
	if err != nil {
		return models.Prescription{}, err
	}
	return models.Prescription{
		ID:        row.ID,
		Status:    row.Status,
		PatientID: row.PatientID,
	}, nil
}

func (r *DBRepository) GetPrescriptionByID(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
	row, err := r.queries.GetPrescriptionByID(ctx, dbgen.GetPrescriptionByIDParams{
		ID: id,
		TenantID: getTenantID(ctx),
	})
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
	err := r.queries.UpdatePrescriptionFileName(ctx, dbgen.UpdatePrescriptionFileNameParams{
		TenantID: getTenantID(ctx),
		FileName: pgtype.Text{String: fileName, Valid: fileName != ""},
		ID:       prescriptionID,
	})
	if err != nil {
		return fmt.Errorf("failed to update prescription file_name: %w", err)
	}
	return nil
}

func (r *DBRepository) CheckPrescriptionFileNameExists(ctx context.Context, fileName string) (bool, error) {
	return r.queries.CheckPrescriptionFileNameExists(ctx, dbgen.CheckPrescriptionFileNameExistsParams{
		FileName: pgtype.Text{String: fileName, Valid: fileName != ""},
		TenantID: getTenantID(ctx),
	})
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
		TenantID: getTenantID(ctx),
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
	rows, err := r.queries.GetDoctorSchedules(ctx, dbgen.GetDoctorSchedulesParams{
		DoctorID: doctorID,
		TenantID: getTenantID(ctx),
	})
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
		TenantID: getTenantID(ctx),
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
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin schedule transaction: %w", err)
	}
	defer tx.Rollback(ctx)

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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
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
	row, err := r.queries.GetLatestPatientVital(ctx, dbgen.GetLatestPatientVitalParams{
		PatientID: patientID,
		TenantID: getTenantID(ctx),
	})
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
		ID:        itemID,
		PatientID: patientID,
	})
}

func (r *DBRepository) HasDoctorPatientRelationship(ctx context.Context, doctorID, patientID uuid.UUID) (bool, error) {
	res, err := r.queries.HasDoctorPatientRelationship(ctx, dbgen.HasDoctorPatientRelationshipParams{
		TenantID: getTenantID(ctx),
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

// RotateRefreshToken executes token rotation atomically in a single Postgres transaction:
//  1. Selects the old token row FOR UPDATE (prevents concurrent rotation races).
//  2. If the old token is already revoked and has a replacement, fetches and returns
//     the existing replacement token so mobile clients that dropped the first response
//     can recover without being permanently locked out.
//  3. Otherwise, creates the new token and revokes the old one in the same transaction.
func (r *DBRepository) RotateRefreshToken(ctx context.Context, oldTokenID, userID uuid.UUID, newHash string, expiresAt time.Time) (models.RefreshToken, error) {
	tx, qtx, err := r.beginTxWithRLS(ctx)
	if err != nil {
		return models.RefreshToken{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Lock the old token row to serialise concurrent refresh attempts, scoped strictly to the authenticated user.
	oldToken, err := qtx.LockRefreshTokenForRotation(ctx, dbgen.LockRefreshTokenForRotationParams{
		ID:     oldTokenID,
		UserID: userID,
	})
	if err != nil {
		return models.RefreshToken{}, err
	}

	// Grace-window retry: the old token was already rotated recently (within 10s).
	// This happens when a client retries due to a dropped response (common on mobile).
	// Correct behaviour: return the EXISTING unconsumed replacement token instead of
	// minting a third token and voiding the second one. Voiding the replacement was
	// the root cause of false-positive theft-detection on legitimate concurrent retries.
	if oldToken.RevokedAt.Valid {
		if time.Since(oldToken.RevokedAt.Time) <= 10*time.Second && oldToken.ReplacedByTokenID.Valid {
			replacementID := uuid.UUID(oldToken.ReplacedByTokenID.Bytes)
			rep, scanErr := qtx.LockRefreshTokenForRotation(ctx, dbgen.LockRefreshTokenForRotationParams{
				ID:     replacementID,
				UserID: userID,
			})
			if scanErr == nil {
				// The replacement was already consumed (rotated onward) — this is a
				// replay of a truly-old ancestor token, so terminate all sessions.
				if rep.RevokedAt.Valid {
					return models.RefreshToken{}, ErrTokenAlreadyRotated
				}

				// Issue a new token for this retry so the HTTP handler can securely return the plaintext.
				newPgExpiry := pgtype.Timestamptz{Time: expiresAt, Valid: true}
				created, err := qtx.CreateRefreshToken(ctx, dbgen.CreateRefreshTokenParams{
					UserID:    userID,
					TokenHash: newHash,
					ExpiresAt: newPgExpiry,
				})
				if err != nil {
					return models.RefreshToken{}, err
				}

				// Void the abandoned replacement token (rep), but set replaced_by_token_id to the NEW token (created).
				// This preserves recursive lineage! If the client accidentally uses 'rep', it enters the grace window again.
				const voidRepQuery = `
					UPDATE refresh_tokens
					SET revoked_at = now(), replaced_by_token_id = $3
					WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`
				if _, err := tx.Exec(ctx, voidRepQuery, rep.ID, userID, pgtype.UUID{Bytes: created.ID, Valid: true}); err != nil {
					return models.RefreshToken{}, err
				}

				// Update the original oldToken's lineage to point to the newest issue.
				const updateOldQuery = `
					UPDATE refresh_tokens
					SET replaced_by_token_id = $2
					WHERE id = $1 AND user_id = $3`
				if _, err := tx.Exec(ctx, updateOldQuery, oldTokenID, pgtype.UUID{Bytes: created.ID, Valid: true}, userID); err != nil {
					return models.RefreshToken{}, err
				}

				if commitErr := tx.Commit(ctx); commitErr != nil {
					return models.RefreshToken{}, commitErr
				}
				return rawToRefreshToken(
					created.ID, created.UserID, created.TokenHash,
					created.ExpiresAt, created.RevokedAt, created.ReplacedByTokenID, created.CreatedAt,
				), nil
			}
			return models.RefreshToken{}, fmt.Errorf("replacement token not found for revoked token %s", oldTokenID)
		}
		// Revoked more than 10 seconds ago with no valid retry window — stolen token.
		return models.RefreshToken{}, ErrTokenAlreadyRotated
	}

	// Create the new token.
	newPgExpiry := pgtype.Timestamptz{Time: expiresAt, Valid: true}
	rep, err := qtx.CreateRefreshToken(ctx, dbgen.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: newHash,
		ExpiresAt: newPgExpiry,
	})
	if err != nil {
		return models.RefreshToken{}, err
	}

	// Revoke the old token, linking to the new one for lineage tracking.
	const revokeQuery = `
		UPDATE refresh_tokens
		SET revoked_at = now(), replaced_by_token_id = $2
		WHERE id = $1 AND user_id = $3 AND revoked_at IS NULL`
	if _, err := tx.Exec(ctx, revokeQuery, oldTokenID, pgtype.UUID{Bytes: rep.ID, Valid: true}, userID); err != nil {
		return models.RefreshToken{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.RefreshToken{}, err
	}

	return rawToRefreshToken(rep.ID, rep.UserID, rep.TokenHash,
		rep.ExpiresAt, rep.RevokedAt, rep.ReplacedByTokenID, rep.CreatedAt), nil
}

// rawToRefreshToken builds a models.RefreshToken from raw pgx scan targets.
func rawToRefreshToken(id, userID uuid.UUID, tokenHash string,
	expiresAt, revokedAt pgtype.Timestamptz, replacedBy pgtype.UUID, createdAt pgtype.Timestamptz,
) models.RefreshToken {
	var revAt *time.Time
	if revokedAt.Valid {
		t := revokedAt.Time
		revAt = &t
	}
	var repBy *uuid.UUID
	if replacedBy.Valid {
		u := uuid.UUID(replacedBy.Bytes)
		repBy = &u
	}
	return models.RefreshToken{
		ID:                id,
		UserID:            userID,
		TokenHash:         tokenHash,
		ExpiresAt:         expiresAt.Time,
		RevokedAt:         revAt,
		ReplacedByTokenID: repBy,
		CreatedAt:         createdAt.Time,
	}
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
		TenantID: getTenantID(ctx),
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
		TenantID: getTenantID(ctx),
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
			Metadata:     row.Metadata,
			CreatedAt:    row.CreatedAt.Time,
		})
	}
	return logs, nil
}

func (r *DBRepository) GetAuditLogs(ctx context.Context, limit, offset int) ([]models.PhiAuditLog, error) {
	lim, off := clampPagination(limit, offset)
	rows, err := r.queries.GetAuditLogs(ctx, dbgen.GetAuditLogsParams{
		TenantID: getTenantID(ctx),
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
			Metadata:     row.Metadata,
			CreatedAt:    row.CreatedAt.Time,
		})
	}
	return logs, nil
}


func (r *DBRepository) CreateOCRProviderConfig(ctx context.Context, arg dbgen.CreateOCRProviderConfigParams) (dbgen.OcrProviderConfig, error) {
	return r.queries.CreateOCRProviderConfig(ctx, arg)
}
func (r *DBRepository) GetOCRProviderConfigsForTenant(ctx context.Context, tenantID uuid.UUID) ([]dbgen.OcrProviderConfig, error) {
	return r.queries.GetOCRProviderConfigsForTenant(ctx, tenantID)
}
func (r *DBRepository) GetOCRProviderConfigByID(ctx context.Context, arg dbgen.GetOCRProviderConfigByIDParams) (dbgen.OcrProviderConfig, error) {
	return r.queries.GetOCRProviderConfigByID(ctx, arg)
}
func (r *DBRepository) DeleteOCRProviderConfig(ctx context.Context, arg dbgen.DeleteOCRProviderConfigParams) error {
	return r.queries.DeleteOCRProviderConfig(ctx, arg)
}
func (r *DBRepository) UpsertTenantSettings(ctx context.Context, arg dbgen.UpsertTenantSettingsParams) (dbgen.TenantSetting, error) {
	return r.queries.UpsertTenantSettings(ctx, arg)
}
func (r *DBRepository) GetTenantSettings(ctx context.Context, tenantID uuid.UUID) (dbgen.TenantSetting, error) {
	return r.queries.GetTenantSettings(ctx, tenantID)
}
func (r *DBRepository) GetOCRProviderConfigsByIDs(ctx context.Context, arg dbgen.GetOCRProviderConfigsByIDsParams) ([]dbgen.OcrProviderConfig, error) {
	return r.queries.GetOCRProviderConfigsByIDs(ctx, arg)
}

func (r *DBRepository) GetUserByIDGlobal(ctx context.Context, id uuid.UUID) (dbgen.GetUserByIDGlobalRow, error) {
	return r.queries.GetUserByIDGlobal(ctx, id)
}
