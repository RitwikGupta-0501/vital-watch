# 🛠️ VitalWatch Implementation Plan: Backend Remediation → Frontend Build

> **Document Version**: 1.0.0
> **Date**: 2026-10-05
> **Depends on**: `docs/frontend-page-inventory.md` (screen set), `TODO.md` Phase 6 (findings)
> **Status**: Draft for review — **no code written against this plan yet**

---

## 0. Three corrections to the tracker, before anything else

The deeper code read that produced this plan invalidated two entries as written and surfaced a third. **Applied to `TODO.md` on 2026-10-05** — the counts there are now 30 findings (5 P0, 13 P1, 6 P2, 6 P3). Recorded here so the reasoning travels with the plan.

**TENANT-01 is understated.** I wrote that only the *refresh* path drops the `tenant_id` claim. It is worse and broader: `queries/admins.sql:10-20`, `queries/patients.sql`, and `queries/doctors.sql` never `SELECT u.tenant_id`, so `DBRepository.GetAdminByEmail` (`db.go:220-237`), `GetAdminByID` (`:240-256`), and the patient/doctor equivalents return models with an **unset `TenantID` field**. `Login` does include the claim (`auth_handlers.go:291` — `user.GetTenantID().String()`) but always emits `00000000-0000-0000-0000-000000000000`, which happens to *be* the System Default Tenant. So no real token has ever carried a non-default tenant, from either path. Single-tenant masking a multi-tenant bug, not a refresh-only bug.

Also missing from that entry: registration has no tenant source at all. `/api/register` sits outside `authGroup`, so `getTenantID(ctx)` (`db.go:33-46`) returns `uuid.Nil` for every signup — and `CreateAdmin` doesn't even pass the field (`db.go:191-194` omits `TenantID` from `CreateAdminUserParams`, while `CreatePatientUser:93-97` and `CreateDoctorUser:308-312` pass `getTenantID(ctx)`). New users land in the default tenant by accident, not by policy.

**OPS-03 is overstated.** I called `prove_notify_bug.go` a build hazard. Verified: it is `package main` with its own `func main` in the *root* directory, `cmd/main/main.go` is a different package, and `go build ./...` succeeds cleanly today. It compiles as a stray binary target and is dead weight, nothing more. Downgrade to P3 hygiene, which is where it already sits — just remove the "build hazard" wording.

**A third finding, now recorded as AUTH-01 (P1)**: `Login` sets the `role` claim from the **request-supplied** `req.Role` (`auth_handlers.go:290`) while `RefreshToken` sets it from the **database row** (`:444` via `:394-405`). The same account can therefore hold two different role strings across its tokens, and that string is what `IsUserActive`, `RequireRole`, `GetUserProfile`, and `GetComplianceAuditLogs` all match on. PR-A1 fixes it as a side effect; it is listed separately because it must be fixed even if the persona decision slips.

---

## 1. Shape of the work

Two tracks, deliberately sequenced:

- **Track A — backend remediation** (5 PRs). Small, contained, mostly in `internal/api/auth_handlers.go`, `internal/repository/`, and `cmd/main/main.go`. A prerequisite for six admin screens and for any tenant-scoped feature.
- **Track B — frontend build** (6 milestones). Track B's M0 scaffold can run in parallel with Track A; M1 onward is unaffected by A except for the admin console.

**Do not attempt Track A as one PR.** A2 (tenant claims) and A3 (logout) are independent of A1's role-vocabulary change and can merge in any order; A1 is the only one that requires a product decision first.

---

## 2. Track A — backend remediation

### The decision that gates PR-A1

`TODO.md` SEC-12 offers a choice; it must be made in writing before code. Current facts: the CHECK constraint (`000013:28`) allows `platform_admin` and `tenant_admin` and rejects `admin`; `admin_profiles` (`000007`) has no `tenant_id` column; `tenant_invites.role` (`000013:16`) allows only `tenant_admin` and `doctor` — so the invite system as designed cannot mint a `platform_admin`.

**Recommended**: keep both personas (the schema already anticipates them) and treat `admin` as retired. `tenant_admin` = clinic operator, scoped by `tenant_id`; `platform_admin` = you, bypasses tenant scope. This matches the existing RLS policy shape and `RequirePlatformAdmin`/`RequireTenantAdmin` (`handlers.go:347-367`), which are already written for exactly this and currently guard nothing.

Rejecting this and collapsing to a single persona is simpler but throws away `admin_profiles`' only real distinction; if you go that way, say so before A1 and drop the unused guard.

### PR-A1 — Make an admin that can be created and can sign in

**Objective**: one admin registration → login → `/api/profile` → one `/api/admin/*` route call, verified against a live database rather than a mock.

**Changes**

1. **`migrations/000014_align_admin_role_vocabulary.up.sql`** (new) + `.down.sql`:
   - `UPDATE users SET role = 'tenant_admin' WHERE role = 'platform_admin' AND tenant_id = '00000000-0000-0000-0000-000000000000'::uuid;` — only if you decide the default tenant's admins are clinic-level. Otherwise leave as-is.
   - Add `admin_profiles.tenant_id UUID NOT NULL DEFAULT '00000000-…'::uuid REFERENCES tenants(id) ON DELETE CASCADE` plus a backfill from `users`, since `000013` skipped this table and `GetAdminByEmail` already filters `u.tenant_id`.
   - Do **not** re-add `'admin'` to the CHECK. A migration that widens the constraint back is the easy wrong answer — it "fixes" register while leaving the two-persona model incoherent.
2. **`internal/repository/queries/admins.sql`** — the actual defect. Parameterize the role instead of hardcoding, so one query serves both personas:
   - `CreateAdminUser`: accept `@role` and write it; add `RETURNING id, role` is unnecessary, keep `:one` shape.
   - `GetAdminByEmail` / `GetAdminByID`: replace `u.role = 'admin'` with `u.role IN ('tenant_admin','platform_admin')`. Add `u.tenant_id` to both SELECT lists (this is also TENANT-01's first half — see PR-A2 for the rest).
   - Note `admin_handlers.go` and `models.Admin.GetRole()` (`models.go:183-188`, fallback `return "admin"`) must move in lockstep; the fallback string is what `Login`'s role-mismatch guard compares against (`auth_handlers.go:270`), so a stale fallback silently 401s every admin login.
3. **`sqlc.yaml`** — **append `migrations/000014_align_admin_role_vocabulary.up.sql` to the explicit `schema:` list.** This is the trap: the list is 12 named files, not a directory, and already omits `000003_river_queue.up.sql`. Forget this step and sqlc generates code against a schema it never saw, silently.
4. Regenerate: `sqlc generate` (pinned `v1.31.1` per `dbgen/admins.sql.go:3`) → re-emits `dbgen/admins.sql.go:37-115`, `dbgen/querier.go:17-18,34-35`, and `dbgen/models.go`.
5. **`internal/repository/db.go`** — `CreateAdmin` (`:182-215`) gains a `role string` parameter and sets `TenantID:` on the params struct (currently omitted at `:191-194`); `GetAdminByEmail`/`GetAdminByID` map `TenantID: row.TenantID` once the queries return it.
6. **`internal/repository/repository.go:97-101`** — interface signature change; update `internal/repository/mock.go:496-501` accordingly.
7. **`internal/api/handlers.go:62-94` `IsUserActive`** — the third, least obvious break. Its switch handles `"patient" | "doctor" | "admin"` and hits `default: active = false` (`:85-86`), so **a `platform_admin` or `tenant_admin` token is rejected 401 on every `authGroup` route before any handler runs**. Add both cases. This is only visible in production: `admin_test.go:32` uses the package-level `AuthMiddleware(jwtSecret)` (`handlers.go:150-152`) which passes `checker = nil`, skipping the active check entirely — which is exactly why the existing suite is green while the feature is dead.
8. **`internal/api/auth_handlers.go`** — `GetUserProfile` (`:517-547`) needs `case "tenant_admin", "platform_admin":` alongside `"admin"`; today those tokens get 400 `Invalid user role` at `:545-546`. And fix AUTH-01 while here: use `user.GetRole()` rather than `req.Role` for the claim (`:290`).
9. **`cmd/main/main.go:560`** — once AUTH-01 is fixed, replace the three-string `RequireRole("admin","tenant_admin","platform_admin")` with the real guards: `adminGroup.Use(api.RequireTenantAdmin())` and a separate `platformGroup.Use(api.RequirePlatformAdmin())` for anything cross-tenant. This is what makes CODE-02 stop being dead code.

**Test deltas** (all currently assert the retired `'admin'` string; the agent enumerated them, so this is the complete list):
- `internal/api/admin_test.go` — lines `44, 79, 117, 150, 172, 209, 223, 251, 264, 287, 305, 346, 357, 404, 446, 493, 510, 552, 590, 633` (router guard, register bodies, mock `Role:` fields, profile assertion `resp.Role != "admin"`, refreshed-claims assertion).
- `internal/api/auth_test.go:278` — register body `"role":"admin"`.
- `internal/api/phase5_test.go:352` — `X-Role: admin` on `/api/compliance/audit-logs`.
- `tests/integration_test.go:84` — `assert admin.Role != "admin"` in `TestLiveDB_AdminDecouplingAndQueries`. **This is the test that should have caught the bug** and currently self-skips when `DATABASE_URL` is unreachable (`tests/testdb.go:50-54`).
- New test, the one that actually matters: a live-DB register → login → `GetUserProfile` → `RequireTenantAdmin` pass, using `tests/testdb.go`'s migration harness. Without it, a mock-based suite stays green over a broken constraint.
- `admin_test.go:650-776` `TestMultiTenancy_PlatformAndTenantAdminRoles` already covers the persona split but passes today only because the checker is `nil`; extend it to route through `h.AuthMiddleware()`.

**Verification**: `go test -race ./internal/api/ ./internal/repository/ ./tests/` with a live Postgres (not the self-skipping default), `go vet ./...`, `golangci-lint run` (CI pins `v1.64`), and manually: `curl` register as `tenant_admin` with `ADMIN_INVITE_CODE` → expect 201, login → expect 200 and a decodable JWT whose `role` is `tenant_admin`, then `GET /api/profile` → expect 200 not 400.

**Revert**: `000014.down.sql` restores `admin_profiles.tenant_id` removal and the role rewrite; the query/Go changes revert as one commit. Do not split the migration and the query change across PRs — half-applied is worse than unapplied.

### PR-A2 — Tenant continuity in tokens

**Objective**: the `tenant_id` claim is real and survives refresh, so tenant scoping stops being decorative.

1. Add `u.tenant_id` to the SELECT lists in `queries/patients.sql`, `queries/doctors.sql`, `queries/admins.sql` (admins half may already be in PR-A1); populate `TenantID` on the returned models in `db.go` (`:126-134`, `:145-152`, `:228-237`, `:248-256`).
2. **`auth_handlers.go:442-448`** — add `"tenant_id": <uuid>.String()` to the refresh `MapClaims`. The role is resolved at `:394-405` by trying patient → doctor → admin; capture the tenant from the same lookup rather than re-querying.
3. **`auth_handlers.go:394-405` + `db.go:33-46`** — during refresh there is no JWT-derived tenant in context (route is outside `authGroup`, `main.go:500`), so `getTenantID(ctx)` is `uuid.Nil` and the lookups are filtered to the default tenant. A non-default-tenant user cannot refresh today. Resolve the user by ID without a tenant predicate, then use *that* row's tenant.
4. **`handlers.go:219-226`** — currently defaults a missing claim to `uuid.Nil` silently. Given `uuid.Nil == SystemDefaultTenantID`, an absent claim is indistinguishable from a default-tenant user. Recommend rejecting tokens with no `tenant_id` claim once every mint site emits one, and defaulting explicitly to `models.SystemDefaultTenantID` (`models/tenant.go:5-9`) by name rather than by the zero value.
5. Add `tenant_id` to `auth_test.go:25-42` `generateTestToken` — every middleware test currently runs with a nil tenant, which is why none of them caught this.
6. New test: login → refresh → assert the refreshed token's `tenant_id` equals the login token's, for a user in a **non-default** tenant. This assertion does not exist anywhere today.

Note `refresh_tokens` has no `tenant_id` column (`000005:1-9`) — don't add one; derive from `users`.

### PR-A3 — Harden logout

Small, independent, merge first if you want a quick win.

1. **`main.go:501`** → `r.POST("/api/auth/logout", authLimiter.Middleware(), h.AuthMiddleware(), h.Logout)`.
2. **`auth_handlers.go:466-490` `Logout`** — it never reads `userID` from context and performs no ownership check; `GetRefreshTokenByHash` (`:477`) matches on hash alone, and `RevokeRefreshToken` (`queries/tokens.sql:11-14`) revokes by `id` with no user scoping. Add `WHERE user_id = $2` semantics and compare `existingToken.UserID` against the authenticated `userID` — otherwise an authenticated user holding someone else's token string can revoke their session. Mitigating factor: the idempotent 200 at `:478` means an unauthenticated probe learns nothing about token validity.
3. Cap `refresh_token` length like `RefreshToken` does at `:354-357` (512); `Logout` has no cap today.
4. Bonus once authenticated: audit rows stop being written with `user_id = NULL` / `user_role = NULL` (`:486-488` → `handlers.go:303-344`), which currently makes logout useless in the `phi_audit_logs` viewer (screen D3).
5. Tests: `phase5_test.go:256-301` `TestLogout_RevokesToken` and `auth_test.go:352-421` `TestLogoutAllDevices` both send no `Authorization` header and assert 200 — they must be updated to authenticate, plus a new case asserting 401 without a token and 403/200-idempotent for another user's token. Their routers (`phase5_test.go:44`, `auth_test.go:390`) also need the middleware to mirror production.

### PR-A4 — Decide RLS: activate it or delete it

The security posture decision, not a bug fix. `000013:57-94` enables RLS with `tenant_isolation_policy` on 10 tables keyed on `app.current_tenant` / `app.is_platform_admin`; **no Go code sets either GUC**. Every CI and deploy DSN is a superuser or owner (`ci.yml:20-23`, `tests/testdb.go:32`), and owners bypass RLS — so the policies are inert even where a non-owner would hit them.

- **Option 1 (real defense-in-depth)**: create a non-superuser app role, `SET LOCAL app.current_tenant = '<uuid>'` (and `app.is_platform_admin`) per transaction from the JWT claim — natural insertion point is the `qtx`/transaction helpers in `db.go`, right where `getTenantID(ctx)` is already called. Add a request-scoped test proving a wrong tenant returns zero rows even with a missing predicate.
- **Option 2 (honest simplification)**: drop the policies and the two GUC functions in `000014` or a follow-up, and rely on the app-side `tenant_id = @tenant_id` predicates alone — but then `TODO.md` and any security write-up must stop describing PHI isolation as RLS-enforced.

Either is defensible. Leaving it as-is is not: a reviewer reading the migrations concludes tenant data is protected at the database layer, and it is not.

### PR-A5 — `/metrics` exposure and remaining P0 noise

`main.go:494` registers `/metrics` unauthenticated, subject only to the 120/min general limiter, exposing route cardinality, request rates, and pool telemetry. Bind it to a separate internal listener (the pattern `HealthCheck` at `:495-496` already suggests: liveness belongs public, metrics do not) or gate it behind a bearer token/IP allowlist. Fold in SEC-15: `SSEAuthMiddleware` accepting `?token=<jwt>` (`handlers.go:183-184`, `patient_handlers.go:99-113`) puts bearer tokens in proxy logs — record the accepted risk in an ADR or move to a short-lived stream ticket; the frontend's `EventSource` design in `docs/frontend-page-inventory.md` §1 assumes the query-token path, so this decision is a contract with the frontend.

### Track A tooling gaps worth fixing in passing

- **No sqlc drift check.** `sqlc generate` is run by hand (no Makefile, no CI step, undocumented in the README) while every generated file is pinned to `v1.31.1`. Add a CI step: `sqlc generate && git diff --exit-code internal/repository/dbgen`. A1 is exactly the PR where forgetting this produces committed drift.
- **CI/test Go version mismatch**: `.github/workflows/ci.yml:31` installs `1.24`, `go.mod` declares `go 1.26.0`. Pick one.
- **CI never runs the live-DB integration suite as a gate** — `tests/testdb.go:50-54` self-skips, so `TestLiveDB_AdminDecouplingAndQueries` (the test that would have caught SEC-12) is silently optional even though CI provisions Postgres. Make it fail rather than skip when `DATABASE_URL` is set.

---

## 3. Track B — frontend build

Per `docs/frontend-page-inventory.md`: 32 screens now, 6 admin screens after PR-A1, 18 blocked until §5 of that doc is addressed. Milestones are ordered by demo value, not by nav order.

| M | Deliverable | Screens | Depends on |
|---|---|---|---|
| M0 | Repo scaffold + design system + API client | — | nothing (parallel with Track A) |
| M1 | Auth vertical slice | A2, A3, login/register, session | PR-A2 (tenant claims) for a stable session model |
| M2 | Patient booking funnel | A1, B1–B5, B8–B9 | M1 |
| M3 | Doctor core + prescribing | C1–C7, C11, C12 | M1 |
| M4 | OCR review queue | C8, C9, C10 | M3; `OCR_ENABLED` + at least one provider key |
| M5 | Engagement: meds, vitals, telehealth | B7, B10–B12, C5, C13 | M2, M3 |
| M6 | Admin console + public verify | A4, A7, D1–D5 | **PR-A1 merged** |

**M0 specifics** — the pieces every later milestone imports, so build them once and deliberately:

- **Scaffold**: Next.js App Router + TypeScript strict mode + Tailwind, in the separate repo decided in the inventory. Route groups `(public)`, `(patient)`, `(doctor)`, `(admin)` exactly as `docs/frontend-page-inventory.md` §1. Add the dev origin to `CORS_ALLOWED_ORIGINS` before the first proxied request; `main.go:467-489` currently defaults to `localhost:3000` plus a hardcoded CloudFront URL.
- **`lib/api/types.ts`** — do **not** hand-write response shapes, but also do not assume a spec exists: there is no OpenAPI document or generator in this repo today. Two options, and the choice is a §6 decision:
  1. Add `swag` annotations to the Go handlers and generate `openapi.json` into the frontend repo at build time. Correct, but it means touching backend files during M0, which collides with Track A's churn in the same functions.
  2. Hand-write the type module from `main.go:446-581` + `models.go` + the handler structs, and add a CI step in the frontend repo that greps the route table for path strings absent from the type module. Fast, drift-prone, no backend edits.
  Given Track A is about to change `Register`/`Login`/admin shapes, recommend **option 2 for M0**, revisiting option 1 once A1–A3 merge — generating from a spec you're actively rewriting just produces rework.
- **Status token module** `lib/status.ts`: one file mapping every enum in §2 of the inventory to `{label, icon, tone}`. Nothing renders a raw enum value. Chip component = color + icon + text, never color alone (WCAG AA).
- **API client** `lib/api/client.ts`: single `fetch` wrapper handling (a) 900s access-token refresh behind a **mutex** — parallel 401s must not both fire refreshes, because reuse outside the 10s grace revokes every session and writes a `TOKEN_REUSE_SECURITY_ALERT`; (b) 429 with `Retry-After`, since the 10/min auth limiter is trivially hit during testing; (c) the three-way success-shape problem (**API-11**) normalized at the boundary: `unwrap()` accepts `{id}`, `{id, status, …}`, `{data: …}`, `{message}` and returns a resource or throws a typed `ApiError`.
- **SSE hook** `hooks/useNotificationStream.ts`: one `EventSource` on `/api/notifications/stream?token=…`, 15s heartbeat handling, exponential reconnect, and **assume event loss** (the broker drops on slow consumers — `notifications.go:215-220`) so every mount refetches its queries. On disconnect, show the global "reconnecting" banner rather than silently stale data.
- **Presigned helpers** `lib/storage.ts`: two-step upload (request URL → `PUT` raw bytes) and download (`{download_url, expires_in: 300}` → fetch-then-redirect immediately). Never persist a presigned URL across idle. Must handle both `STORAGE_PROVIDER=s3` and `local` (HMAC query params) because local is the default for demo.
- **Error/empty/permission shells**: `403` role gate, `404`, `429`, and one `EmptyState` component with a *distinct* variant per cause — this repo has several endpoints that return an empty list where a real product would error (the slot grid for a doctor with no configured schedule is the notable one), so "nothing yet" and "nothing available" cannot share a component instance.
- **Test harness**: Vitest + Testing Library + MSW with handlers copied from the route table; Playwright for the M1+ auth and booking flows. Playwright is where Track A's live-DB fixes and the frontend get verified together.
- **Done when**: a build passes with zero `any`, `client.ts` survives a concurrent-401 storm with exactly one refresh call, and the status module covers every enum value in `models.go:10-60` with a test asserting no unmapped value can render.

### M1 — Auth vertical slice

Covers A2, A3, A5–A6 (legal placeholders are cheap here), session plumbing.

- A2 login: role selector is **functional** — `Login` requires `role` in the body and does a per-role lookup, rejecting a mismatch (`auth_handlers.go:268-272`). Its own error state ("you picked the wrong account type"), not a generic invalid-credentials message.
- A3 register: patient form open; doctor and admin forms reveal the invite-code field. Handle 409 email conflict inline. **Admin registration will 500 until PR-A1 merges** — ship the form behind a `adminRegistration: false` feature flag in `config/flags.ts` so the flag flip is the only change when A1 lands, rather than a code edit under review pressure.
- Token storage decision: refresh token is single-use and rotating, so it **cannot** live in localStorage safely if multiple tabs are open (two tabs race the rotation and trip all-session revocation). Recommended: `httpOnly` cookie via a Next route-handler BFF (`app/api/session/…`) that owns rotation in one place. This is a §6 decision; it changes M0's client shape, so decide it before M0 ends.
- B12-style sign-out-all-devices is stubbed here and finished in M5.
- **Done when**: Playwright covers login→refresh→expired-session-return-with-preserved-form-state, and the wrong-role path produces the specific message. **Blocked by nothing** if the BFF is used; `PR-A2` only matters once a non-default tenant exists, but M1 must already read `tenant_id` off the session and put it in the org/clinic header so A2's fix is a no-op for the UI.

### M2 — Patient booking funnel

A1 landing, B1 dashboard, B2–B4 booking, B5 appointments, B8–B9 prescriptions.

- B3 slot grid: render only what the server returns; slots are constrained to the doctor's `slot_duration` grid inside working hours, with a 15-minute lead time and a 1-year horizon. Show the clinic timezone on every slot next to the patient's local time.
- B4: client-side pre-check against the patient's own existing appointments (the patient-side GiST exclusion in `000009` will 409 anyway; the pre-check exists to avoid a wasted submission), plus 409 handling that refetches the grid and says "this time just went."
- B8 is **single-state by design** — patients receive only `approved` prescriptions; anything else 404s (`prescription_handlers.go` ownership/status gating). Do not invent a "processing" row. Inventory §7 raises it as a product decision.
- B9: PDF download plus the A4 verify link.
- **Done when**: the inventory's §9 steps 1 and 2 (partially) pass against `docker compose up --build`, including the deliberate double-booking 409 from two sessions.

### M3 — Doctor core + prescribing

C1 today, C2 appointments, C3 roster, C4 chart, C6 digital prescribe, C7 safety interrupt, C11 prescriptions, C12 schedule.

- Navigation is **roster-first**: doctor→patient access is relationship-gated, so there is no global patient search to build. C3 is the entry point to every chart.
- C7 safety modal: **blocking**, not a banner. OpenFDA is fail-closed — unreachable yields HIGH severity plus `service_degraded`/`degraded_reason`/`unchecked_drugs` (`safety.go:403-417`), and the copy must state that the interaction database could not be reached. Override requires a typed reason, which is appended to the prescription notes and therefore becomes visible in B9 and the public A4 page — write the field's helper text accordingly.
- Do **not** render an allergy section: `prescription_handlers.go:525` passes `nil` allergies, so `allergy_alerts` can never be non-empty (**SAFE-02**). An "Allergy alerts: none" empty state is a false claim that a check ran. Add it only when PROF-01 + SAFE-02 land.
- C6 issues already-approved prescriptions with a server-generated PDF and QR, so the success state is "download / send," not "pending."
- C12: slot duration is a whitelist `{15,20,30,45,60}`; day removal is `DELETE /api/doctor/schedules/:day`.
- **Done when**: §9 step 2 passes end-to-end (interacting drugs → block → override → 201 approved → A4 matches the QR) and §9 step 3 passes with OpenFDA blocked.

### M4 — OCR review queue

C8 upload, C9 queue, C10 split-screen HITL reviewer.

- C10 must treat **zero extracted items plus a bracketed error note** as a normal state — every OCR path lands at `needs_review`, success and failure alike (`queue.go:81,95,103,119,126,159`) — and `VerifyPrescription` refuses approval with no items (`prescription_handlers.go:777-779`). So the reviewer needs a full "build the list by hand" mode, not an empty screen.
- No confidence meter (**OCR-05**). Show `ocr_provider` (persisted) and label the extraction as unverified AI output (ADR-004).
- Split-pane: scan on the left via `.../download-url`, editable fields on the right, per-line accept/reject, keyboard-first, and an approve shortcut — this screen is where clinicians spend the most time in the product.
- C8: 15 MB cap, pdf/jpg/jpeg/png whitelist, requires a prior appointment with the patient, duplicate filename → 409 (regenerate the key client-side). Warn in-copy that PDF is rejected by OpenAI-only chains (**OCR-06**) and default the UI toward image capture.
- Depends on `OCR_ENABLED` plus at least one provider config, which requires **PR-A1** to be merged (the vault is admin-only, D4). Without A1, seed keys directly in Postgres for local development and say so in the README.
- **Done when**: §9 steps 4 and 5 pass, including arriving at `needs_review` over SSE with no manual refresh.

### M5 — Engagement: medications, vitals, telehealth

B10 medication board, B11 vitals, B7/C13 telehealth rooms, B6 visit detail, B12 settings completion.

- B10: upsert semantics — re-tapping a dose updates rather than inserts; date bounds 1 day forward / 2 years back; `dose_number` ≤ 24. **No push or email reminders exist** (**NOTIF-02**), so the surface must be labeled as an in-app today list. Do not write "we'll remind you."
- B11 is safety-critical: render `critical_alerts` **verbatim** from the create response — the backend already emits the emergency instructions, and softening them in copy is a patient-safety risk. Mirror every input range client-side (SBP 50–300, DBP 30–200, HR 30–250, glucose 10–1000, SpO₂ 50–100, temp 30–45, weight 1–500, SBP > DBP) so a 400 becomes an inline error. Show `recorder_name`/`recorder_role` provenance rather than implying the patient logged everything.
- Telehealth: shared room component for B7/C13, with the hard access window surfaced as its own state — opens start−15 min, closes end+24 h, 403/410 outside ("you can join from HH:MM"). This is the single most likely support call in the product; do not let it render as a generic error.
- **Known behavior to design around**: `vital.alert` reaches the **patient only** (**NOTIF-03**). A clinician learns of a crisis only by opening the chart. Ship it honestly — a C4 chart badge computed on read, not an alert claim.
- **Done when**: §9 steps 6 and 7 pass, including the deliberate assertion that the linked doctor receives no SSE event.

### M6 — Admin console + public verification

D1–D5, A4 hardening, A7 security page. **Hard gate: PR-A1 merged**, plus PR-A4's RLS decision for anything written about isolation.

- D4 BYOK vault is **write-only by design** — `GetOCRSettings` returns `provider_name` and `created_at` only; keys are AES-256-GCM envelope-encrypted with AAD bound to tenant+provider. No reveal affordance, and a confirm-before-submit warning that the key cannot be retrieved. Rotation today means adding a new config because delete/replace does not exist (**ADMIN-01**) — say that in the UI until the endpoint lands.
- D3 audit viewer: async auditor with a file DLQ (**AUD-01**), so never label it "complete log of all access." Include the audit-the-audit access itself (`admin_handlers.go:164-178`). Offset pagination will degrade on this table until **API-14** is fixed — keep the date-range filter tight by default.
- D1: self-deactivation is refused server-side; disable the control on the actor's own row with the reason, and state that deactivation instantly revokes all their refresh tokens.
- D6 (`/admin/settings`) stays out: no endpoint exists for clinic identity or PDF branding (**TENANT-04**).
- A4: bare page, `noindex`, no nav chrome, deliberate trust treatment (issue date, prescriber, medication table, masked patient id). The 10/min limiter means a clinic event scanning many copies hits 429 — that is a designed state, not a failure.
- **Done when**: §9 step 9 passes, and D4 is verified to never round-trip a key to the client (assert on the wire, not in a mock).

---

## 4. Cross-cutting implementation notes

These are the things that bite in more than one milestone; build them into M0 so nobody improvises per screen.

- **PHI hygiene as a lint rule, not a guideline.** No PHI in URLs, localStorage, or analytics events. Enforce with an ESLint custom rule banning `localStorage.setItem` under `app/(patient)|doctor|admin)` and a test that no route definition embeds a patient name or email in its path. The one exception is the SSE `?token=<jwt>` query string (**SEC-15**) — document it in the repo README as an accepted risk tied to PR-A5, and prefer the BFF/cookie path if §6's storage decision goes that way.
- **`staleTime: 0` on PHI queries.** The auth group sends `no-store` via `SensitiveCacheControlMiddleware` (`main.go:508`), so bfcache reuse cannot be relied on and navigation must expect refetch. Put this in the QueryClient defaults, not per-call.
- **Response-shape drift (API-11)** is normalized in `unwrap()`, and each normalization is annotated with the handler it exists for, so deleting one when the backend is unified is a one-line change with a test that fails when the shape no longer matches.
- **Pagination** is offset-only today (**API-14**). For D3 and B11 use date-range filters as the primary narrowing control and cap the offset ceiling in the UI (e.g. page 50) rather than letting a user page into a multi-second query.
- **403 vs 404 semantics.** Several ownership violations return 404 deliberately (patients cannot see non-approved prescriptions). Do not render a 404 as "something went wrong" — it is "this is not yours to see," and for the patient prescription list it is the expected result for anything still in review.
- **Rate-limit states are first-class.** 120/min general, 10/min on register/login/refresh/`/verify/rx`. A shared `useRateLimitedSubmit` hook that surfaces `Retry-After` and stops the button beats a per-screen implementation.
- **Feature flags** in one `config/flags.ts` for exactly three conditionals: `adminRegistration` (until PR-A1), `safetyAllergySection` (until PROF-01 + SAFE-02), `multiTenantUI` (until PR-A2 + TENANT-03/04). Flags exist so an unblock is a config flip, not a PR that changes component trees.
- **Timezones**: appointments are `TIMESTAMPTZ`, `doctor_schedules` carry their own IANA zone. One `formatClinicTime()` helper, and every slot/appointment renders the zone explicitly.
- **Accessibility**: WCAG 2.1 AA is a product requirement, not a polish pass. The three surfaces that must be keyboard-operable from day one are the booking grid (B3), the medication board (B10), and the HITL reviewer (C10) — they are the high-volume clinical interactions.
- **Backend repo changes during Track B**: resist them. Anything discovered while building goes into `TODO.md` Phase 6 as a numbered finding; the frontend repo stays out of Go files except where §3 M0's spec-generation decision explicitly opens that door.

---

## 5. Ordering, dependencies, and effort

Rough single-engineer estimates in dev-days for the work as scoped; they assume the backend is stable at merge of each listed PR.

| Unit | Scope | Depends on | Days |
|---|---|---|---|
| PR-A3 | Logout hardening | — | 1 |
| PR-A1 | Admin role vocabulary | **§6 decision Q1** | 3–4 |
| PR-A2 | Tenant claims | A1 (admins query overlap) | 2–3 |
| PR-A5 | `/metrics` + SEC-15 ADR | — | 1 |
| PR-A4 | RLS activate-or-delete | A2 | 4–6 (activate) / 1 (delete) |
| CI: sqlc drift, Go version, live-DB gate | Track A tooling | — | 1 |
| M0 | Scaffold, design system, API client, SSE, presign | — | 5–7 |
| M1 | Auth slice | **§6 decision Q2** (token storage) | 3–4 |
| M2 | Patient booking funnel | M0, M1 | 6–8 |
| M3 | Doctor core + prescribing | M0, M1 | 8–10 |
| M4 | OCR review queue | M3, A1, `OCR_ENABLED` + a provider key | 6–8 |
| M5 | Meds, vitals, telehealth | M2, M3 | 7–9 |
| M6 | Admin console + public verify | A1, A4 | 5–7 |

**Critical path**: `Q1 → PR-A1 → {PR-A2, M4 seed, M6}`. Everything else can move in parallel with `M0 → M1 → M3 → M4`. Track A's four small PRs (A3, A1, A2, A5) total roughly a week and unblock six screens plus every tenant-aware feature, so they should not wait on frontend work.

**What must not be reordered**: PR-A1's migration and query change land together (half-applied is worse than unapplied), and M6 does not start before A1 merges — the admin console is the one milestone where the UI would have to be written twice against two different role models.

**Explicitly out of this plan** (each is its own numbered finding in `TODO.md`, not frontend work): PROF-01/SAFE-02 allergy + profile schema, API-12 profile edit and password change, API-13 doctor search, RX-04 expiry surfacing, ADMIN-01 key rotation, NOTIF-02 reminder delivery, TENANT-04 tenant CRUD, the platform operator console (inventory §E), COMP-01 erasure/export.

---

## 6. Decisions needed from you before code

1. **Admin persona** (blocks PR-A1). Recommendation: keep `tenant_admin` + `platform_admin`, treat `admin` as retired. The CHECK constraint, `admin_profiles`, and the two unused guards already anticipate two personas; collapsing to one makes `RequireTenantAdmin`/`RequirePlatformAdmin` deletable and the RLS policy meaningless.
2. **Refresh-token storage** (blocks M0's client and M1). Recommendation: `httpOnly` cookie via a Next route-handler BFF. Single-use rotating tokens + multiple tabs is a revocation trap, and a client-side implementation will ship that bug to real clinic desktops with three tabs open.
3. **Type generation** (M0). Recommendation: hand-written `types.ts` with a route-table drift check now, `swag`/OpenAPI after PR-A1–A3 merge.
4. **RLS: activate or delete** (PR-A4, and any security copy). Recommendation: activate it as a real second boundary before the first non-default tenant is onboarded — cheaper now than after a security review reads the migrations and finds inert policies.
5. **Patient visibility of in-flight prescriptions** (inventory §7.2). Currently invisible; affects whether B8 needs a "your doctor is preparing…" state. A product call, not a UI call.
6. **Clinician crisis alerts** (inventory §7.1, NOTIF-03). Currently suppressed. Either add the push and a clinician alert surface, or record the suppression rationale in an ADR — a patient-safety product that ships silent crisis values should have that in writing.

---

## 7. Verification

**Per-PR (Track A)** — each must include a live-Postgres test, because mocks are what let SEC-12 ship:
- A1: register `tenant_admin` with `ADMIN_INVITE_CODE` → 201; login → 200 with a decodable JWT whose `role` is `tenant_admin`; `GET /api/profile` → 200 (not 400); one `RequireTenantAdmin` route passes; `IsUserActive` passes for both personas. Reproduce the original failure first — before the fix, the same test must fail on the check constraint. `go test -race ./...`, `go vet ./...`, `golangci-lint run` (CI pins v1.64), `sqlc generate && git diff --exit-code internal/repository/dbgen`.
- A2: login → refresh → assert `tenant_id` survives and equals the login token's, for a **non-default** tenant. No such assertion exists today anywhere.
- A3: logout without a token → 401; logout presenting another user's refresh token → does not revoke it; audit row carries a real `user_id`/`user_role`.
- A4: request-scoped test proving a wrong tenant yields zero rows; or, if deleting, a grep-level check that no doc or comment still claims RLS enforcement.
- A5: `/metrics` unreachable from the public listener; an ADR records the SSE query-token decision either way.

**Per-milestone (Track B)**: unit + Playwright in the frontend repo, plus the inventory §9 end-to-end run against `docker compose up --build` with the API on `:8080`. Milestone gates:
- M0: `client.ts` fires exactly one refresh under a concurrent-401 storm; no enum in `models.go:10-60` is unmapped.
- M1: session-expiry round-trip preserves in-flight form state.
- M2: §9 step 1 fully, including both doctor-side and patient-side 409 exclusion constraints.
- M3: §9 steps 2 and 3, including the fail-closed OpenFDA path.
- M4: §9 steps 4 and 5, including the zero-item error-note state and the PDF-against-OpenAI-chain rejection.
- M5: §9 steps 6 and 7, including the assertion that the doctor receives no `vital.alert`.
- M6: §9 step 9, and an on-the-wire assertion that no provider key ever appears in a response body.

**Release gate before any real patient data**: PR-A1–A3 merged, A4's decision recorded, `docs/frontend-page-inventory.md` §5's blocked rows still unshipped (nothing built against a page that cannot work), A5–A7 legal/security copy reviewed with AUD-01's DLQ caveat stated, and the PHI-hygiene lint rule enforced in CI.

---

## 8. What this plan deliberately does not do

It does not resolve the 18 blocked pages in inventory §5, does not add the profile/allergy schema (PROF-01), does not build reminders or the platform console, and does not attempt GDPR erasure (COMP-01). Those are separately-scoped backend decisions; the frontend plan above works around each of them explicitly rather than assuming they exist, which is the difference between this plan and a template-generated build order.