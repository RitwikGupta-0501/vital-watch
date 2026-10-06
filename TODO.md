# 📌 VitalWatch Project Tracker & Backlog

> **Status**: Active  
> **Last Updated**: October 2026  
> **Latest review**: Frontend design review (2026-10-05) added **Phase 6** — 30 findings from auditing the live API surface against `docs/frontend-page-inventory.md`, sequenced for remediation in `docs/implementation-plan.md`. Existing entries in Phases 1–5 were also corrected where their file references or status had drifted.  
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
  - *File*: ~~`migrations/000002_add_app_features.up.sql`~~ *(reference was stale — that file does not exist; the schema was consolidated into `migrations/000001_init_schema.up.sql`, which contains no seed `INSERT` statements)*

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
  - *Partial*: only `limit`/`offset` shipped — there is no cursor/keyset path on any list endpoint. Tracked as **API-14**.
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

## 🟢 Phase 3: Dual-Mode Prescriptions, Vision AI OCR & Clinical Intelligence (P2)

- [x] **RX-01: Dual-Mode Prescription Schema & Database Migration**
  - *Goal*: Refactor `prescriptions` to support both uploaded image slips and direct digital e-prescriptions. Drop `file_name NOT NULL`, add `source VARCHAR(20)` (`'uploaded'`, `'digital'`), lifecycle `status VARCHAR(20)` (`'pending_ocr'`, `'needs_review'`, `'approved'`, `'rejected'`), and structured fields: `dosage`, `frequency`, `duration`, `timing`, and `instructions`.
  - *File*: `migrations/`, `internal/repository/queries/prescriptions.sql`, `internal/models/`

- [x] **RX-02: Digital Native E-Prescribing Endpoint (Doctor-Only)**
  - *Goal*: Create `POST /api/prescriptions/digital` protected by `RequireRole("doctor")`. Allows physicians to directly create structured prescriptions during or after appointments without paper slips, pre-validating against `SAFE-01` and committing directly with `status = 'approved'`.
  - *File*: `internal/api/handlers.go`, `internal/repository/`

- [x] **RX-03: Automated Prescription PDF Generator**
  - *Goal*: Auto-generate standardized, clinic-branded downloadable PDF prescription slips with doctor credentials, clinic letterhead, structured medication tables, and a verification QR code for digital prescriptions, stored directly to S3.
  - *File*: `internal/pdf/`, `internal/api/handlers.go`

- [x] **OCR-01: River Background Task Queue Integration (PostgreSQL-Backed)**
  - *Goal*: Deploy [River](https://github.com/riverqueue/river) using the existing `jackc/pgx/v5` pool. Enforce transactional job enqueueing (insert uploaded prescription + schedule OCR job atomically in the same DB transaction to eliminate dual-write risks).
  - *File*: `internal/queue/`, `cmd/main/main.go` *(corrected 2026-10-05: there is no `cmd/worker/` — the River consumer runs in-process inside the API server, so scaling OCR throughput means scaling the API replicas)*

- [x] **OCR-02: Pluggable OCR Provider Interface & Gemini Vision AI Primary**
  - *Goal*: Create `internal/ocr/` with a clean `Provider` interface (`ExtractPrescription(ctx, fileBytes, mimeType)`). Implement Gemini Vision API (`google-genai` / structured JSON response schema) as primary provider extracting: `medication_name`, `dosage`, `frequency`, `duration`, `timing`, and `special_instructions`.
  - *File*: `internal/ocr/`

- [x] **OCR-03: Human-in-the-Loop (HITL) Verification Workflow**
  - *Goal*: Uploaded prescriptions follow a database-backed lifecycle state machine: `pending_ocr` ➔ `needs_review` ➔ `approved`. Provide authenticated doctor review endpoints (`GET /api/prescriptions/pending-review`, `PATCH /api/prescriptions/:id/verify`) to inspect, adjust, and approve AI extractions before committing them to active records.
  - *File*: `internal/api/handlers.go`, `internal/models/`, `internal/repository/`

- [x] **OCR-04: Resilient Provider Fallback Chain & BYOK (Bring Your Own Key) Vault** *(was still `[ ]`; verified complete 2026-10-05 — see notes below)*
  - *Goal*: 
    1. Composite `FallbackChain` decorator to failover across vision providers (e.g., Primary: Gemini ➔ Fallback: Claude / OpenAI) on 429 rate limits or transient 5xx outages.
    2. Clinic-level BYOK credential storage with AES-256-GCM envelope encryption (KMS master key) allowing healthcare providers to supply their own cloud API keys for HIPAA BAA compliance and direct cost accounting.
  - *File*: `internal/ocr/fallback.go`, `internal/crypto/`, `migrations/000012_ocr_configurations.up.sql`, `internal/api/admin_handlers.go`
  - *Verified*: `FallbackChain` failover across Gemini/Claude/OpenAI (`ocr/manager.go` reads `tenant_settings.ocr_fallback_chain`), keys sealed with AES-256-GCM and AAD bound to tenant+provider (`crypto/aes.go:54-72`), `KMS_MASTER_KEY` wired, and admin routes live at `main.go:565-567`. Residual gaps tracked as **OCR-06** (provider MIME asymmetry) and **ADMIN-01** (no way to rotate or delete a stored key).

- [x] **SAFE-01: Drug-Drug Interaction (DDI) & Allergy Checker**
  - *Goal*: Cross-reference medications (both digitally entered and OCR-extracted) against the patient's existing active medications and documented allergies using the [OpenFDA Drug API](https://open.fda.gov/apis/).
  - *File*: `internal/safety/`

- [x] **NOTIF-01: Real-Time Prescription Status Events**
  - *Goal*: Push WebSocket or Server-Sent Events (SSE) notification to the doctor's and patient's frontend when OCR analysis is complete or a new digital prescription is issued.
  - *File*: `internal/api/`

---

## 🟢 Phase 4: Advanced Scheduling & Patient Experience (P2)

- [x] **SCHED-01: Doctor Working Hours & Slot Generation**
  - *Goal*: Doctors define available days, working hours (e.g., 09:00–17:00), and slot durations (15m/30m). Backend dynamically returns unbooked slots.
  - *File*: `internal/models/`, `internal/repository/`

- [x] **SCHED-02: Telehealth Video Room Integration**
  - *Goal*: Auto-generate secure video meeting links (Daily.co / Twilio / Jitsi) for appointments with `type = 'virtual'`.
  - *File*: `internal/telehealth/`

- [x] **PAT-01: Longitudinal Vitals Tracking**
  - *Goal*: Add endpoints and models for logging blood pressure, heart rate, blood glucose, and body weight over time.
  - *File*: `internal/models/`, `internal/api/`

- [x] **PAT-02: Smart Medication Schedules & Reminders**
  - *Goal*: Transform approved prescription dosages into a daily patient schedule with push/email reminder hooks.
  - *File*: `internal/schedule/`

---

## 🔵 Phase 5: Testing, Compliance & Observability (P2 / P3)

- [x] **TEST-01: Core Unit Tests**
  - *Goal*: Test password hashing, JWT claims validation, and RBAC middleware (`*_test.go`).
  - *File*: `utils/utils_test.go`, `internal/api/auth_test.go`

- [x] **TEST-02: Integration Test Suite**
  - *Goal*: End-to-end API testing with `dockertest` or ephemeral PostgreSQL test containers.
  - *File*: `tests/`

- [x] **SEC-06: HIPAA PHI Access Audit Logging**
  - *Goal*: Middleware logging all reads/writes to medical history, prescriptions, and appointment records with user ID, IP address, and timestamp.
  - *File*: `internal/middleware/audit.go`

- [x] **SEC-09: Token Lifetime & Revocation Strategy**
  - *Goal*: Shorten access token lifetime (15-30m), implement refresh token rotation and revocation blocklists for sensitive health data access.
  - *File*: `internal/api/handlers.go`

- [x] **OBS-01: Structured JSON Logging (`slog`) & Request Tracing**
  - *Goal*: Replace standard `log.Println` with Go's `log/slog` and attach unique `X-Request-ID` to all logs and responses.
  - *File*: `cmd/main/main.go`, `internal/middleware/`

- [x] **CI-01: GitHub Actions Automation Pipeline**
  - *Goal*: Workflow executing `golangci-lint`, `govulncheck`, and `go test -v -race ./...` on every pull request.
  - *File*: `.github/workflows/ci.yml`

- [x] **DOCKER-01: Container Hardening**
  - *Goal*: Run Docker container as non-root user (`appuser`) and install `ca-certificates tzdata` for outbound HTTPS calls.
  - *File*: `Dockerfile`

---

## 🔍 Phase 6: Backend Findings from the Frontend Design Review (2026-10-05)

> **Source**: Auditing the live API surface against `docs/frontend-page-inventory.md`.
> **Severity** uses the legend above; *Blocks* lines are added where a finding gates specific frontend screens.
> 30 findings: 5 P0, 13 P1, 6 P2, 6 P3.
> **Remediation sequencing**: `docs/implementation-plan.md` §2 (Track A, PR-A1…PR-A5). AUTH-01 added and TENANT-01/OPS-03 reworded after the deeper code read that produced that plan.

### 🔴 P0 — Production Blockers

- [ ] **SEC-12: Admin identity is unreachable after the multi-tenancy migration**
  - *Issue*: `migrations/000013_multi_tenancy.up.sql:28` reinstates `users_role_check` as `('patient','doctor','platform_admin','tenant_admin')` and `:31` migrates existing `'admin'` rows out of that value, but `internal/repository/queries/admins.sql:3` still INSERTs the literal `'admin'` (check-constraint violation → 500 on register) and `:14`/`:20` filter `u.role = 'admin'` (login matches no rows → 401). With `AUTO_MIGRATE=true`, **no admin can be created or sign in on any deployed environment**. `Register` also accepts `tenant_admin`/`platform_admin` (`auth_handlers.go:153-191`) and silently discards the role, while `GetUserProfile` (`:536-546`) and `IsUserActive` (`handlers.go:82`) branch only on `"admin"`.
  - *Fix*: Choose one admin persona (`tenant_admin` recommended for a clinic product); align the insert and lookups with the constraint; add profile/active-check branches for the chosen roles; regression-test admin register → login → `/api/profile`.
  - *File*: `migrations/000013_multi_tenancy.up.sql`, `internal/repository/queries/admins.sql`, `internal/api/auth_handlers.go`, `internal/api/handlers.go`
  - *Blocks*: the entire clinic admin console (6 screens) and any operator console.

- [ ] **SEC-13: Logout bypasses authentication and rate limiting**
  - *Issue*: `cmd/main/main.go:501` registers `POST /api/auth/logout` with neither `authLimiter` nor `AuthMiddleware`, unlike its siblings at `:498-500`. Anyone can POST refresh tokens against it; the deliberately idempotent 200 makes it a cheap revocation-state oracle and an unthrottled write path.
  - *Fix*: Mount behind `h.AuthMiddleware()` and `authLimiter.Middleware()` like the other auth routes.
  - *File*: `cmd/main/main.go`, `internal/api/auth_handlers.go`

- [ ] **TENANT-01: No token has ever carried a real tenant**
  - *Issue*: broader than first recorded. `Login` does mint a `tenant_id` claim (`auth_handlers.go:291` — `user.GetTenantID().String()`), but `queries/admins.sql:10-20`, `queries/patients.sql`, and `queries/doctors.sql` never `SELECT u.tenant_id`, so `GetAdminByEmail`/`GetAdminByID` (`db.go:220-256`) and the patient/doctor equivalents return models with an **unset** `TenantID`. The claim is therefore always `00000000-0000-0000-0000-000000000000` — which *is* the System Default Tenant, so single-tenant testing masks it. The refresh path additionally omits the claim entirely (`:442-448`), and `parseTokenMiddleware` defaults the missing claim to `uuid.Nil` (`handlers.go:219-226`), which is numerically the same value.
  - *Also*: registration has no tenant source at all. `/api/register` sits outside `authGroup`, so `getTenantID(ctx)` (`db.go:33-46`) returns `uuid.Nil` for every signup, and `CreateAdmin` doesn't even pass the field (`db.go:191-194` omits `TenantID` from `CreateAdminUserParams`, while `CreatePatientUser:93-97` and `CreateDoctorUser:308-312` do). New users land in the default tenant by accident, not by policy.
  - *Fix*: `SELECT u.tenant_id` and populate the model in every identity query; emit `tenant_id` in the refresh claims; resolve the refresh lookup by user id **without** a tenant predicate (`auth_handlers.go:394-405`, since `getTenantID(ctx)` is `uuid.Nil` there and currently blocks a non-default-tenant user from refreshing at all); reject tokens with no `tenant_id` claim once every mint site emits one, and reference `models.SystemDefaultTenantID` by name instead of relying on the zero value.
  - *File*: `internal/repository/queries/{admins,patients,doctors}.sql`, `internal/repository/db.go`, `internal/api/auth_handlers.go`, `internal/api/handlers.go`

- [ ] **TENANT-02: Row-level security is enabled but never activated**
  - *Issue*: `000013:57-94` enables RLS and creates `tenant_isolation_policy` across 10 tables keyed on `current_setting('app.current_tenant')` and `app.is_platform_admin`. **No Go code sets either GUC** (zero non-test references anywhere in `internal/` or `cmd/`). Isolation in practice comes solely from the app-side `tenant_id = @tenant_id` predicate in sqlc queries, so a single missing predicate is an unmitigated cross-tenant read.
  - *Fix*: `SET LOCAL app.current_tenant` per request/transaction from the JWT claim — or drop the policies and stop implying a defense-in-depth layer that doesn't exist. Either way, make the actual boundary unambiguous before a security review reads it as RLS-protected PHI.
  - *File*: `migrations/000013_multi_tenancy.up.sql`, `internal/repository/db.go`, `internal/api/handlers.go`

- [ ] **SEC-14: `/metrics` is publicly reachable**
  - *Issue*: `main.go:494` exposes the Prometheus handler with no auth and no network allowlist; only the 120/min general limiter applies. Leaks route cardinality, request rates, and database pool telemetry.
  - *Fix*: Bind metrics to an internal-only listener or require a scrape credential / IP allowlist.

### 🟡 P1 — Data Integrity & Correctness

- [ ] **AUTH-01: The `role` claim comes from the request on login, from the DB on refresh**
  - *Issue*: `Login` mints the claim from request-supplied `req.Role` (`auth_handlers.go:290`) while `RefreshToken` mints it from the database row (`:444`, resolved at `:394-405`). The same account can therefore hold two different role strings across its tokens — and that string is what `IsUserActive` (`handlers.go:62-94`), `RequireRole`, `GetUserProfile` (`:517-547`), and `GetComplianceAuditLogs` all branch on. A client that sends a `role` the switch doesn't know reaches `default: active = false` (`handlers.go:85-86`) and 401s on every `authGroup` route. Invisible to CI: `admin_test.go:32` uses the package-level `AuthMiddleware(jwtSecret)` (`handlers.go:150-152`), which passes `checker = nil` and skips the active check entirely.
  - *Fix*: derive the claim from `user.GetRole()` on both paths — never from the request. Add a test asserting a login token and its refreshed successor carry identical `role` and `tenant_id` claims.
  - *File*: `internal/api/auth_handlers.go`, `internal/api/handlers.go`

- [ ] **DB-05: `doctor_profiles.available` has no write path**
  - *Issue*: Read by every doctor query (`queries/doctors.sql:11,17,23`) and used to gate booking (`appointment_handlers.go:99-188`), but no `UPDATE` of that column exists in any query or migration. It defaults to `true` at registration and neither the doctor nor an admin can change it — so the "not accepting patients" state is unmanageable while the booking flow depends on it.
  - *Fix*: Add `PATCH /api/doctor/availability` (plus an admin override) and surface it in the doctor settings screen.
  - *File*: `internal/repository/queries/doctors.sql`, `internal/api/appointment_handlers.go`

- [ ] **API-09: `DownloadPrescription` is mounted four times behind a polymorphic path param**
  - *Issue*: `main.go:514` (UUID), `:533` (patient filename), `:548` (doctor `:filename/download-url`), and `:549` — a bare alias whose own source comment reads `// alias`. One handler branches on whether the path segment parses as a UUID, so the same capability has four URLs with two input grammars.
  - *Fix*: Standardize on UUID-keyed routes, delete the alias, take filename as a query parameter if it must remain supported.
  - *File*: `cmd/main/main.go`, `internal/api/prescription_handlers.go`

- [ ] **API-10: Two sources of truth for what was prescribed**
  - *Issue*: The legacy `prescriptions.medication` column (made nullable in `migrations/000002_prescriptions_dual_mode.up.sql:8`) still exists and `POST /api/prescriptions` still accepts a `medication` field, while every structured path writes `prescription_items.medication_name`. Backfill logic in `000002:29-33` proves the duplication was transitional.
  - *Fix*: Stop writing the legacy column, omit it from responses, drop it in a follow-up migration.

- [ ] **API-11: No consistent response envelope**
  - *Issue*: Lists return `{data, limit, offset}` and errors `{error}`, but creates return `{id}`, `{id, status, …}`, `{data: …}`, or `{message}` depending on the handler. A generated API client has to special-case each resource.
  - *Fix*: Pick one shape per verb and document it before the frontend types are generated from the routes.

- [ ] **API-12: No profile editing or password change**
  - *Issue*: Only `GET /api/profile` exists (`main.go:512`). There is no `PATCH /api/profile`, no email change, and no password change — table stakes for every role, and a prerequisite for the settings screens on all three surfaces.
  - *Fix*: Add profile update and `POST /api/auth/change-password`, revoking all refresh tokens on password change (the revocation machinery already exists).

- [ ] **API-13: Doctor directory has no search or filtering**
  - *Issue*: `GET /api/doctors` returns a flat paginated list with no specialty, name, or availability parameters, so a patient must page through the whole directory to find a specialist.
  - *Fix*: Add `?specialty=`, `?q=`, `?available=` with an index on `specialty` before promising "find your doctor."

- [ ] **API-14: Offset-only pagination on append-only clinical tables**
  - *Issue*: API-03 promised `cursor`/`offset`; only `limit`/`offset` shipped. `phi_audit_logs`, `patient_vitals`, and `prescriptions` grow without bound, so deep offsets degrade — and the keyset indexes were already built for this in `migrations/000011_production_hardening.up.sql` (items 2–3).
  - *Fix*: Implement keyset pagination on `(recorded_at DESC, id DESC)` for the audit and vitals lists.

- [ ] **CODE-02: Dead authorization code makes the access model unreadable**
  - *Issue*: `RequirePlatformAdmin`/`RequireTenantAdmin` (`handlers.go:347-367`) guard zero production routes and are exercised only by `admin_test.go:690-693`; package-level `AuthMiddleware(jwtSecret)`/`SSEAuthMiddleware(jwtSecret)` (`:150-157`) duplicate the Handler methods. A reviewer cannot tell which authorization model is live.
  - *Fix*: Wire the tenant/platform guards to their intended route groups or delete both sets.

- [ ] **ADMIN-01: No rotation or removal for stored provider keys**
  - *Issue*: `ocr_provider_configs` supports create (`POST /api/admin/ocr-settings/configs`) and chain reorder only. A leaked or externally-revoked clinic API key cannot be deleted from the product, and orphaned configs remain eligible in the fallback chain. Keys are correctly non-readable, which makes rotation more important, not less.
  - *Fix*: Add delete/replace endpoints with an audit action; validate chain membership on delete.
  - *File*: `internal/api/admin_handlers.go`, `internal/repository/queries/ocr.sql`

- [ ] **RX-04: Prescription expiry is honored but never surfaced**
  - *Issue*: `queries/medications.sql:31` correctly excludes expired prescriptions from the medication schedule, but no list or detail response exposes `expires_at` and no endpoint filters on it. A patient's daily medication list can silently lose a drug with nothing in the UI explaining it.
  - *Fix*: Return `expires_at` on prescription detail, add "expired"/"expiring soon" states to the patient and doctor surfaces, and let doctors extend/refill.

- [ ] **OCR-05: OCR confidence is extracted, then discarded**
  - *Issue*: `ocr.ExtractedPrescription.Confidence` (`internal/ocr/ocr.go:26-30`) has no column, no field on `models.Prescription`, and no consumer anywhere in `queue.go` or the prescriptions queries. Roadmap §3.2's `flagged_unclear_words` was never implemented.
  - *Fix*: Persist confidence (ideally per item) or remove it from the provider contract. Until then the human-review UI has no signal to triage on and must review every line.

- [ ] **OCR-06: Providers accept different MIME types, so uploads can be legitimately unextractable**
  - *Issue*: Gemini and Claude accept PDF+PNG+JPEG+WebP (`gemini.go:65-69`, `claude.go:42-46`) but OpenAI rejects PDF (`openai.go:42-47`), and `FallbackChain` skips providers that don't support the type (`fallback.go:39`). The upload whitelist permits PDF regardless of the clinic's configured chain, so a PDF can correctly arrive in review with zero extracted items.
  - *Fix*: Expose accepted formats for the active chain via `GET /api/admin/ocr-settings` and validate PDF at upload time with a clear rejection when no provider supports it.

### 🟢 P2 — Product Gaps That Block Frontend Pages

- [ ] **SAFE-02: Allergy checking never receives allergies**
  - *Issue*: `prescription_handlers.go:525` passes `nil` for patient allergies, and `handlers.go:1032` still reads *"TODO: Pass patient documented allergies once allergy schema is added"*. `SafetyReport.AllergyAlerts` is fully implemented and tested but structurally can never fire, while the README and roadmap advertise allergy alerts as a headline safety feature.
  - *Fix*: Depends on PROF-01; then wire the array into `CheckPrescriptionSafety` and add a regression test that an allergic match blocks issuance.

- [ ] **PROF-01: Patient clinical profile is missing**
  - *Issue*: `patient_profiles` holds only `first_name`, `last_name`, `tenant_id` (`000001:27-31`). Roadmap §3.1 specifies `date_of_birth`, `gender`, `phone_number`, `allergies TEXT[]`, `emergency_contact JSONB`, and `license_number`/`is_verified`/`consultation_fee` on `doctor_profiles` — none of which were ever migrated.
  - *Fix*: One migration plus CRUD endpoints. Unblocks SAFE-02, the chart header, doctor vetting, and demographics-aware vitals interpretation (120 bpm means different things at age 8 and 78).

- [ ] **NOTIF-02: No reminder delivery despite advertised push/email**
  - *Issue*: PAT-02 and the README describe "push/email reminder hooks," but the only delivery mechanism in the codebase is SSE (`internal/notifications/`), with no scheduler, no email integration, and no web-push/VAPID code. Nothing reaches a closed app, so medication reminders do not exist in practice.
  - *Fix*: Either build delivery (transactional email at minimum) or correct the README and design the medication surface honestly as in-app-only.

- [ ] **NOTIF-03: Critical vitals never reach a clinician**
  - *Issue*: `patient_handlers.go:259-271` publishes `vital.alert` with `PatientID` set and **no `DoctorID`**, and `deliverLocal` fans out strictly per user id (`notifications.go:207-228`). `phase4_test.go:1414-1417` asserts the doctor receives nothing. A patient can log a hypertensive-crisis reading (≥180/≥120, with server-generated "seek emergency medical care" copy) and no clinician learns of it until they independently open the chart.
  - *Fix*: Confirm product intent. If clinicians should be alerted, publish to the linked doctor(s) and add a clinician alert surface; if the suppression is deliberate, record the rationale in an ADR — a patient-safety product shipping silent crisis values deserves a written decision, not an inferred one.

- [ ] **TENANT-03: Tenant IDs hardcoded to the system default**
  - *Issue*: `admin_handlers.go:201,252,286` and `queue.go:114` pin `models.SystemDefaultTenantID`, with `models/tenant.go:5-9` explicitly noting "before full multi-tenancy is introduced." BYOK provider configs are therefore not genuinely per-tenant.
  - *Fix*: Derive from the caller's token tenant once TENANT-01 lands.

- [ ] **TENANT-04: `tenants` and `tenant_invites` have no API**
  - *Issue*: Both tables exist (`000013:2-19`, including `tenants.status IN ('active','suspended')` and invite roles `tenant_admin`/`doctor`) with zero endpoints — no create, list, suspend, or redeem flow. Registration still gates doctors and admins on global env invite codes (`DOCTOR_INVITE_CODE`, `ADMIN_INVITE_CODE`), so onboarding a new clinic is a deploy-time configuration change.
  - *Fix*: Tenant CRUD plus invite redeem; replace the global env codes with per-tenant invite tokens.

### 🔵 P3 — Hygiene, Documentation & Compliance

- [ ] **OPS-03: Debug artifacts committed inside the module**
  - *Issue*: `prove_notify_bug.go` at the repo root is a second `package main` with its own `func main`, and `prove_refresh_bug.py` targets port 8000 while the app defaults to 8080 (`PORT=8080`); `changes.diff` sits untracked in the tree. *Correction after verification*: this is **not** a build hazard — `go build ./...` and `go vet ./...` both succeed today, since the root package and `cmd/main` are separate directories. It is dead weight and a stray binary target, nothing more.
  - *Fix*: delete, or move under `scripts/`.

- [ ] **OPS-04: A stale full copy of the tree is searchable**
  - *Issue*: `.kilo/worktrees/big-lip/` contains a second complete source tree. During this review, a grep for allergy logic resolved into that copy and reported a path that is not live code.
  - *Fix*: Exclude tool worktrees from search, or clean them out.

- [ ] **DOC-01: README and roadmap describe a different system than the one that exists**
  - *Issue*: The README endpoint catalog predates RBAC, refresh tokens, vitals, schedules, admin, and OCR routes, still documents integer-ID patient/doctor routes and streamed `/api/prescriptions/:filename` downloads, and cites port 8000. Roadmap §3.3 lists an `in_progress` appointment status absent from the real enum (`models.go:13-17`), and §3.1's target schema was never migrated (PROF-01). This is the document set a new frontend engineer reads first.
  - *Fix*: Regenerate the endpoint list from `setupRouter()`; label roadmap sections as historical vs current.

- [ ] **COMP-01: GDPR-style data rights are architecturally foreclosed**
  - *Issue*: The posture is deliberately HIPAA-styled — no consent capture, no erasure or export endpoint, no retention job. `migrations/000006` triggers block `UPDATE`/`DELETE`/`TRUNCATE` on `phi_audit_logs`, and `000008`/`000010` converted clinical FKs to `ON DELETE RESTRICT`, so "delete my data" cannot be implemented as a cascade without a retention-versus-erasure policy decision.
  - *Fix*: Decide policy, then implement pseudonymization and export; document the constraint before any EU/UK rollout.

- [ ] **AUD-01: Audit log completeness is not guaranteed**
  - *Issue*: The auditor writes asynchronously and offloads to a file DLQ (`audit_dlq.jsonl`, `main.go:339-344`) on database failure and queue overflow, so PHI-access records can be delayed or stranded by design. Nothing drains or alerts on that file.
  - *Fix*: Add DLQ replay plus an alerting metric, and constrain product/legal copy from claiming provably-complete access logging until then.

- [ ] **SEC-15: SSE bearer token travels in the query string**
  - *Issue*: `EventSource` cannot set headers, so `SSEAuthMiddleware` accepts `?token=<jwt>` (`patient_handlers.go:99-113`, `handlers.go:154-167`). Tokens land in reverse-proxy and server access logs, and the stream sits outside `authGroup`, so it inherits neither `SensitiveCacheControl` nor role gating.
  - *Fix*: Prefer cookie-authenticated SSE (`SameSite=strict`) or issue a short-lived single-use stream ticket; otherwise record the accepted risk explicitly.

## 🟣 Phase 7: Frontend-Driven Backend Adjustments (P2)

- [ ] **API-15: Expose in-flight prescriptions to patients safely**
  - *Issue*: Patients currently receive a `404 Not Found` for any prescription not explicitly `approved`. The product decision is to allow patients to see their `pending_ocr` and `needs_review` prescriptions (so they know the doctor is processing them), but they must absolutely not see any unverified AI-extracted medication items.
  - *Fix*: Update `ListPatientPrescriptions` and `GetPrescriptionByFilenameForPatient` to return prescriptions regardless of status (for the patient's own records). If the status is `pending_ocr` or `needs_review`, explicitly scrub/nil the `items` array in the response payload before serialization so unverified OCR data never reaches the patient UI.
  - *File*: `internal/api/prescription_handlers.go`, `internal/repository/db.go`

- [ ] **NOTIF-04: Emergency Contacts & Alert Routing Workflow (Roadmap)**
  - *Issue*: Currently, `vital.alert` only goes to the patient. We need a workflow allowing patients to opt-in to sending these crisis alerts to other users or non-user contacts (e.g., family members via SMS/email).
  - *Fix*: Introduce a schema for `patient_emergency_contacts` and routing rules. Update `deliverLocal` and the vitals evaluation logic to fan out alerts to the selected contacts when a crisis threshold is breached.

- [ ] **AUTH-02: Native httpOnly Cookie Support & CORS Credentials for Vite SPA**
  - *Issue*: Using a Vite + React SPA eliminates the Next.js server-side BFF. To maintain secure, XSS-resilient token storage via `httpOnly` cookies without a proxy layer, the Go backend must natively issue and parse cookies.
  - *Fix*:
    1. Update `POST /api/login` and `POST /api/auth/refresh` to emit `Set-Cookie` for `access_token` (`Path=/; HttpOnly; SameSite=Lax; Secure`) and `refresh_token` (`Path=/api/auth; HttpOnly; SameSite=Lax; Secure`).
    2. Update `POST /api/auth/logout` to clear both cookies (`Max-Age=-1`).
    3. Update `AuthMiddleware` and `SSEAuthMiddleware` to read from `c.Cookie("access_token")` when the `Authorization` header is not present (this also solves SEC-15 by allowing cookie-based SSE authentication without query parameter tokens).
    4. Enable `AllowCredentials: true` in the Gin CORS configuration (`main.go`) and add Vite's default dev origin `http://localhost:5173` to `CORS_ALLOWED_ORIGINS`.
  - *File*: `internal/api/auth_handlers.go`, `internal/api/handlers.go`, `cmd/main/main.go`
