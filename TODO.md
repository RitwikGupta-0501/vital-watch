# 📌 VitalWatch Project Tracker & Backlog

> **Status**: Active  
> **Last Updated**: September 2026  
> **Legend**:  
> 🔴 **P0 - Critical**: Security vulnerabilities, data loss risks, production blockers.  
> 🟡 **P1 - High**: Data integrity, architectural refactoring, performance, error leaks.  
> 🟢 **P2 - Medium**: New features (OCR pipeline, scheduling, safety checks).  
> 🔵 **P3 - Low**: Nice-to-haves, polish, automated tooling.

---

## 🔴 Phase 1: Critical Security & Production Blockers (P0)

- [x] **SEC-01: Fix Silent Empty JWT Secret Bug**
  - *Issue*: `var jwtSecret = []byte(os.Getenv("JWT_SECRET"))` in `handlers.go` evaluates before `godotenv.Load()` in `main.go`.
  - *Fix*: Pass `jwtSecret` explicitly via config struct or initialize inside `main.go` after `godotenv.Load()`. Enforce fatal termination if `JWT_SECRET` is empty.
  - *File*: `internal/api/handlers.go`, `cmd/main/main.go`

- [x] **SEC-02: Fix Broken Object-Level Authorization (BOLA/IDOR)**
  - *Issue*: Any authenticated user can mark any appointment as completed (`PATCH /api/appointments/:id`).
  - *Fix*: Verify that the caller is the specific assigned doctor for the appointment before updating (`UpdateAppointmentAsCompletedForDoctor`).
  - *File*: `internal/api/handlers.go` (`MarkAppointmentAsCompleted`), `internal/repository/db.go`

- [x] **SEC-03: Implement Strict Role-Based Access Control (RBAC) Middleware**
  - *Issue*: Auth middleware sets `role` in context, but handlers never verify it. Patients can call doctor-only routes and vice-versa.
  - *Fix*: Create role guard middlewares (`RequireRole("doctor")`, `RequireRole("patient")`) and apply them to respective route groups.
  - *File*: `internal/api/handlers.go`, `cmd/main/main.go`

- [x] **SEC-04: Prevent Colliding User IDs Across Doctors and Patients**
  - *Issue*: `patients` and `doctors` both use auto-incrementing integer IDs starting at 1. Patient #1 and Doctor #1 share ID `1`, enabling cross-role impersonation in ambiguous queries.
  - *Fix*: Migrate to unified `users` table with UUIDs, or enforce table-qualified lookup with strict role verification.
  - *File*: `migrations/`, `internal/models/models.go`, `internal/repository/db.go`

- [x] **SEC-05: Restrict Public Doctor Self-Registration**
  - *Issue*: Anyone can register with `"role": "doctor"` and immediately prescribe medications and access patient data.
  - *Fix*: Added invite token validation check for doctor registration.
  - *File*: `internal/api/handlers.go` (`Register`)

- [x] **SEC-07: Fail-Closed Doctor Invite Code Enforcement**
  - *Issue*: `h.DoctorInviteCode != ""` bypasses verification when `DOCTOR_INVITE_CODE` is unset or empty, permitting unauthorized doctor creation.
  - *Fix*: Fail-closed validation in `Register` (`h.DoctorInviteCode == "" || req.InviteCode != h.DoctorInviteCode`) and enforce mandatory configuration in `main.go`.
  - *File*: `internal/api/handlers.go`, `cmd/main/main.go`

- [x] **OPS-01: Fix PostgreSQL RAM Data Wipe in Docker Compose**
  - *Issue*: `docker-compose.yml` mounts Postgres data to `tmpfs: /var/lib/postgresql/data`. Any container restart wipes all database records.
  - *Fix*: Replace `tmpfs` with a persistent named Docker volume (`vital-watch-db-data`).
  - *File*: `docker-compose.yml`

- [x] **DB-01: Remove Hardcoded Test Credentials from Production Migration**
  - *Issue*: `000002_add_app_features.up.sql` inserts dummy doctors with plaintext/invalid hash `'dummyhash'`.
  - *Fix*: Separate migrations from seed data; remove dummy `INSERT` statements from migration files.
  - *File*: `migrations/000002_add_app_features.up.sql`

---

## 🟡 Phase 2: Data Integrity & Architectural Hardening (P1)

- [x] **ARCH-01: Unified Identity Schema Migration**
  - *Goal*: Consolidate `patients` and `doctors` into `users` (`id UUID`, `email`, `role`, `hashed_password`) + `patient_profiles` and `doctor_profiles`. Consolidated cleanly into `000001_init_schema.up.sql` for pre-launch greenfield setup.
  - *File*: `migrations/000001_init_schema.up.sql`, `internal/models/`

- [x] **DB-02: Double-Booking Prevention via Exclusion Constraints**
  - *Goal*: Enforce Postgres `EXCLUDE USING gist (doctor_id WITH =, tstzrange(start_time, end_time) WITH &&)` so no two appointments can overlap.
  - *File*: `migrations/`, `internal/repository/db.go`

- [x] **SEC-10: Verify Doctor Role in Appointment Creation**
  - *Issue*: `CreateAppointment` does not verify that `doctor_id` has `role = 'doctor'` (the foreign key in `appointments` only references `users(id)`). A patient could submit another patient's UUID as `doctor_id`.
  - *Fix*: Validate `doctor_id` is an active doctor in `doctor_profiles` or via `users.role = 'doctor'` before scheduling.
  - *File*: `internal/api/handlers.go`, `internal/repository/db.go`

- [x] **SEC-11: Authorize Prescribing Doctor on File Download**
  - *Issue*: `GetPrescriptionByFilenameForDoctor` strictly joins `appointments`. If a doctor created a prescription directly without an appointment record, the doctor who authored the prescription cannot download the file.
  - *Fix*: Update query to allow download if `p.doctor_id = $2 OR EXISTS (SELECT 1 FROM appointments a WHERE a.patient_id = p.patient_id AND a.doctor_id = $2)`.
  - *File*: `internal/repository/db.go`

- [x] **API-07: Handle Duplicate Email Conflict with HTTP 409**
  - *Issue*: When registration fails due to duplicate email (`users_email_key` unique violation), `Register` returns `500 Internal Server Error` instead of `409 Conflict`.
  - *Fix*: Catch unique constraint violations and return `409 Conflict: {"error": "An account with this email already exists"}`.
  - *File*: `internal/api/handlers.go`

- [x] **API-08: Add Timeout to S3 Rollback Goroutine**
  - *Issue*: S3 rollback in `CreatePrescription` runs in a background goroutine using untimed `context.Background()`, risking leaked/hanging goroutines on network issues.
  - *Fix*: Use `context.WithTimeout(context.Background(), 15*time.Second)` with `defer cancel()`.
  - *File*: `internal/api/handlers.go`

- [x] **CODE-01: Standardize Repository Parameter Ordering & Remove Dead Code**
  - *Issue*: Repository methods have inconsistent argument orders (e.g. `(patientID, doctorID)` vs `(doctorID, patientID)`), prone to transposed UUID bugs. `UpdateAppointmentAsCompleted` is dead code lacking ownership verification.
  - *Fix*: Standardize method parameter signatures and delete unused `UpdateAppointmentAsCompleted`.
  - *File*: `internal/repository/db.go`

- [x] **API-04: Appointment Input Validation**
  - *Goal*: Reject invalid appointment requests where `start_time <= now()`, `end_time <= start_time`, unrealistic durations, or invalid `appointment_type` enums.
  - *File*: `internal/api/handlers.go`

- [x] **S3-01: Replace S3 Backend Streaming with Direct Pre-Signed URLs**
  - *Issue*: Backend currently proxies files via `io.Copy(c.Writer, out.Body)`, burning Go memory and bandwidth.
  - *Fix*: Implement `POST /api/prescriptions/upload-url` and `GET /api/prescriptions/:id/download-url` returning short-lived (5 min) S3 Pre-signed URLs.
  - *File*: `internal/api/handlers.go`

- [x] **SEC-08: File Upload Sanitization & Stored XSS Prevention**
  - *Issue*: `CreatePrescription` accepts any file extension and content-type without validation, risking malicious uploads or Stored XSS.
  - *Fix*: Whitelist extensions (`.pdf`, `.jpg`, `.jpeg`, `.png`), validate MIME/magic bytes with `http.DetectContentType`, and sanitize storage filenames.
  - *File*: `internal/api/handlers.go`

- [x] **API-05: Context Propagation & Request Cancellation**
  - *Goal*: Propagate `c.Request.Context()` down to repository queries (`QueryContext`, `ExecContext`) and AWS SDK calls to cancel in-flight work when clients disconnect.
  - *File*: `internal/repository/db.go`, `internal/api/handlers.go`

- [x] **DB-03: Tune Database Connection Pooling**
  - *Goal*: Configure `SetMaxOpenConns(25)`, `SetMaxIdleConns(25)`, `SetConnMaxLifetime(5 * time.Minute)` on `sql.DB`.
  - *File*: `cmd/main/main.go`

- [x] **API-01: Sanitize Internal Error Leaks**
  - *Issue*: Returning `"err": err.Error()` in 500 responses leaks SQL schemas, S3 bucket names, and internal paths (specifically lines 280, 337, 395, 507, 527).
  - *Fix*: Log detailed error internally via structured logger; return standard safe messages (`{"error": "Internal server error"}`) to clients.
  - *File*: `internal/api/handlers.go`

- [x] **API-02: Dynamic CORS & Config Management**
  - *Goal*: Make CORS allowed origins dynamic via `CORS_ALLOWED_ORIGINS` env var instead of hardcoded CloudFront URL.
  - *File*: `cmd/main/main.go`, `.env.example`

- [x] **OPS-02: Implement Graceful Server Shutdown**
  - *Goal*: Replace `r.Run()` with `http.Server` and listen for `SIGINT`/`SIGTERM` to safely drain in-flight requests.
  - *File*: `cmd/main/main.go`

- [x] **DB-04: Add Missing Database Indexes**
  - *Goal*: Add B-tree indexes for `appointments(doctor_id, start_time)`, `appointments(patient_id)`, and `prescriptions(patient_id, file_name)`.
  - *File*: `migrations/`

- [x] **API-03: Add Pagination to List Endpoints**
  - *Goal*: Add `limit` and `cursor`/`offset` query parameters to `GetDoctors`, `GetAppointments`, and `GetPrescriptions`.
  - *File*: `internal/api/handlers.go`, `internal/repository/db.go`

- [x] **ARCH-02: Decouple Handlers via Repository Interface**
  - *Goal*: Introduce `type Querier interface` so handlers can be unit-tested using mocks without requiring a live PostgreSQL instance.
  - *File*: `internal/repository/`, `internal/api/handlers.go`

- [x] **API-06: Model JSON Key Standardization**
  - *Goal*: Standardize inconsistent `camelCase` keys (`doctorName`, `patientName`, `specialty`) to `snake_case` across models.
  - *File*: `internal/models/models.go`

- [x] **ARCH-03: Full Migration to jackc/pgx/v5 & Type-Safe sqlc Code Generation**
  - *Goal*: Eliminate legacy `lib/pq` driver across application runtime and migrations (`migrate/v4/database/pgx/v5`). Adopt `sqlc` for compile-time verified queries, automated scan mapping, and zero manual SQL parsing bugs.
  - *File*: `sqlc.yaml`, `internal/repository/queries/`, `internal/repository/dbgen/`, `internal/repository/db.go`, `cmd/main/main.go`

---

## 🟢 Phase 3: Prescription OCR & Clinical Intelligence (P2)

- [ ] **OCR-01: Background Task Queue Integration**
  - *Goal*: Deploy an asynchronous job queue ([River](https://github.com/riverqueue/river) via PostgreSQL or Redis [Asynq](https://github.com/hibiken/asynq)) for non-blocking prescription analysis.
  - *File*: `internal/queue/`, `cmd/worker/`

- [ ] **OCR-02: Multimodal Vision AI Prescription Parser**
  - *Goal*: Integrate Vision AI (Gemini Vision API / AWS Textract Medical) to analyze handwritten/typed prescription images.
  - *Output Schema*: Extract `medication_name`, `dosage`, `frequency`, `duration`, `timing`, and `special_instructions`.
  - *File*: `internal/ocr/`

- [ ] **OCR-03: Human-in-the-Loop (HITL) Verification Workflow**
  - *Goal*: Extracted prescriptions are saved as `status = 'needs_review'`. Provide doctor endpoint to review, edit, and approve extracted data.
  - *File*: `internal/api/handlers.go`, `internal/models/`

- [ ] **SAFE-01: Drug-Drug Interaction (DDI) & Allergy Checker**
  - *Goal*: Cross-reference extracted medication names against the patient's existing active medications and documented allergies using [OpenFDA Drug API](https://open.fda.gov/apis/).
  - *File*: `internal/safety/`

- [ ] **NOTIF-01: Real-Time OCR Completion Event**
  - *Goal*: Push WebSocket or Server-Sent Events (SSE) notification to the doctor's frontend when OCR analysis is complete.
  - *File*: `internal/api/`

---

## 🟢 Phase 4: Advanced Scheduling & Patient Experience (P2)

- [ ] **SCHED-01: Doctor Working Hours & Slot Generation**
  - *Goal*: Doctors define available days, working hours (e.g., 09:00–17:00), and slot durations (15m/30m). Backend dynamically returns unbooked slots.
  - *File*: `internal/models/`, `internal/repository/`

- [ ] **SCHED-02: Telehealth Video Room Integration**
  - *Goal*: Auto-generate secure video meeting links (Daily.co / Twilio / Jitsi) for appointments with `type = 'virtual'`.
  - *File*: `internal/telehealth/`

- [ ] **PAT-01: Longitudinal Vitals Tracking**
  - *Goal*: Add endpoints and models for logging blood pressure, heart rate, blood glucose, and body weight over time.
  - *File*: `internal/models/`, `internal/api/`

- [ ] **PAT-02: Smart Medication Schedules & Reminders**
  - *Goal*: Transform approved prescription dosages into a daily patient schedule with push/email reminder hooks.
  - *File*: `internal/schedule/`

---

## 🔵 Phase 5: Testing, Compliance & Observability (P2 / P3)

- [x] **TEST-01: Core Unit Tests**
  - *Goal*: Test password hashing, JWT claims validation, and RBAC middleware (`*_test.go`).
  - *File*: `utils/utils_test.go`, `internal/api/auth_test.go`

- [ ] **TEST-02: Integration Test Suite**
  - *Goal*: End-to-end API testing with `dockertest` or ephemeral PostgreSQL test containers.
  - *File*: `tests/`

- [ ] **SEC-06: HIPAA PHI Access Audit Logging**
  - *Goal*: Middleware logging all reads/writes to medical history, prescriptions, and appointment records with user ID, IP address, and timestamp.
  - *File*: `internal/middleware/audit.go`

- [ ] **SEC-09: Token Lifetime & Revocation Strategy**
  - *Goal*: Shorten access token lifetime (15-30m), implement refresh token rotation and revocation blocklists for sensitive health data access.
  - *File*: `internal/api/handlers.go`

- [ ] **OBS-01: Structured JSON Logging (`slog`) & Request Tracing**
  - *Goal*: Replace standard `log.Println` with Go's `log/slog` and attach unique `X-Request-ID` to all logs and responses.
  - *File*: `cmd/main/main.go`, `internal/middleware/`

- [ ] **CI-01: GitHub Actions Automation Pipeline**
  - *Goal*: Workflow executing `golangci-lint`, `govulncheck`, and `go test -v -race ./...` on every pull request.
  - *File*: `.github/workflows/ci.yml`

- [ ] **DOCKER-01: Container Hardening**
  - *Goal*: Run Docker container as non-root user (`appuser`) and install `ca-certificates tzdata` for outbound HTTPS calls.
  - *File*: `Dockerfile`
