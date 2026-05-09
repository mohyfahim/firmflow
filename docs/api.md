# HTTP API Reference

Base path (local default): `http://localhost:8080`

All successful JSON responses use the envelope `{ "data": ... , "meta": ... }` where `meta` appears only when pagination or extra metadata is returned. Errors use `{ "error": { "code", "message", "details?", "request_id" } }`.

Send `X-Request-ID` from clients if you want distributed tracing; the server may generate one when absent.

---

## Health

| Method | Path | Auth |
|--------|------|------|
| GET | `/health/live` | No |
| GET | `/health/ready` | No |

---

## Auth (`/api/v1/auth`)

| Method | Path | Body | Notes |
|--------|------|------|-------|
| POST | `/register` | `{ "email", "password" }` | Starts registration; verify email next |
| POST | `/verify-email` | `{ "token" }` | Completes signup |
| POST | `/login` | `{ "email", "password", "totp_code?", "recovery_code?" }` | Returns tokens in `data` |
| POST | `/refresh` | `{ "refresh_token" }` | New access (+ refresh rotation per service rules) |
| POST | `/logout` | `{ "refresh_token" }` | Ends refresh session |
| POST | `/forgot-password` | `{ "email" }` | Generic success message (no email enumeration) |
| POST | `/reset-password` | `{ "token", "new_password" }` | After email link |

---

## Current user (`/api/v1/me`)

All routes require `Authorization: Bearer <access_token>`.

| Method | Path | Body |
|--------|------|------|
| GET | `/profile` | — |
| PATCH | `/profile` | Optional `first_name`, `last_name`, `avatar_url`, `company_name`, `phone_number`, `timezone`, `preferred_language` |
| POST | `/change-password` | `{ "current_password", "new_password" }` |
| GET | `/sessions` | — |
| DELETE | `/sessions/:sessionID` | — |
| DELETE | `/sessions/others` | — |
| POST | `/2fa/enable` | `{ "password" }` |
| POST | `/2fa/confirm` | `{ "code" }` |
| POST | `/2fa/disable` | `{ "password", "totp_code?", "recovery_code?" }` |
| DELETE | `/` | `{ "password", "grace_period_days?" }` |

---

## Projects (`/api/v1/projects`)

All routes require `Authorization: Bearer <access_token>`.

### Collection

| Method | Path | Query / body | Permission (conceptual) |
|--------|------|----------------|-------------------------|
| POST | `/projects` | `{ "name", "description?" }` | Authenticated creator becomes owner |
| GET | `/projects` | `page`, `page_size` (max 100), `sort` (default `-created_at`), `q` | Lists projects for current user |

### Single project (`:projectID` = UUID)

| Method | Path | Body | Permission key |
|--------|------|------|----------------|
| GET | `/projects/:projectID` | — | `project.read` |
| GET | `/projects/:projectID/summary` | — | `dashboard.read` |
| PATCH | `/projects/:projectID` | `name?`, `description?` | `project.update` |
| DELETE | `/projects/:projectID` | — | `project.delete` |
| POST | `/projects/:projectID/archive` | — | `project.update` |
| POST | `/projects/:projectID/unarchive` | — | `project.update` |

### Audit

| Method | Path | Query |
|--------|------|-------|
| GET | `/projects/:projectID/audit-logs` | `page`, `page_size`, `actor_id?`, `event?`, `from?`, `to?` (RFC3339) |

Requires `audit.read`.

### Members

| Method | Path | Body |
|--------|------|------|
| GET | `/projects/:projectID/members` | — |
| POST | `/projects/:projectID/members` | `{ "email", "role_id" }` |
| PATCH | `/projects/:projectID/members/:userID` | `{ "role_id" }` |
| DELETE | `/projects/:projectID/members/:userID` | — |
| POST | `/projects/:projectID/ownership/transfer` | `{ "new_owner_user_id" }` |

Permissions: `member.read`, `member.invite`, `member.update_role`, `member.remove` as applicable; transfer is enforced in the service layer for the current owner.

### Roles

| Method | Path | Body |
|--------|------|------|
| GET | `/projects/:projectID/roles` | — |
| GET | `/projects/:projectID/roles/assignable` | — |
| POST | `/projects/:projectID/roles` | `{ "name", "description?", "permission_keys": ["..."] }` |
| PATCH | `/projects/:projectID/roles/:roleID` | optional `name`, `description`, `permission_keys` |
| DELETE | `/projects/:projectID/roles/:roleID` | — |

Permissions: `role.read`, `member.read` (assignable list), `role.create`, `role.update`, `role.delete`.

---

## Permission keys (reference)

Registered keys include project/member/role management, `audit.read`, `dashboard.read`, and reserved keys for future device/firmware/campaign modules (`device.*`, `firmware.*`, `campaign.*`). See `internal/domain/rbac/permission/registry.go` for the canonical list.
