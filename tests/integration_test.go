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
	"github.com/jackc/pgx/v5"

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
