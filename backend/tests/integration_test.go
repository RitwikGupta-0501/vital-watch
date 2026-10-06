package tests

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/utils"
)

func TestLiveDB_MigrationsAndConstraints(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	// Verify table creation by querying pg_tables
	ctx := context.Background()
	var count int
	err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name IN ($2, $3, $4, $5, $6)",
		"public", "users", "admin_profiles", "doctor_profiles", "patient_profiles", "phi_audit_logs",
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query information_schema: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 key domain tables in public schema, got %d", count)
	}

	_ = repo
}

func TestLiveDB_AdminDecouplingAndQueries(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	hashedPassword, _ := utils.HashPassword("SecureAdminPass123!")

	// 1. Create Admin
	adminEmail := fmt.Sprintf("admin-%s@vitalwatch.org", uuid.New().String()[:8])
	adminID, err := repo.CreateAdmin(ctx, "Alice", "Admin", adminEmail, hashedPassword, "Compliance")
	if err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}
	if adminID == uuid.Nil {
		t.Fatalf("expected non-nil adminID")
	}

	// 2. Create Doctor
	doctorEmail := fmt.Sprintf("doctor-%s@vitalwatch.org", uuid.New().String()[:8])
	doctorID, err := repo.CreateDoctor(ctx, "Bob", "Doctor", doctorEmail, hashedPassword, "Cardiology", 10)
	if err != nil {
		t.Fatalf("failed to create doctor: %v", err)
	}

	// 3. Create Patient
	patientEmail := fmt.Sprintf("patient-%s@vitalwatch.org", uuid.New().String()[:8])
	patientID, err := repo.CreatePatient(ctx, "Charlie", "Patient", patientEmail, hashedPassword)
	if err != nil {
		t.Fatalf("failed to create patient: %v", err)
	}

	// 4. Verify Admin lookup succeeds
	admin, err := repo.GetAdminByEmail(ctx, adminEmail)
	if err != nil {
		t.Fatalf("failed to get admin by email: %v", err)
	}
	if admin.ID != adminID || admin.Department != "Compliance" || admin.Role != "admin" {
		t.Errorf("unexpected admin fields: %+v", admin)
	}

	adminByID, err := repo.GetAdminByID(ctx, adminID)
	if err != nil {
		t.Fatalf("failed to get admin by ID: %v", err)
	}
	if adminByID.Email != adminEmail {
		t.Errorf("expected email %s, got %s", adminEmail, adminByID.Email)
	}

	// 5. CRITICAL: Verify Doctor queries DO NOT return Admin (Role Isolation)
	_, err = repo.GetDoctorByEmail(ctx, adminEmail)
	if err == nil {
		t.Fatalf("expected GetDoctorByEmail for admin email to fail, but it succeeded")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected pgx.ErrNoRows for doctor lookup with admin email, got %v", err)
	}

	// 6. Test GetAllUsers pagination
	users, err := repo.GetAllUsers(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to GetAllUsers: %v", err)
	}
	if len(users) < 3 {
		t.Errorf("expected at least 3 users, got %d", len(users))
	}

	// 7. Test UpdateUserActiveStatus
	err = repo.UpdateUserActiveStatus(ctx, patientID, false)
	if err != nil {
		t.Fatalf("failed to update user active status: %v", err)
	}
	var isActive bool
	err = pool.QueryRow(ctx, "SELECT is_active FROM users WHERE id = $1", patientID).Scan(&isActive)
	if err != nil {
		t.Fatalf("failed to scan is_active: %v", err)
	}
	if isActive {
		t.Errorf("expected is_active to be false after deactivation")
	}

	_ = doctorID
}

func TestLiveDB_TelehealthRoomRowLocking_MutualExclusion(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	hashedPassword, _ := utils.HashPassword("TestPass123!")

	pID, err := repo.CreatePatient(ctx, "Jane", "Doe", fmt.Sprintf("p-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create patient: %v", err)
	}
	dID, err := repo.CreateDoctor(ctx, "John", "Smith", fmt.Sprintf("d-%s@vw.org", uuid.New().String()[:8]), hashedPassword, "General", 5)
	if err != nil {
		t.Fatalf("failed to create doctor: %v", err)
	}

	// Create virtual appointment with empty meeting link
	apptID := uuid.New()
	startTime := time.Now().Add(2 * time.Hour)
	endTime := startTime.Add(30 * time.Minute)
	_, err = repo.CreateAppointment(ctx, apptID, pID, dID, startTime, endTime, "virtual", "", "")
	if err != nil {
		t.Fatalf("failed to create appointment: %v", err)
	}

	// Concurrently call GetOrGenerateAppointmentMeetingRoom
	var generatorCalls int32
	const concurrency = 8
	var wg sync.WaitGroup
	results := make([]models.Appointment, concurrency)
	errs := make([]error, concurrency)

	generator := func() (string, string, error) {
		atomic.AddInt32(&generatorCalls, 1)
		time.Sleep(20 * time.Millisecond) // simulate external telehealth API latency
		roomID := "live-room-" + apptID.String()[:8]
		link := "https://telehealth.vitalwatch.local/" + roomID
		return link, roomID, nil
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			results[idx], errs[idx] = repo.GetOrGenerateAppointmentMeetingRoom(callCtx, apptID, generator)
		}(i)
	}

	wg.Wait()

	for i := 0; i < concurrency; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d failed: %v", i, errs[i])
		}
	}

	// CRITICAL: Generator MUST be invoked exactly ONCE across all concurrent participants
	if calls := atomic.LoadInt32(&generatorCalls); calls != 1 {
		t.Errorf("expected generator to be called exactly 1 time, but was called %d times", calls)
	}

	// All callers must receive the exact same room
	expectedLink := results[0].MeetingLink
	expectedID := results[0].MeetingID
	if expectedLink == "" || expectedID == "" {
		t.Fatalf("expected meeting link and ID to be populated, got empty")
	}

	for i := 1; i < concurrency; i++ {
		if results[i].MeetingLink != expectedLink {
			t.Errorf("goroutine %d received divergent meeting link: %s != %s", i, results[i].MeetingLink, expectedLink)
		}
		if results[i].MeetingID != expectedID {
			t.Errorf("goroutine %d received divergent meeting ID: %s != %s", i, results[i].MeetingID, expectedID)
		}
	}
}

func TestLiveDB_RefreshToken_GraceWindowRecovery(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	hashedPassword, _ := utils.HashPassword("TestPass123!")

	uID, err := repo.CreatePatient(ctx, "Sam", "Smith", fmt.Sprintf("u-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// 1. Create Token A
	tokenAHash := "hash-token-A-12345"
	tokenA, err := repo.CreateRefreshToken(ctx, uID, tokenAHash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("failed to create Token A: %v", err)
	}

	// 2. Rotate Token A -> Token B
	tokenBHash := "hash-token-B-67890"
	tokenB, err := repo.RotateRefreshToken(ctx, tokenA.ID, uID, tokenBHash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("failed to rotate Token A to Token B: %v", err)
	}
	if tokenB.ID == uuid.Nil || tokenB.TokenHash != tokenBHash {
		t.Fatalf("invalid Token B record: %+v", tokenB)
	}

	// Verify Token A is marked revoked and linked to Token B
	var aRevokedAt *time.Time
	var aReplacedBy uuid.UUID
	err = pool.QueryRow(ctx, "SELECT revoked_at, replaced_by_token_id FROM refresh_tokens WHERE id = $1", tokenA.ID).Scan(&aRevokedAt, &aReplacedBy)
	if err != nil || aRevokedAt == nil || aReplacedBy != tokenB.ID {
		t.Fatalf("Token A not properly revoked or linked to Token B: rev=%v, rep=%s", aRevokedAt, aReplacedBy)
	}

	// 3. RETRY within 10s: Client dropped connection, retries with Token A!
	// Token B has NOT been consumed (revoked_at IS NULL).
	// Server must void Token B, issue Token C, and commit successfully.
	tokenCHash := "hash-token-C-11111"
	tokenC, err := repo.RotateRefreshToken(ctx, tokenA.ID, uID, tokenCHash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("failed to recover dropped retry with Token A: %v", err)
	}
	if tokenC.ID == uuid.Nil || tokenC.TokenHash != tokenCHash {
		t.Fatalf("invalid Token C record: %+v", tokenC)
	}

	// Verify Token B was voided (revoked) with NULL replacement so it cannot be used
	var bRevokedAt *time.Time
	var bReplacedBy *uuid.UUID
	err = pool.QueryRow(ctx, "SELECT revoked_at, replaced_by_token_id FROM refresh_tokens WHERE id = $1", tokenB.ID).Scan(&bRevokedAt, &bReplacedBy)
	if err != nil || bRevokedAt == nil || bReplacedBy != nil {
		t.Fatalf("Token B not properly voided: rev=%v, rep=%v", bRevokedAt, bReplacedBy)
	}

	// Verify Token A now points to the new Token C
	err = pool.QueryRow(ctx, "SELECT replaced_by_token_id FROM refresh_tokens WHERE id = $1", tokenA.ID).Scan(&aReplacedBy)
	if err != nil || aReplacedBy != tokenC.ID {
		t.Fatalf("Token A not updated to point to Token C: %v", aReplacedBy)
	}

	// 4. REPLAY ATTACK: Attacker tries to use voided Token B
	_, err = repo.RotateRefreshToken(ctx, tokenB.ID, uID, "hash-attacker-222", time.Now().Add(24*time.Hour))
	if !errors.Is(err, repository.ErrTokenAlreadyRotated) {
		t.Errorf("expected ErrTokenAlreadyRotated for consumed Token B replay, got %v", err)
	}
}

func TestLiveDB_AuditLogImmutabilityTrigger(t *testing.T) {
	pool, _, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()

	// Insert a genuine audit log record
	logID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO phi_audit_logs (id, action, resource_type, ip_address, status_code)
		VALUES ($1, $2, $3, $4, $5)
	`, logID, "read_record", "patient", "127.0.0.1", 200)
	if err != nil {
		t.Fatalf("failed to insert audit log: %v", err)
	}

	// 1. Attempt UPDATE on phi_audit_logs -> must be rejected by PostgreSQL trigger
	_, err = pool.Exec(ctx, "UPDATE phi_audit_logs SET action = $1 WHERE id = $2", "tampered_action", logID)
	if err == nil {
		t.Fatalf("expected PostgreSQL trigger to block UPDATE on phi_audit_logs, but it succeeded")
	}

	// 2. Attempt DELETE on phi_audit_logs -> must be rejected by PostgreSQL trigger
	_, err = pool.Exec(ctx, "DELETE FROM phi_audit_logs WHERE id = $1", logID)
	if err == nil {
		t.Fatalf("expected PostgreSQL trigger to block DELETE on phi_audit_logs, but it succeeded")
	}
}

func TestLiveDB_GiSTDoubleBookingExclusion(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	hashedPassword, _ := utils.HashPassword("TestPass123!")

	dID, err := repo.CreateDoctor(ctx, "Gregory", "House", fmt.Sprintf("house-%s@vw.org", uuid.New().String()[:8]), hashedPassword, "Diagnostics", 15)
	if err != nil {
		t.Fatalf("failed to create doctor: %v", err)
	}

	p1ID, err := repo.CreatePatient(ctx, "Patient", "One", fmt.Sprintf("p1-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create patient 1: %v", err)
	}

	p2ID, err := repo.CreatePatient(ctx, "Patient", "Two", fmt.Sprintf("p2-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create patient 2: %v", err)
	}

	baseTime := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	slot1Start := baseTime
	slot1End := baseTime.Add(30 * time.Minute)

	// 1. First appointment for doctor in 09:00 - 09:30 -> succeeds
	appt1ID, err := repo.CreateAppointment(ctx, uuid.New(), p1ID, dID, slot1Start, slot1End, "in_person", "", "")
	if err != nil {
		t.Fatalf("failed to create appointment 1: %v", err)
	}

	// 2. Overlapping appointment for same doctor in 09:15 - 09:45 -> MUST fail with 23P01 (ExclusionViolation)
	slotOverlapStart := baseTime.Add(15 * time.Minute)
	slotOverlapEnd := baseTime.Add(45 * time.Minute)
	_, err = repo.CreateAppointment(ctx, uuid.New(), p2ID, dID, slotOverlapStart, slotOverlapEnd, "in_person", "", "")
	if err == nil {
		t.Fatalf("expected GiST exclusion error for overlapping appointment, but succeeded")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.ExclusionViolation {
		t.Fatalf("expected PostgreSQL exclusion violation error (23P01), got: %v", err)
	}

	// 3. Contiguous/adjacent appointment in 09:30 - 10:00 -> MUST succeed (boundary touching is not overlapping in tstzrange)
	slotAdjacentStart := slot1End
	slotAdjacentEnd := slot1End.Add(30 * time.Minute)
	_, err = repo.CreateAppointment(ctx, uuid.New(), p2ID, dID, slotAdjacentStart, slotAdjacentEnd, "in_person", "", "")
	if err != nil {
		t.Fatalf("expected adjacent appointment to succeed, got: %v", err)
	}

	// 4. Cancel slot 1: GiST exclusion constraint specifies WHERE (status != 'cancelled')
	_, err = pool.Exec(ctx, "UPDATE appointments SET status = 'cancelled' WHERE id = $1", appt1ID)
	if err != nil {
		t.Fatalf("failed to cancel appointment 1: %v", err)
	}

	// 5. Booking new appointment in the exact same slot (09:00 - 09:30) for doctor now succeeds!
	_, err = repo.CreateAppointment(ctx, uuid.New(), p2ID, dID, slot1Start, slot1End, "in_person", "", "")
	if err != nil {
		t.Fatalf("expected booking previously cancelled slot to succeed, got: %v", err)
	}
}

func TestLiveDB_VitalsConstraintValidation(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	hashedPassword, _ := utils.HashPassword("TestPass123!")

	dID, err := repo.CreateDoctor(ctx, "Lisa", "Cuddy", fmt.Sprintf("cuddy-%s@vw.org", uuid.New().String()[:8]), hashedPassword, "Endocrinology", 12)
	if err != nil {
		t.Fatalf("failed to create doctor: %v", err)
	}

	pID, err := repo.CreatePatient(ctx, "John", "Doe", fmt.Sprintf("p-vitals-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create patient: %v", err)
	}

	intPtr := func(i int) *int { return &i }
	floatPtr := func(f float64) *float64 { return &f }

	// 1. Systolic <= Diastolic check constraint violation
	_, err = repo.CreatePatientVital(ctx, models.PatientVital{
		PatientID:   pID,
		RecordedBy:  dID,
		SystolicBP:  intPtr(80),
		DiastolicBP: intPtr(120),
	})
	if err == nil {
		t.Fatalf("expected check violation for systolic <= diastolic, got nil")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.CheckViolation {
		t.Fatalf("expected CheckViolation (23514), got: %v", err)
	}

	// 2. Heart rate out of range (< 30)
	_, err = repo.CreatePatientVital(ctx, models.PatientVital{
		PatientID:  pID,
		RecordedBy: dID,
		HeartRate:  intPtr(20),
	})
	if err == nil || !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.CheckViolation {
		t.Fatalf("expected CheckViolation for heart_rate < 30, got: %v", err)
	}

	// 3. SpO2 out of range (> 100.0)
	_, err = repo.CreatePatientVital(ctx, models.PatientVital{
		PatientID:        pID,
		RecordedBy:       dID,
		OxygenSaturation: floatPtr(105.0),
	})
	if err == nil || !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.CheckViolation {
		t.Fatalf("expected CheckViolation for oxygen_saturation > 100.0, got: %v", err)
	}

	// 4. Temperature out of range (< 30.0)
	_, err = repo.CreatePatientVital(ctx, models.PatientVital{
		PatientID:   pID,
		RecordedBy:  dID,
		Temperature: floatPtr(25.0),
	})
	if err == nil || !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.CheckViolation {
		t.Fatalf("expected CheckViolation for temperature < 30.0, got: %v", err)
	}

	// 5. Valid vitals insertion succeeds
	vitalID, err := repo.CreatePatientVital(ctx, models.PatientVital{
		PatientID:        pID,
		RecordedBy:       dID,
		SystolicBP:       intPtr(120),
		DiastolicBP:      intPtr(80),
		HeartRate:        intPtr(72),
		OxygenSaturation: floatPtr(98.5),
		Temperature:      floatPtr(36.6),
		WeightKg:         floatPtr(70.5),
		Notes:            "Healthy checkup baseline",
	})
	if err != nil {
		t.Fatalf("expected valid vitals to be recorded successfully, got: %v", err)
	}
	if vitalID == uuid.Nil {
		t.Fatalf("expected non-nil vitalID")
	}
}

func TestLiveDB_ClinicalCascadeRestriction(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	hashedPassword, _ := utils.HashPassword("TestPass123!")

	dID, err := repo.CreateDoctor(ctx, "Robert", "Chase", fmt.Sprintf("chase-%s@vw.org", uuid.New().String()[:8]), hashedPassword, "Surgery", 8)
	if err != nil {
		t.Fatalf("failed to create doctor: %v", err)
	}

	pID, err := repo.CreatePatient(ctx, "Allison", "Cameron", fmt.Sprintf("cameron-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create patient: %v", err)
	}

	// Record clinical vitals
	intPtr := func(i int) *int { return &i }
	_, err = repo.CreatePatientVital(ctx, models.PatientVital{
		PatientID:   pID,
		RecordedBy:  dID,
		SystolicBP:  intPtr(115),
		DiastolicBP: intPtr(75),
	})
	if err != nil {
		t.Fatalf("failed to record vitals: %v", err)
	}

	// Record clinical appointment
	start := time.Now().Add(time.Hour)
	end := start.Add(30 * time.Minute)
	_, err = repo.CreateAppointment(ctx, uuid.New(), pID, dID, start, end, "in_person", "", "")
	if err != nil {
		t.Fatalf("failed to create appointment: %v", err)
	}

	// Record clinical prescription
	items := []models.PrescriptionItem{
		{
			MedicationName: "Amoxicillin",
			Dosage:         "500mg",
			Frequency:      "3x daily",
			Duration:       "10 days",
		},
	}
	_, err = repo.CreateDigitalPrescription(ctx, pID, dID, "Take after meals", items)
	if err != nil {
		t.Fatalf("failed to create prescription: %v", err)
	}

	// 1. Attempt hard DELETE of Doctor user -> MUST fail with ForeignKeyViolation (23503) due to ON DELETE RESTRICT
	_, err = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", dID)
	if err == nil {
		t.Fatalf("expected hard DELETE of doctor with clinical records to be restricted, but it succeeded")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.ForeignKeyViolation {
		t.Fatalf("expected ForeignKeyViolation (23503) on doctor delete, got: %v", err)
	}

	// 2. Attempt hard DELETE of Patient user -> MUST fail with ForeignKeyViolation (23503)
	_, err = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", pID)
	if err == nil {
		t.Fatalf("expected hard DELETE of patient with clinical records to be restricted, but it succeeded")
	}
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.ForeignKeyViolation {
		t.Fatalf("expected ForeignKeyViolation (23503) on patient delete, got: %v", err)
	}

	// 3. Soft deactivation is the compliant method to deactivate users
	err = repo.UpdateUserActiveStatus(ctx, dID, false)
	if err != nil {
		t.Fatalf("expected soft-deactivation to succeed, got: %v", err)
	}

	var isActive bool
	err = pool.QueryRow(ctx, "SELECT is_active FROM users WHERE id = $1", dID).Scan(&isActive)
	if err != nil || isActive {
		t.Fatalf("expected is_active to be false, got: is_active=%v, err=%v", isActive, err)
	}
}

func TestLiveDB_RefreshTokenUserScoping(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	hashedPassword, _ := utils.HashPassword("TestPass123!")

	user1ID, err := repo.CreatePatient(ctx, "User", "One", fmt.Sprintf("u1-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create user 1: %v", err)
	}

	user2ID, err := repo.CreatePatient(ctx, "User", "Two", fmt.Sprintf("u2-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create user 2: %v", err)
	}

	// Create Refresh Token for User 1
	token1Hash := "token-hash-1-abcdef"
	token1, err := repo.CreateRefreshToken(ctx, user1ID, token1Hash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("failed to create refresh token: %v", err)
	}

	// 1. Attempt rotation passing User 2's ID -> MUST be rejected because token belongs to User 1
	_, err = repo.RotateRefreshToken(ctx, token1.ID, user2ID, "token-hash-hijack-attempt", time.Now().Add(24*time.Hour))
	if err == nil {
		t.Fatalf("expected RotateRefreshToken with mismatched user_id to fail, but it succeeded")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got: %v", err)
	}

	// Verify Token 1 was NOT revoked or modified by the failed attempt
	var revokedAt *time.Time
	var replacedBy *uuid.UUID
	err = pool.QueryRow(ctx, "SELECT revoked_at, replaced_by_token_id FROM refresh_tokens WHERE id = $1", token1.ID).Scan(&revokedAt, &replacedBy)
	if err != nil {
		t.Fatalf("failed to query token status: %v", err)
	}
	if revokedAt != nil || replacedBy != nil {
		t.Fatalf("token was tampered with during unauthorized rotation attempt: rev=%v, rep=%v", revokedAt, replacedBy)
	}

	// 2. Rotate with correct user_id (User 1) -> succeeds
	token2Hash := "token-hash-2-legitimate"
	token2, err := repo.RotateRefreshToken(ctx, token1.ID, user1ID, token2Hash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("expected legitimate rotation with correct user_id to succeed, got: %v", err)
	}
	if token2.ID == uuid.Nil || token2.TokenHash != token2Hash {
		t.Fatalf("unexpected rotated token output: %+v", token2)
	}
}

func TestLiveDB_TelehealthDecoupledConcurrency(t *testing.T) {
	pool, repo, teardown := SetupTestDB(t)
	if pool == nil {
		return
	}
	defer teardown()

	ctx := context.Background()
	hashedPassword, _ := utils.HashPassword("TestPass123!")

	dID, err := repo.CreateDoctor(ctx, "Eric", "Foreman", fmt.Sprintf("foreman-%s@vw.org", uuid.New().String()[:8]), hashedPassword, "Neurology", 9)
	if err != nil {
		t.Fatalf("failed to create doctor: %v", err)
	}

	pID, err := repo.CreatePatient(ctx, "Patient", "Test", fmt.Sprintf("pt-%s@vw.org", uuid.New().String()[:8]), hashedPassword)
	if err != nil {
		t.Fatalf("failed to create patient: %v", err)
	}

	start := time.Now().Add(3 * time.Hour)
	end := start.Add(30 * time.Minute)
	apptID, err := repo.CreateAppointment(ctx, uuid.New(), pID, dID, start, end, "virtual", "", "")
	if err != nil {
		t.Fatalf("failed to create appointment: %v", err)
	}

	// Simulate slow external Daily.co API
	slowStarted := make(chan struct{})
	doneGen := make(chan struct{})
	slowGenerator := func() (string, string, error) {
		close(slowStarted)
		time.Sleep(300 * time.Millisecond) // slow external HTTP latency
		return "https://telehealth.vitalwatch.local/room-slow", "room-slow", nil
	}

	go func() {
		defer close(doneGen)
		_, _ = repo.GetOrGenerateAppointmentMeetingRoom(context.Background(), apptID, slowGenerator)
	}()

	// Wait until generator is confirmed in-flight
	<-slowStarted

	// Concurrently query database for this appointment
	// Because external generator is decoupled from DB transactions, this must complete immediately (< 100ms)
	queryStart := time.Now()
	queriedAppt, err := repo.GetAppointmentByID(ctx, apptID)
	queryDuration := time.Since(queryStart)

	if err != nil {
		t.Fatalf("concurrent GetAppointmentByID failed: %v", err)
	}
	if queriedAppt.ID != apptID {
		t.Fatalf("expected appointment ID %s, got %s", apptID, queriedAppt.ID)
	}
	if queryDuration > 150*time.Millisecond {
		t.Errorf("GetAppointmentByID took %v while room generator was running; expected < 150ms (proving no lock held)", queryDuration)
	}

	<-doneGen
}
