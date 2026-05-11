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

**List response (`GET /roles`)**: `data` is an object with `roles` (each item: `id`, `name`, `type` = `system` \| `custom`, optional `slug` for system roles, `description`, `permissions` as `{ key, description }[]`, `assigned_user_count`) and `permission_catalog` (all registry keys with `key`, `description`, `group` for UI checklists). Custom role **create** returns a single role-shaped object in `data`; **update** returns the same. Raw role tokens are never returned here.

---

## Device types (`/api/v1/projects/:projectID/device-types`)

| Method | Path | Body | Permission |
|--------|------|------|--------------|
| GET | `/device-types` | — | `device.read` |
| POST | `/device-types` | `{ "name", "processor_architecture", "hardware_board_version", "flash_size_bytes", "memory_notes?" }` (custom type) | `device.create` |
| PATCH | `/device-types/:deviceTypeID` | optional same fields | `device.update` (custom only) |
| DELETE | `/device-types/:deviceTypeID` | — | `device.update` (custom only; blocked if devices still use the type) |

Predefined catalog types are global; custom types are scoped to the project. Names are unique per project for custom types.

---

## Device groups (`/api/v1/projects/:projectID/device-groups`)

| Method | Path | Body | Permission |
|--------|------|------|--------------|
| GET | `/device-groups` | — | `device.read` |
| POST | `/device-groups` | `{ "name", "description?" }` | `device.update` |
| PATCH | `/device-groups/:groupID` | optional `name`, `description` | `device.update` |
| DELETE | `/device-groups/:groupID` | — | `device.update` (removes memberships, then deletes the group) |
| POST | `/device-groups/:groupID/members` | `{ "device_ids": ["uuid", ...] }` | `device.assign_group` (idempotent add) |
| POST | `/device-groups/:groupID/members/remove` | `{ "device_ids": ["uuid", ...] }` | `device.assign_group` (idempotent remove) |

Group names are unique per project (normalized). Membership changes are audited on the project.

---

## Devices (dashboard) (`/api/v1/projects/:projectID/devices`)

| Method | Path | Query / body | Permission |
|--------|------|----------------|------------|
| GET | `/devices` | Query: `online` (`true`\|`false`\|`1`\|`0`), `blocked`, `device_type_id`, `group_id`, `firmware_version` (exact), `last_seen_from`, `last_seen_to` (RFC3339), `q` (name / hardware search), pagination `page`, `page_size` (max 100), `sort` (`name`, `created_at`, `last_seen_at`, `current_firmware_version`, prefix `-` for descending; default `-created_at`) | `device.read` |
| POST | `/devices/bulk` | See **Bulk devices** below | Route requires `device.read`; **service** enforces `device.assign_group`, `device.block`, or `device.token.rotate` per `action` |
| POST | `/devices` | `{ "name", "device_type_id", "hardware_identifier" }` | `device.create` |
| GET | `/devices/:deviceID` | — (device twin: type, blocked, online/offline from `last_seen_at` vs 5m threshold, firmware, `last_seen_at`, recent connection logs, token metadata **without** raw token) | `device.read` |
| POST | `/devices/:deviceID/block` | — | `device.block` |
| POST | `/devices/:deviceID/unblock` | — | `device.block` |
| POST | `/devices/:deviceID/rotate-token` | — | `device.token.rotate` |

**Registration / rotation**: successful `POST /devices` or `rotate-token` returns `data` including `auth_token` **once** (plaintext). Store it client-side; the server stores only a hash.

**Hardware identifier**: unique per **project** (normalized). Duplicate returns `409` with code `hardware_id_in_use`.

---

## Bulk devices (`POST /api/v1/projects/:projectID/devices/bulk`)

Body:

```json
{
  "action": "add_to_group | remove_from_group | block | unblock | rotate_tokens",
  "apply_to_filter": false,
  "filter": {},
  "device_ids": ["uuid"],
  "group_id": "uuid"
}
```

- If `apply_to_filter` is **true**, `device_ids` is ignored; targets are all devices matching `filter` (same fields as **GET /devices** list, expressed as JSON booleans / strings in `filter`: `online`, `blocked`, `device_type_id`, `group_id`, `firmware_version`, `last_seen_from`, `last_seen_to`, `q`).
- If `apply_to_filter` is **false**, `device_ids` is required (non-empty).
- `group_id` is required for `add_to_group` and `remove_from_group`.
- Synchronous cap: **500** devices per request. If `apply_to_filter` matches more than 500, the API returns **`400`** with code `bulk_too_large`, `details.matched`, and `async_recommended: true` (reserved for future async jobs).

Response `data` includes `action`, `processed`, `succeeded`, `failed[]` (`device_id`, `code`, `message`), optional `tokens_by_device_id` for `rotate_tokens`, `sync_cap`, `async_recommended`.

---

## Device client (OTA agent) (`/api/v1/device`)

These routes use **device** authentication, not the user JWT.

**Header**: `Authorization: Device <raw_device_token>`

| Method | Path | Body |
|--------|------|------|
| POST | `/device/poll` | — |
| POST | `/device/report` | `{ "current_firmware_version" }` |

Blocked devices or revoked/disabled tokens receive `401` / `403` with stable error codes. Each call appends a **connection log** and updates `last_seen_at` (and firmware on report).

---

## Permission keys (reference)

Registered keys cover project/member/role management, **devices** (`device.read`, `device.create`, `device.update`, `device.block`, `device.token.rotate`, `device.assign_group`), `audit.read`, `dashboard.read`, and reserved keys for firmware/campaign modules (`firmware.*`, `campaign.*`). See `internal/domain/rbac/permission/registry.go` for the canonical list.
