# HTTP API Reference

Base path (local default): `http://localhost:8080`

New contributors: see **[Developer onboarding](onboarding.md)** for how routes and domains map to code.

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

When campaigns are enabled: **poll** may include `data.ota` (`campaign_id`, `firmware_id`, `version`, `checksum_sha256`) if an **active** campaign has a **pending** or **offered** assignment for the device. On first poll with a **pending** assignment, the server moves it to **offered**; further polls while still **offered** return the same OTA payload so the device can recover metadata (e.g. after losing local state). Paused or cancelled campaigns never offer. The JSON poll path does **not** embed a download secret in the response; use the **binary TCP OTA** flow or the **short-lived download token** endpoint below when issuing tokens from the TCP handler.

**Report**: if an **offered** or **downloaded** assignment exists for an **active** campaign and `current_firmware_version` matches the campaign firmware **version string** (case-insensitive trim), the assignment is marked **installed**; the campaign **completes** when all assignments are terminal (`installed` or `failed`).

Blocked devices or revoked/disabled tokens receive `401` / `403` with stable error codes. Each call appends a **connection log** and updates `last_seen_at` (and firmware on report).

---

## Firmware download (short-lived OTA token)

**No authentication.** Intended only for devices that already received a **time-limited, opaque token** from an authenticated poll flow (typically the binary TCP OTA server). The long-lived **device session token** must never appear in URLs.

| Method | Path | Query |
|--------|------|--------|
| GET | `/api/v1/device/firmware-download` | `token` (required): 64-character hex secret |

Success: streams `application/octet-stream` with `Content-Disposition`, `Content-Length`, and `X-Checksum-Sha256` (same shape as the authenticated project firmware download).

Failure: **`401`** with error code `invalid_ota_token` for unknown, expired, consumed, or blocked-device tokens (message is intentionally generic).

The token is **one-time**: the first successful open of the object stream marks the token consumed; retries must obtain a new token from **poll**.

---

## Device OTA (binary TCP)

When `DEVICE_OTA_TCP_ADDR` is set (e.g. `:9001`), the API process also listens on **raw TCP** for a compact proprietary protocol used by constrained MCUs (implementation: `internal/transport/devotcp`).

**Environment (see `.env.example`)**

| Variable | Purpose |
|----------|---------|
| `DEVICE_OTA_TCP_ADDR` | Listen address for TCP OTA; empty disables the listener. |
| `OTA_DOWNLOAD_TOKEN_TTL` | Lifetime for download tokens issued on poll (Go duration, e.g. `20m`). |
| `PUBLIC_HTTP_BASE_URL` | Optional origin (no trailing slash), e.g. `https://api.example.com`, prepended to `/api/v1/device/firmware-download?token=…` in poll responses so devices receive an absolute URL. If unset, the poll payload contains a **relative** path only. |

**Session**: connect → **auth frame** (raw `Device` token in payload) → server **auth OK** or **error frame** → one or more **poll** / **report** frames on the same connection.

**Wire specification**: see **[Device TCP protocol](device-tcp-protocol.md)** (frame layout, message IDs, payloads, error codes). Source: `internal/transport/devotcp/protocol.go` and `handler.go`.

**Poll** (after auth): updates `last_seen_at`, logs `ota_poll`, resolves campaign offer (pending→offered as for HTTP), and when an update exists returns `update_available`, target version, raw 32-byte SHA-256 checksum, campaign and firmware UUIDs, download URL/path, and token expiry (Unix seconds). **Report** carries `campaign_id` and status (`downloaded` / `installed` / `failed`) plus optional error fields; server updates assignment state idempotently and logs `ota_report`.

Shutdown closes the TCP listener via `App.StopOTA()` (see `cmd/server/main.go`).

---

## Firmware (`/api/v1/projects/:projectID/firmwares`)

Project-scoped firmware binaries with metadata. Binaries are stored via an internal **object store** (local filesystem in development); API responses never expose filesystem paths—only opaque storage metadata and `checksum_sha256`.

| Method | Path | Body / query | Permission |
|--------|------|----------------|------------|
| GET | `/firmwares` | Pagination `page`, `page_size` (max 100), `sort`: `created_at`, `-created_at` (default), `version`, `-version` (semver columns when parseable, then `version_normalized`; non-semver rows sort after semver for `version` / `-version`) | `firmware.read` |
| POST | `/firmwares` | **multipart**: `file` (required), `version` (required), `changelog` (optional), `device_type_ids` (required JSON array of UUID strings, e.g. `["uuid"]`) — at least one compatible device type (predefined or project custom) | `firmware.upload` |
| GET | `/firmwares/:firmwareID` | — (metadata + `compatible_device_type_ids`; no raw storage path) | `firmware.read` |
| GET | `/firmwares/:firmwareID/download` | — streams `application/octet-stream`; headers `X-Checksum-Sha256`, `Content-Length` | `firmware.read` |
| DELETE | `/firmwares/:firmwareID` | — soft-deletes row and removes blob from object store (best-effort) | `firmware.upload` |

**Versioning**

- **`version`**: stored exactly as provided (trimmed for leading/trailing spaces in input validation only); shown in API as uploaded.
- **`version_normalized`**: `strings.ToLower(strings.TrimSpace(version))` used for **duplicate detection** per project (partial unique index where `deleted_at IS NULL`). Example: `1.0.0` and `  1.0.0  ` collide.
- **Semver**: when the string parses as [SemVer 2.0.0](https://semver.org/) (via `github.com/Masterminds/semver`), `semver_major` / `semver_minor` / `semver_patch` and `semver_prerelease` are populated for list ordering. Non-semver strings (e.g. `REL-2024-11`) still persist; those columns stay null and sort after semver tuples for `sort=version` / `sort=-version` (best-effort; prerelease ordering vs release is not fully modeled in SQL).

**Upload limits**: max body size is capped by `FIRMWARE_MAX_UPLOAD_BYTES` (default 64 MiB; see `.env.example`). Server computes **SHA-256** after size-limited buffering, then writes the object with a deterministic internal key `projects/{projectID}/firmware/{firmwareID}/blob`.

**Errors**: duplicate normalized version → **`409`** `firmware_version_exists`. Oversized upload → **`400`**.

**Audit**: `firmware_uploaded`, `firmware_downloaded`, `firmware_deleted` on the project audit stream (actor, firmware id, checksum/size on upload/download).

---

## OTA campaigns (`/api/v1/projects/:projectID/campaigns`)

Rollouts tie a **firmware** to a **target scope** (device groups and/or explicit device IDs). Only devices whose **device type** is compatible with the firmware are included. Archived projects reject creation.

| Method | Path | Body | Permission |
|--------|------|------|------------|
| GET | `/campaigns` | `page`, `page_size`, `sort` (default `-created_at` via list impl) | `campaign.read` |
| POST | `/campaigns` | JSON below | `campaign.create` |
| GET | `/campaigns/:campaignID` | — (detail + **progress** counts) | `campaign.read` |
| POST | `/campaigns/:campaignID/pause` | — | `campaign.pause` |
| POST | `/campaigns/:campaignID/resume` | — | `campaign.update` |
| POST | `/campaigns/:campaignID/cancel` | — | `campaign.cancel` |

**Create body**

```json
{
  "name": "Pilot rollout",
  "firmware_id": "uuid",
  "rollout_kind": "immediate | time_scheduled | percentage",
  "rollout_percent": 20,
  "scheduled_start_at": "2026-01-15T12:00:00Z",
  "device_group_ids": ["uuid"],
  "explicit_device_ids": ["uuid"]
}
```

- **`rollout_kind`**
  - **`immediate`**: activate as soon as created (unless `scheduled_start_at` is in the future — then status is **scheduled** until due).
  - **`time_scheduled`**: requires **`scheduled_start_at`** (UTC). Status **scheduled** until that time, then **active** (background tick; idempotent `UPDATE … WHERE status='scheduled' AND scheduled_start_at <= now()`).
  - **`percentage`**: requires **`rollout_percent`** (1–100). After resolving **compatible** devices, the server sorts device UUIDs **ascending** and takes the first **ceil(n × percent / 100)** devices — **deterministic and stable** for the same scope and percent (not random per request).
- **Targets**: union of devices in `device_group_ids` and `explicit_device_ids` (deduped), then compatibility-filtered. Empty compatible set → **`400`**.

**States**: `draft` (reserved), `scheduled`, `active`, `paused`, `cancelled`, `completed`. Valid transitions are enforced in code (`internal/domain/campaign/model/state.go`). **Pause** stops new OTA offers (poll skips non-active). **Cancel** ends the campaign. **Completed** when every assignment is `installed` or `failed`.

**Progress** (`GET …/:campaignID`): `target_count`, per-status counts (`pending`, `offered`, `downloaded`, `installed`, `failed`), `completion_percent` = \((installed+failed)/target×100\), `success_percent` = \(installed/target×100\).

**Project delete**: soft-delete is blocked while any campaign is **not** `completed` or `cancelled` (includes `draft`, `scheduled`, `active`, `paused`).

**Audit**: `campaign_created`, `campaign_paused`, `campaign_resumed`, `campaign_cancelled`.

---

## Permission keys (reference)

Registered keys cover project/member/role management, **devices**, **firmware**, **campaigns** (`campaign.read`, `campaign.create`, `campaign.update`, `campaign.pause`, `campaign.cancel`), `audit.read`, `dashboard.read`. See `internal/domain/rbac/permission/registry.go` for the canonical list.
