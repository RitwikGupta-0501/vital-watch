# 🖥️ VitalWatch Frontend: Production Page Inventory & IA Map

> **Document Version**: 1.0.0
> **Target Audience**: Product, Design, Frontend Engineering
> **Status**: Approved for review — **implementation deferred**; no frontend code exists in this repo yet
> **Date compiled**: 2026-10-05
> **Backend commit range inspected**: `feature/phase-5-compliance-and-observability` (working tree, incl. uncommitted multi-tenancy work)

---

## Context

VitalWatch's Go backend is far ahead of its own documentation: JWT with rotating refresh tokens, RBAC, a River-backed async prescription OCR pipeline with human-in-the-loop review, OpenFDA drug-interaction checks, longitudinal vitals with crisis-threshold alerting, medication adherence logging, telehealth rooms, SSE realtime, HIPAA-styled PHI audit logging, an encrypted BYOK provider-key vault, and an in-flight multi-tenancy migration. **No frontend exists** — the README's diagram names a "Next.js / React Web Client" that has never been built.

This document decides what the production release actually ships, scoped to what the API supports *today*. The important finding is that several pages a generic health-app template would include are unbuildable against this backend, and one whole surface (admin) is currently unreachable due to a schema/code conflict. Shippable screens, blocked screens, and the backend prerequisite for each are separated below.

Verified against: `cmd/main/main.go:446-581` (routes + role guards), `internal/models/models.go` (enums), `migrations/000001-000013` (real schema), `internal/notifications/notifications.go:20-29`, `internal/safety/safety.go:21-51`, `internal/api/patient_handlers.go:255-338`, `internal/queue/queue.go:56-167`.

Line numbers are a snapshot and will drift — treat §10 as the list to re-verify against before building.

---

## 1. Stack and repository shape

- Next.js (App Router) + TypeScript + Tailwind, in a **separate repo**.
- Add the new origin to `CORS_ALLOWED_ORIGINS` (`main.go:467-489`, which currently defaults to `localhost:3000` plus a hardcoded CloudFront URL).
- Route Groups give one shared design system with clean per-role boundaries:

```
app/
├── (public)/         # landing, auth, QR verification
├── (patient)/        # role: patient
├── (doctor)/         # role: doctor
├── (admin)/          # role: admin personas — see §5, blocked today
└── api/              # optional BFF for token exchange + PHI-safe server reads
```

- **Data layer:** TanStack Query, `staleTime: 0` on PHI queries. The auth group sends `SensitiveCacheControlMiddleware()` no-store (`main.go:508`), so bfcache reuse can't be relied on; navigation must expect a refetch.
- **Auth:** 15-min access JWT (900s) + 7-day single-use rotating refresh token. Implement refresh behind a mutex: concurrent 401s must not fire parallel refreshes, because reuse of an already-rotated token (outside a 10s grace) revokes **all** sessions and writes a `TOKEN_REUSE_SECURITY_ALERT` audit row.
- **Realtime:** one `EventSource` on `/api/notifications/stream`, using the `?token=<jwt>` query fallback since EventSource can't set headers. Handle the 15s `: ping` heartbeat; reconnect with backoff; assume dropped events (the broker discards on slow consumers) and re-fetch on mount.

---

## 2. Status vocabulary — and who is allowed to see what

One shared constant file maps snake_case API values to labels. Never render a raw enum. Color never carries meaning alone — clinical UIs are a WCAG 2.1 AA obligation, and status chips pair color + icon + text.

| Domain | API values | Notes on visibility |
|---|---|---|
| Appointment | `upcoming` `completed` `cancelled` | `completed`/`cancelled` are terminal; only the assigned doctor can complete, and not before start time |
| Prescription | `pending_ocr` `needs_review` `approved` `rejected` | **Patients only ever see `approved`.** Anything else 404s for them — so the patient UI has one state, and the doctor review UI has four |
| Source | `uploaded` `digital` | Scanned slip / E-prescription |
| Med log | `pending` `taken` `skipped` | upsertable, see B10 |
| Time of day | `morning` `afternoon` `evening` `bedtime` `as_needed` | |
| Safety severity | `HIGH` `MODERATE` `LOW` | red / amber / grey |
| Role | `patient` `doctor` `admin` `tenant_admin` `platform_admin` | see §5 — admin personas are broken today |
| Tenant | `active` `suspended` | unreachable; no tenant API exists |

---

## 3. Page inventory

### A. Public surface — 7 screens

| # | Route | Purpose | Endpoints |
|---|---|---|---|
| A1 | `/` | Landing: OCR + drug-safety story, security posture | — |
| A2 | `/login` | Sign in | `POST /api/login`, `/api/auth/refresh`, `/api/auth/logout` |
| A3 | `/register` | Account creation | `POST /api/register` |
| A4 | `/verify/rx/[id]` | **QR prescription authenticity** page | `GET /verify/rx/:id` |
| A5 | `/notice-of-privacy-practices` | HIPAA NPP | — |
| A6 | `/terms` | Terms | — |
| A7 | `/security` | KMS/SSE, presigned TTL policy, immutable audit trail | — |

Plus system screens rather than pages: `403` role-gate, `404`, `429`, and a global SSE "reconnecting" banner.

Design notes:
- **A2's role selector is functional, not decorative.** `Login` requires `role` in the body and runs a separate lookup per role, rejecting a mismatch. "You picked the wrong account type" needs its own recovery message — a generic "invalid credentials" here will generate support tickets from real clinic staff.
- **A3 gating:** patients open; doctor and admin forms reveal an invite-code field (`DOCTOR_INVITE_CODE` / `ADMIN_INVITE_CODE`, fail-closed). Handle 409 email conflict inline. Note the admin path currently 500s — see §5.
- **A4 is the highest-leverage public screen** — the target of the QR printed on every generated PDF. Response is already a masked projection: `verified: true`, `masked_patient_id` (last chars of the UUID), `doctor_id`, items, notes with safety-override annotations stripped. Design it bare: no nav chrome, `noindex`, and a deliberate trust treatment (issue date, prescriber, medication table). The 10/min limiter means a clinic event scanning many copies will hit 429 — design that state.

### B. Patient portal — 12 screens

Shell: left nav on desktop, bottom tab bar on mobile (dose and vitals logging are phone-first jobs), notification bell on SSE, patient's name and clinic in the header.

| # | Route | Purpose | Endpoints |
|---|---|---|---|
| B1 | `/patient/dashboard` | Next appointment, doses due today, latest vitals, active alerts | `/patient/appointments`, `/patients/medication-schedule`, `/vitals`, `/patient/prescriptions` |
| B2 | `/patient/doctors` | Directory: name, specialty, experience | `GET /api/doctors?limit&offset` |
| B3 | `/patient/doctors/[id]` | Doctor detail + slot picker | `GET /api/doctors/:id/available-slots?date=YYYY-MM-DD` |
| B4 | `/patient/doctors/[id]/book` | Confirm time; choose `in_person` \| `virtual` | `POST /api/appointments` |
| B5 | `/patient/appointments` | Upcoming / Past / Cancelled; cancel | `GET /api/patient/appointments`, `PATCH /api/appointments/:id/cancel` |
| B6 | `/patient/appointments/[id]` | Visit detail, join button, add-to-calendar | detail + `/api/appointments/:id/meeting-room` |
| B7 | `/patient/call/[appointmentId]` | Telehealth room, device pre-check, waiting state | `GET /api/appointments/:id/meeting-room` |
| B8 | `/patient/prescriptions` | Issued prescription history (approved only) | `GET /api/patient/prescriptions` |
| B9 | `/patient/prescriptions/[id]` | Med list, prescriber, notes, PDF download, verify link | `GET /api/prescriptions/:id`, `.../download-url` |
| B10 | `/patient/medications` | **Today's adherence board** by time-of-day, tap taken/skip, week view | `GET /api/patients/medication-schedule?date=`, `POST .../medication-schedule/log` |
| B11 | `/patient/vitals` | Trends for BP, HR, glucose, SpO₂, temp, weight + manual entry with instant crisis feedback | `GET /api/vitals?start_date&end_date&limit&offset`, `POST /api/vitals` |
| B12 | `/patient/settings` | Identity, timezone, notification prefs, sign out all devices | `GET /api/profile`, `POST /api/auth/logout {all_devices:true}` |

Design constraints taken from the code:

- **B3's grid is server-constrained and the UI must say so.** Slots only exist on the doctor's `slot_duration` grid within their working hours, with a 15-minute lead time and a 1-year horizon. A doctor with no configured schedule returns an **empty list, not an error** — "not yet accepting online bookings" is a distinct empty state from "fully booked."
- **B4 handles 409 double-booking** (GiST exclusion constraints exist for *both* doctor and patient, `000009`), so a patient cannot hold two overlapping visits. Message: "this time just went — pick another," then refetch the grid. Also block the patient from booking against their own existing appointment before submission, client-side.
- **B7 has a hard access window:** room opens at start −15 min, closes at end +24 h (403/410 outside). Build an explicit "you can join from HH:MM" state — this is the most likely support call in the product.
- **B8 is a single-state list.** Patients cannot see `pending_ocr`/`needs_review` scripts at all. That is a defensible clinical choice, but it means "your doctor is preparing something" is invisible — raise it as a product decision (§7), don't paper over it in the UI.
- **B10 date bounds mirror the API:** log up to 1 day forward and 2 years back, `dose_number` ≤ 24, `taken` sets `taken_at`. Design the undo affordance around upsert semantics — re-tapping is an update, not a new record. **There are no push or email reminders anywhere in the system** despite the README advertising them; SSE only reaches an open tab, so B10's "reminder" framing must be honest (in-app today list, not notifications).
- **B11 is the safety-critical screen.** Crisis thresholds are computed server-side and returned on the create response (`is_critical`, `critical_alerts`): hypertensive crisis ≥180 or ≥120; hypotensive shock <80/<50; SpO₂ <90%; HR <40 or >140; glucose <54 or ≥350 mg/dL; temp ≥40°C or ≤35°C. Render `critical_alerts` **verbatim** — the backend already contains the emergency instructions ("seek emergency medical care", "ingest fast-acting carbohydrates"). Inventing softer copy here is a patient-safety risk. Mirror the input ranges client-side (SBP 50–300, DBP 30–200, HR 30–250, glucose 10–1000, SpO₂ 50–100, temp 30–45, weight 1–500, SBP strictly > DBP) so users get inline errors instead of a 400 round-trip.
- Vitals can be entered by the patient or recorded by staff; `recorder_name`/`recorder_role` are returned — render provenance in B11 rather than implying the patient logged everything.

### C. Doctor workspace — 13 screens

Shell: dense, keyboard-navigable, table-first. Clinicians have the highest screen time and least tolerance for clicks.

| # | Route | Purpose | Endpoints |
|---|---|---|---|
| C1 | `/doctor/today` | Today's timeline: slot, patient, type, complete/no-show actions | `GET /api/doctor/appointments`, `PATCH /api/appointments/:id` |
| C2 | `/doctor/appointments` | Full list + filters, cancel | same |
| C3 | `/doctor/patients` | Patient roster | `GET /api/doctor/patients` |
| C4 | `/doctor/patients/[id]` | **Patient chart**: vitals, visits, prescriptions, record-vitals | `/doctor/patients/:id/appointments`, `/:id/prescriptions?status=`, `GET /api/vitals?patient_id=` |
| C5 | `/doctor/patients/[id]/record-vitals` | Enter measurements at the consult | `POST /api/vitals {patient_id}` |
| C6 | `/doctor/patients/[id]/prescribe/new` | **Digital e-prescribe**: line items with dosage/frequency/duration/timing/instructions | `POST /api/prescriptions/digital` |
| C7 | (modal on C6 + C10) Safety interrupt | DDI soft-stop + override with typed reason | 409 `{safety_report, requires_override:true}` → resubmit with `override_safety`+`override_reason` |
| C8 | `/doctor/patients/[id]/prescribe/upload` | Scan a paper slip: 2-step presigned PUT | `POST /api/prescriptions/upload-url` → `PUT` bytes → `POST /api/prescriptions` |
| C9 | `/doctor/review` | **OCR review queue** | `GET /api/prescriptions/pending-review` |
| C10 | `/doctor/review/[id]` | **Split-screen HITL reviewer**: scan left, editable extracted fields right | `GET /api/prescriptions/:id/download-url`, `PATCH /api/prescriptions/:id/verify` |
| C11 | `/doctor/prescriptions` | Issued prescriptions, filter by status/source | per-patient history endpoints |
| C12 | `/doctor/settings/schedule` | Weekly hours: day on/off, start/end, slot ∈ {15,20,30,45,60}, timezone | `GET/PUT /api/doctor/schedules`, `DELETE /api/doctor/schedules/:day` |
| C13 | `/doctor/call/[appointmentId]` | Telehealth room (shared component with B7) | `GET /api/appointments/:id/meeting-room` |

Design constraints:

- **The OCR worker always lands at `needs_review` — success and every failure path alike** (`queue.go:81,95,103,119,126,159`), and permanent failures are recorded as bracketed error notes in the prescription notes. So C10 must handle an **empty medication list with a failure note** as a normal state, where the doctor builds the list by hand. `VerifyPrescription` explicitly refuses approval with zero items unless items are supplied (`prescription_handlers.go:777-779`).
- **Do not build a confidence meter.** OCR `confidence` is parsed from the model and then **discarded** — it is not persisted or exposed anywhere. `ocr_provider` *is* persisted, so show which engine produced the extraction and label the whole screen as unverified AI output requiring line-by-line confirmation (ADR-004). Per-field confidence, "unclear word" chips, and score-based triage sorting are all impossible today.
- **Two upload paths, one review queue.** Whether an upload enters `pending_ocr` or arrives already at `needs_review` depends on server OCR enablement (`OCR_ENABLED`, provider keys present — `main.go:327-332`, `queue.go:79-80`). Never assume a processing phase is visible.
- **Safety check is fail-closed.** If OpenFDA is unreachable the checker emits **HIGH severity and demands an override** (`safety.go:403-417`); `service_degraded`/`degraded_reason`/`unchecked_drugs` ride along. So C7 is a blocking modal, not an info banner, and its copy must say plainly that the interaction database could not be reached. Reuse the identical component on C6 and C10 — verify re-runs the same check and can also 409 (`prescription_handlers.go:785-798`). An override reason is appended to the prescription notes, so it becomes part of the record shown in B9 and A4 — write it accordingly.
- **The allergy half of `SafetyReport` can never fire**: allergies are passed `nil` (`prescription_handlers.go:525`) because no allergy column exists. Do not render an "Allergy alerts: none" empty state — it implies a check ran. Show only interaction alerts until §5's allergy schema lands.
- **C8 is presigned, never multipart.** `PUT` raw bytes to `upload_url`; 15 MB cap; pdf/jpg/jpeg/png whitelist; requires a prior appointment with that patient; duplicate filename → 409 (regenerate client-side). With `STORAGE_PROVIDER=local` the same flow hits `PUT /storage/upload` with HMAC query params. **Format nuance:** Gemini and Claude accept PDF+PNG+JPEG+WebP but **OpenAI rejects PDF** (`openai.go:42-47`) — so a PDF upload can fail extraction depending on the clinic's chain. Nudge image capture as the default, and make D5 state which formats each provider accepts.
- **C6 issues immediately:** digital prescriptions are created already `approved`, with a signed PDF + QR generated server-side, so "download / send to patient" appears right after 201, and A4 is its public twin.
- C4's doctor→patient access is relationship-gated (only patients they've actually seen), so the roster in C3 — not a global search — is the entry point to every chart. Design navigation around "my patients," never "search all patients."

### D. Clinic admin console — 6 screens (blocked, see §5)

| # | Route | Purpose | Endpoints |
|---|---|---|---|
| D1 | `/admin/users` | Cross-role directory; activate/deactivate | `GET /api/admin/users`, `PATCH /api/admin/users/:id/status` |
| D2 | `/admin/users/[id]` | Actor detail + their access trail | `/api/admin/audit-logs` filtered |
| D3 | `/admin/audit` | **PHI audit viewer**: action, resource type, patient, IP, status code, request id, date range | `GET /api/admin/audit-logs?limit&offset&patient_id` |
| D4 | `/admin/ocr-providers` | BYOK vault — add and list provider keys | `GET /api/admin/ocr-settings`, `POST /api/admin/ocr-settings/configs` |
| D5 | `/admin/ocr-chain` | Reorder provider fallback chain | `PUT /api/admin/ocr-settings/chain` |
| D6 | `/admin/settings` | Clinic identity + prescription-PDF branding | *no endpoint — §5* |

Design constraints:
- **D4 is write-only by design and must look that way.** Keys are AES-256-GCM envelope-encrypted with AAD bound to tenant+provider, and `GetOCRSettings` returns only `provider_name` and `created_at`. No reveal affordance; rotating means adding a new config. Warn before submit that the key cannot be retrieved.
- **D3 is the compliance artifact**, and it is the audit-the-audit path (`admin_handlers.go:164-178` logs admin audit access). The auditor is async with a file DLQ (`audit_dlq.jsonl`) on DB failure and overflow, so gaps are possible — never market the log as provably complete.
- **D1:** self-deactivation is blocked server-side; visibly disable the control on the actor's own row with a reason, and state that deactivation instantly revokes all their refresh tokens.

### E. Platform operator console — defer

Conceptually a tenant directory (create/suspend per `tenants.status`), per-tenant OCR cost/quota, cross-tenant audit, fleet health, and a doctor-vetting queue. Buildable today: nothing. There is no tenant CRUD or invite endpoint, `tenant_invites` is a dead table, and `RequirePlatformAdmin`/`RequireTenantAdmin` (`handlers.go:347-367`) are attached to zero routes. Omit from the release entirely.

---

## 4. Total count

7 public + 12 patient + 13 doctor = **32 shippable screens**, plus 6 admin screens gated on §5 fixes. Telehealth room, safety interrupt, and prescription detail are shared components reused across surfaces.

---

## 5. Blocked pages, and the backend work each needs

Do not schedule these; each is a template trap that looks easy and isn't.

| Tempting page | Why it can't ship | Prerequisite |
|---|---|---|
| **Entire admin console (D1–D6)** | Role vocabulary conflicts with itself. `000013:28` re-adds `users_role_check` allowing only `patient/doctor/platform_admin/tenant_admin`, and `000013:31` migrates existing `'admin'` rows away — but `CreateAdminUser` still inserts the literal `'admin'` (`queries/admins.sql:3`), so **admin registration violates the check constraint**, and `GetAdminByEmail/ByID` filter `role='admin'`, so **login finds no rows**. `Register` accepting `tenant_admin`/`platform_admin` never reaches the DB either, and `GetUserProfile` has no branch for them (400) | Pick one admin persona; align the insert and lookups with the constraint; add profile branches. Small fix, blocks 6 screens |
| Patient "who accessed my record" | `/api/compliance/audit-logs` is **403 for patients** — the handler restricts to doctor-with-relationship or admin | Allow self-scoped reads |
| Allergies & chronic conditions | `patient_profiles` holds only `first_name`, `last_name`, `tenant_id`. No allergies, DOB, gender, phone, or emergency contact. DDI's allergy half is hardwired to `nil` | Add columns + CRUD + wire into `CheckPrescriptionSafety` |
| Edit profile / change password | `GET /api/profile` only — no PATCH profile, no password change | Both are table-stakes product gaps, not UI gaps |
| Doctor license vetting | `doctor_profiles` has no `license_number`/`is_verified` (roadmap §3.1 proposed; never migrated) | Schema + endpoints |
| Tenant switcher / clinic branding / invites | `tenants` + `tenant_invites` exist with zero endpoints; RLS session GUCs are **never set** by Go, and admin OCR handlers pin to `SystemDefaultTenantID` | Tenant CRUD + invite redeem + set `app.current_tenant` |
| "Delete my data" / export | Posture is HIPAA-styled, not GDPR: no consent capture, no erasure or export endpoint, no retention job — and immutability triggers plus `ON DELETE RESTRICT` (`000006`, `000008`, `000010`) architecturally block erasure | Decide retention-vs-erasure policy first |
| Find-your-doctor search | `GET /api/doctors` has no search/filter params | Add query params before promising search |

**Live auth defect to know before testing:** the refreshed access JWT **omits the `tenant_id` claim** (`auth_handlers.go:442-448`) while `Login` includes it (`:291`). After the first 15-minute refresh, tenant context collapses to `uuid.Nil`, which *equals* the System Default Tenant — so it silently scopes to the wrong tenant rather than erroring. Harmless while single-tenant; a data-isolation bug the moment tenants are real. Any tenant-scoped test failing at step 7 of §9 is this bug, not a frontend defect.

---

## 6. Cross-cutting UI requirements

- **Token lifecycle:** silent refresh before 900s expiry; on failure route to A2 with a "session expired" notice and **preserve in-flight form state** — a half-typed prescription must survive re-login.
- **SSE event catalog and recipients:** `prescription.ocr_completed` / `approved` / `rejected`, `appointment.booked` / `completed` / `cancelled`, `vital.alert`, `medication.logged`, `ping`. Fan-out is strictly per `PatientID`/`DoctorID`, and **`vital.alert` goes to the patient only** — the doctor is deliberately not pushed unsolicited criticals (`patient_handlers.go:259-271`). So a doctor learns of a crisis only when they open the chart. Whether a clinician alert inbox belongs in the product is a real decision (§7), not a UI choice.
- **Rate limits as UX:** 120/min general, 10/min on register/login/refresh/`/verify/rx`. 429s are HIPAA-audited, so bursts of failed logins are visible in D3 — state this on A7 rather than implying invisibility.
- **Timezones:** appointments are `TIMESTAMPTZ`; `doctor_schedules` carry their own IANA `timezone`. Show the zone explicitly on every slot and appointment, and give the booking flow a clinic-time-vs-your-time affordance.
- **Downloads:** `{download_url, expires_in:300}` — fetch then redirect within 5 minutes; never store the URL in state across idle.
- **Degraded states:** OpenFDA unreachable is a blocking condition (see C7), and OCR failure is a normal queue state (see C10). Both must be explicit; neither may render as a silent pass.
- **PHI hygiene:** no PHI in URLs, localStorage, or analytics events. The SSE token in a query string is the one unavoidable exception — flag it for security review.

---

## 7. Product decisions needed before build

1. Should doctors be notified of patient-recorded crisis vitals? Currently: no.
2. Should patients see in-flight prescriptions (`needs_review`), or is invisibility intentional? Currently: invisible.
3. Are medication reminders in scope? README claims push/email; system has in-app only.
4. Does `prescriptions.expires_at` (with an "active prescriptions" index, `000011:51`) change UI state? Currently no consumer.
5. Single-tenant UI now, or reserve room for clinic branding and a tenant switcher? Tenancy is plumbed but dark-launched.
6. Which admin persona is real — `admin`, `tenant_admin`, or `platform_admin`? Code disagrees with the schema.

---

## 8. Build order

1. **Foundation** — Next.js scaffold, Tailwind clinical palette, §2 status tokens, auth + refresh client, SSE hook, presigned upload/download helpers, error/429/403 shells.
2. **Patient core** — A2/A3, B1, B2–B4 booking funnel, B5, B8–B9. Shortest path to a demoable product.
3. **Doctor core** — C1, C3–C4, C6–C7 prescribing, C8–C10 OCR review. The differentiating surface.
4. **Patient engagement** — B10 medications, B11 vitals + crisis alerts, B7/C13 telehealth.
5. **Admin & compliance** — requires §5's admin-role fix, then D1–D5; A4 verification hardening; A5–A7 legal.
6. **Deferred** — remaining §5 rows, platform console (E).

---

## 9. Verification

Design-time:
- Walk every screen against §2 — no raw enum, no color-only state.
- AA contrast on all status chips; keyboard-operable booking grid, medication board, and the dense doctor tables; visible focus rings.

End-to-end against the running stack (`docker compose up --build`, API on `:8080`; add the dev origin to `CORS_ALLOWED_ORIGINS` first):
1. Register a patient → book with a doctor who has a schedule → confirm non-grid-aligned and <15-min-out times are rejected → trigger 409 by booking the same slot from two sessions → book an overlapping slot for the *same patient* and confirm the patient-side exclusion constraint also fires.
2. Doctor: complete C1 → prescribe digitally in C6 with two interacting drugs → confirm the HIGH-severity interrupt **blocks** submit → override with a reason → confirm 201 `status: approved`, the generated PDF, and that `/verify/rx/:id` renders the masked public view matching that QR target.
3. Block OpenFDA (bad egress or unset key) and prescribe again → confirm fail-closed behavior: HIGH severity and a required override, with degraded copy shown rather than a clean pass.
4. Upload a handwritten slip (C8) → watch `pending_ocr` → `needs_review` arrive over SSE with no manual refresh → edit an extracted dose in C10 → approve → confirm it appears in B8. Then upload a file that fails extraction and confirm the empty-list-plus-error-note state, and that approve is refused until items are supplied.
5. Upload a **PDF** with an OpenAI-only fallback chain → confirm the unsupported-MIME path surfaces as a reviewable empty state rather than a hung queue.
6. Vitals: post 190/125 in B11 → assert `is_critical: true` with verbatim emergency copy. Confirm the linked doctor receives **nothing** (documented behavior, per §6) and that the reading is visible in C4.
7. Medication board: mark a dose taken → reload → status persists via upsert → confirm B1 counts update. Try logging 2 days forward and confirm rejection.
8. Session: idle past 900s, then act → confirm silent refresh with no lost form state; then attempt a second refresh with the old token and confirm all-sessions-revocation is handled gracefully in the UI.
9. Once §5's admin fix is in: add an OCR key in D4 and verify it never round-trips to the client; reorder D5 and confirm the next extraction uses the new order; verify D3 shows the reads performed during steps 1–7, including the audit-the-audit access.

---

## 10. Critical files to read while building

| File | Why |
|---|---|
| `cmd/main/main.go:446-581` | The only route table; authoritative on middleware and role guards |
| `internal/models/models.go:10-60` | Every enum the UI must label |
| `internal/api/auth_handlers.go` | Login/refresh/logout shapes, invite gating, the `tenant_id` claim bug at `:442-448` |
| `internal/api/prescription_handlers.go` | Upload/presign, digital issue, verify, and the public masked projection at `:871-939` |
| `internal/api/patient_handlers.go` | Vitals ranges + `evaluateVitalAlerts` (`:292-338`), medication log rules, SSE handler |
| `internal/safety/safety.go:21-51` | `SafetyReport` shape and fail-closed behavior |
| `internal/queue/queue.go:56-167` | Proof that every OCR path ends at `needs_review` |
| `internal/notifications/notifications.go:20-29` | SSE event catalog and recipient model |
| `migrations/000013_multi_tenancy.up.sql` | What tenancy actually is today |
| `migrations/000001_init_schema.up.sql` + `000004` | The real profile fields (and their absences) |
