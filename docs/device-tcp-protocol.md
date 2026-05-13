# Device OTA binary TCP protocol (FRM1)

This document specifies the **on-the-wire** format for the optional device OTA service exposed when `DEVICE_OTA_TCP_ADDR` is set. The reference implementation is in `internal/transport/devotcp/` (`protocol.go`, `handler.go`, `server.go`).

For HTTP behavior (download URL, token semantics), see [HTTP API reference](api.md) (sections *Device OTA (binary TCP)* and *Firmware download (short-lived OTA token)*).

---

## Transport

- **Protocol**: TCP, binary frames (no TLS in the reference server; use a sidecar, VPN, or mutual TLS at the edge in production if needed).
- **Byte order**: All multi-byte integers are **big-endian** (network byte order).
- **Strings**: Length-prefixed segments use **UTF-8** unless noted otherwise.
- **Maximum payload size**: **8192** bytes per frame (after the 16-byte header). Oversized frames are rejected.

---

## Connection lifecycle

1. Client opens a TCP connection to the configured listen address.
2. Client sends exactly one **Auth request** as the first frame. Any other first frame receives **MsgErr** with a bad-payload style error.
3. On success, server sends **Auth OK** (empty payload). On failure, server sends **MsgErr** and may close the connection; clients should not assume further reads.
4. After auth, the client may send zero or more **Poll** and/or **Report** frames in any order. Each request should use a **request ID** the client can correlate with responses (the server echoes `req_id` on the matching response or error frame).
5. The server sets read deadlines (implementation-defined: initial connection window, then per-frame window). Idle or malformed connections are closed.

---

## Frame format

Every message is:

| Offset | Size | Field | Description |
|--------|------|--------|-------------|
| 0 | 4 | `magic` | Must be `0x46 0x52 0x4d 0x31` (ASCII **FRM1**). |
| 4 | 1 | `version` | Protocol version; must be **1**. |
| 5 | 1 | `flags` | Reserved; must be **0**. |
| 6 | 2 | `msg_type` | Message type (see tables below). |
| 8 | 4 | `req_id` | Opaque correlation id chosen by the sender; echoed on the reply. |
| 12 | 4 | `payload_len` | Length of the payload in bytes (0 … 8192). |
| 16 | `payload_len` | `payload` | Message body. |

Total frame size = **16 + payload_len**.

---

## Message types

### Client → server (requests)

| Value | Name | Payload |
|-------|------|---------|
| 1 | `MsgAuthReq` | See [Auth payload](#auth-payload-msgauthreq). |
| 2 | `MsgPollReq` | See [Poll request payload](#poll-request-payload-msgpollreq). |
| 3 | `MsgReportReq` | See [Report request payload](#report-request-payload-msgreportreq). |

### Server → client (responses)

| Value | Name | Payload |
|-------|------|---------|
| 128 | `MsgAuthOK` | Empty (length 0). |
| 129 | `MsgPollResp` | See [Poll response payload](#poll-response-payload-msgpollresp). |
| 130 | `MsgReportResp` | One byte: **0x01** = acknowledged. |
| 255 | `MsgErr` | See [Error payload](#error-payload-msgerr). |

---

## Auth payload (`MsgAuthReq`)

| Field | Type | Description |
|-------|------|-------------|
| `token_len` | `uint16` | Byte length of the following token string. |
| `token` | `token_len` bytes UTF-8 | **Raw** device session token (same value as in HTTP `Authorization: Device <token>`). Leading/trailing ASCII whitespace is ignored after decode. |

The server stores only **SHA-256(token)** at rest; it hashes the provided string the same way as the HTTP device middleware.

---

## Poll request payload (`MsgPollReq`)

| Field | Type | Description |
|-------|------|-------------|
| `cur_ver_len` | `uint16` | Length of `current_firmware_version` UTF-8. |
| `current_firmware_version` | `cur_ver_len` bytes | Device-reported firmware version (may be empty length if unknown). |
| `meta_len` | `uint16` | Optional; if absent, treat as **0** (no metadata). If present, length of `meta`. |
| `meta` | `meta_len` bytes | Opaque UTF-8 metadata for future use; currently ignored by the server. |

**Layout rule**: If the payload is only `cur_ver_len` + version bytes and total length equals `2 + cur_ver_len`, then `meta_len` is **0** and there is no `meta`.

---

## Poll response payload (`MsgPollResp`)

| Field | Type | Description |
|-------|------|-------------|
| `update_available` | `uint8` | **0** = no update; **1** = fields below are present. |

If `update_available == 0`, the payload is **one byte** (`0x00`) and ends here.

If `update_available == 1`:

| Field | Type | Description |
|-------|------|-------------|
| `target_len` | `uint16` | Length of target firmware **version string** (UTF-8). |
| `target_version` | `target_len` bytes | Campaign firmware version string (not normalized). |
| `checksum_sha256` | **32 bytes** | Raw SHA-256 digest of the firmware blob (decoded from the 64-hex-character form stored in the database). |
| `campaign_id` | **16 bytes** | UUID in RFC 4122 binary form. |
| `firmware_id` | **16 bytes** | UUID in RFC 4122 binary form. |
| `url_len` | `uint16` | Length of `download_url` UTF-8. |
| `download_url` | `url_len` bytes | Either a path starting with `/api/v1/device/firmware-download?token=…` or an **absolute** URL if `PUBLIC_HTTP_BASE_URL` is configured on the server. The `token` query value is **64 hex characters** (opaque, short-lived, one-time use for HTTP GET). |
| `expires_unix` | `uint32` | Token expiry instant as **Unix seconds** (UTC). |

---

## Report request payload (`MsgReportReq`)

| Field | Type | Description |
|-------|------|-------------|
| `campaign_id` | **16 bytes** | UUID of the campaign (from poll response). |
| `status` | `uint8` | **1** = downloaded, **2** = installed, **3** = failed. |
| `err_code` | `uint16` | Optional application error code; use **0** if none. |
| `err_msg_len` | `uint16` | Length of `err_msg` UTF-8 (may be **0**). |
| `err_msg` | `err_msg_len` bytes | Human-readable diagnostic (truncated server-side if too long for storage). |
| `cur_ver_len` | `uint16` | Optional. If the payload ends after `err_msg` (length shorter than `off + 2` after consuming `err_msg`), the server treats current version as empty. Otherwise this is the length of `current_version` UTF-8. |
| `current_version` | `cur_ver_len` bytes | Device-reported firmware string for logging / twin updates (recommended on downloaded/failed). |

**Minimum payload length**: **21** bytes = `campaign_id` (16) + `status` (1) + `err_code` (2) + `err_msg_len` (2) with **`err_msg_len` = 0** and no `cur_ver_len` field (payload length exactly 21). To include an explicit **zero-length** current version field, append `cur_ver_len = 0` (2 bytes); total length **23**.

---

## Report response payload (`MsgReportResp`)

Single byte **0x01** = success. Business logic errors on apply are still returned as **MsgErr** where applicable.

---

## Error payload (`MsgErr`)

| Field | Type | Description |
|-------|------|-------------|
| `code` | `uint32` | Machine-readable code (see below). |
| `msg_len` | `uint16` | Length of `message` UTF-8 (capped server-side when building, typically ≤ 512). |
| `message` | `msg_len` bytes | Short English phrase for logs or UI. |

### Error codes (`code`)

| Code | Constant (Go) | Typical meaning |
|------|----------------|-----------------|
| 1 | `ErrCodeUnauthorized` | Unknown / invalid token (e.g. not found). |
| 2 | `ErrCodeBlocked` | Device blocked. |
| 3 | `ErrCodeRevokedOrDisabled` | Token revoked or polling disabled. |
| 4 | `ErrCodeBadPayload` | Malformed frame or payload, wrong first message, unknown `msg_type`, invalid report status. |
| 5 | `ErrCodeInternal` | Server-side failure (campaign, token issue, storage, etc.). |

---

## Server-side effects (behavioral contract)

- **Auth success**: Subsequent poll/report use the authenticated device’s `project_id` and `device_id`.
- **Poll**: Updates **last seen**, appends a connection log (`action` = `ota_poll`, endpoint `tcp/ota`), evaluates **active** campaign offers for **pending** or **offered** assignments; may transition **pending → offered**. If an offer exists and firmware service is configured, creates a **new OTA download token** and returns it inside `download_url`.
- **Report**: Applies campaign assignment progress (`downloaded` / `installed` / `failed`) for the given `campaign_id`; may update device firmware version on **installed**; appends connection log (`ota_report`). Idempotent transitions are implemented in the campaign repository layer.
- **HTTP download**: Device performs `GET` on the returned URL **without** the long-lived device token; the URL carries only the short-lived hex token.

---

## Implementation reference

| Concern | Package / file |
|---------|----------------|
| Frame codec, payload layouts | `internal/transport/devotcp/protocol.go` |
| Session rules, auth, poll/report orchestration | `internal/transport/devotcp/handler.go` |
| TCP accept loop | `internal/transport/devotcp/server.go` |
| Unit tests (encoding) | `internal/transport/devotcp/protocol_test.go` |

When this document and the code disagree, **the code wins** until the document is updated.
