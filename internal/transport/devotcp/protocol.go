package devotcp

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	frameHeaderLen = 16
	maxPayloadLen  = 8192

	magic0   = 0x46
	magic1   = 0x52
	magic2   = 0x4d
	magic3   = 0x31
	protoVer = 1
)

const (
	MsgAuthReq   uint16 = 1
	MsgPollReq   uint16 = 2
	MsgReportReq uint16 = 3

	MsgAuthOK     uint16 = 128
	MsgPollResp   uint16 = 129
	MsgReportResp uint16 = 130
	MsgErr        uint16 = 255
)

const (
	ErrCodeUnauthorized      uint32 = 1
	ErrCodeBlocked           uint32 = 2
	ErrCodeRevokedOrDisabled uint32 = 3
	ErrCodeBadPayload        uint32 = 4
	ErrCodeInternal          uint32 = 5
)

var (
	ErrBadFrame      = errors.New("devotcp: bad frame")
	ErrPayloadTooBig = errors.New("devotcp: payload too large")
)

type Frame struct {
	MsgType uint16
	ReqID   uint32
	Payload []byte
}

func readFrame(r io.Reader) (Frame, error) {
	var hdr [frameHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	if hdr[0] != magic0 || hdr[1] != magic1 || hdr[2] != magic2 || hdr[3] != magic3 {
		return Frame{}, ErrBadFrame
	}
	if hdr[4] != protoVer {
		return Frame{}, ErrBadFrame
	}
	msgType := binary.BigEndian.Uint16(hdr[6:8])
	reqID := binary.BigEndian.Uint32(hdr[8:12])
	payloadLen := binary.BigEndian.Uint32(hdr[12:16])
	if payloadLen > maxPayloadLen {
		return Frame{}, ErrPayloadTooBig
	}
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
	}
	return Frame{MsgType: msgType, ReqID: reqID, Payload: payload}, nil
}

func writeFrame(w io.Writer, msgType uint16, reqID uint32, payload []byte) error {
	if len(payload) > maxPayloadLen {
		return ErrPayloadTooBig
	}
	var hdr [frameHeaderLen]byte
	hdr[0], hdr[1], hdr[2], hdr[3] = magic0, magic1, magic2, magic3
	hdr[4] = protoVer
	hdr[5] = 0
	binary.BigEndian.PutUint16(hdr[6:8], msgType)
	binary.BigEndian.PutUint32(hdr[8:12], reqID)
	binary.BigEndian.PutUint32(hdr[12:16], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func writeErr(w io.Writer, reqID uint32, code uint32, msg string) error {
	if len(msg) > 512 {
		msg = msg[:512]
	}
	if !utf8.ValidString(msg) {
		msg = strings.ToValidUTF8(msg, "")
	}
	b := make([]byte, 4+2+len(msg))
	binary.BigEndian.PutUint32(b[0:4], code)
	binary.BigEndian.PutUint16(b[4:6], uint16(len(msg)))
	copy(b[6:], msg)
	return writeFrame(w, MsgErr, reqID, b)
}

func decodeAuthPayload(p []byte) (token string, err error) {
	if len(p) < 2 {
		return "", ErrBadFrame
	}
	n := int(binary.BigEndian.Uint16(p[0:2]))
	if n < 0 || 2+n > len(p) {
		return "", ErrBadFrame
	}
	raw := string(p[2 : 2+n])
	if !utf8.ValidString(raw) {
		raw = strings.ToValidUTF8(raw, "")
	}
	return strings.TrimSpace(raw), nil
}

func decodePollPayload(p []byte) (currentVersion, meta string, err error) {
	if len(p) < 2 {
		return "", "", ErrBadFrame
	}
	n := int(binary.BigEndian.Uint16(p[0:2]))
	if 2+n > len(p) {
		return "", "", ErrBadFrame
	}
	cur := string(p[2 : 2+n])
	off := 2 + n
	if off+2 > len(p) {
		return strings.TrimSpace(cur), "", nil
	}
	m := int(binary.BigEndian.Uint16(p[off : off+2]))
	if off+2+m > len(p) {
		return "", "", ErrBadFrame
	}
	metaStr := string(p[off+2 : off+2+m])
	if !utf8.ValidString(cur) {
		cur = strings.ToValidUTF8(cur, "")
	}
	if !utf8.ValidString(metaStr) {
		metaStr = strings.ToValidUTF8(metaStr, "")
	}
	return strings.TrimSpace(cur), strings.TrimSpace(metaStr), nil
}

func decodeReportPayload(p []byte) (campaignID uuid.UUID, status byte, errCode uint16, errMsg, currentVer string, err error) {
	if len(p) < 16+1+2+2 {
		return uuid.Nil, 0, 0, "", "", ErrBadFrame
	}
	copy(campaignID[:], p[0:16])
	status = p[16]
	errCode = binary.BigEndian.Uint16(p[17:19])
	off := 19
	if off+2 > len(p) {
		return uuid.Nil, 0, 0, "", "", ErrBadFrame
	}
	eml := int(binary.BigEndian.Uint16(p[off : off+2]))
	off += 2
	if off+eml > len(p) {
		return uuid.Nil, 0, 0, "", "", ErrBadFrame
	}
	errMsg = string(p[off : off+eml])
	off += eml
	if off+2 > len(p) {
		return campaignID, status, errCode, errMsg, "", nil
	}
	cvl := int(binary.BigEndian.Uint16(p[off : off+2]))
	off += 2
	if off+cvl > len(p) {
		return uuid.Nil, 0, 0, "", "", ErrBadFrame
	}
	currentVer = string(p[off : off+cvl])
	if !utf8.ValidString(errMsg) {
		errMsg = strings.ToValidUTF8(errMsg, "")
	}
	if !utf8.ValidString(currentVer) {
		currentVer = strings.ToValidUTF8(currentVer, "")
	}
	return campaignID, status, errCode, strings.TrimSpace(errMsg), strings.TrimSpace(currentVer), nil
}

func encodePollResponse(updateAvailable bool, targetVersion, downloadURL string, checksumSHA256 [32]byte, campaignID, firmwareID uuid.UUID, expiresUnix uint32) []byte {
	var flag byte
	if updateAvailable {
		flag = 1
	}
	tv := []byte(targetVersion)
	du := []byte(downloadURL)
	if len(tv) > maxPayloadLen/4 || len(du) > maxPayloadLen {
		return nil
	}
	out := make([]byte, 1+2+len(tv)+32+16+16+2+len(du)+4)
	o := 0
	out[o] = flag
	o++
	binary.BigEndian.PutUint16(out[o:o+2], uint16(len(tv)))
	o += 2
	copy(out[o:], tv)
	o += len(tv)
	copy(out[o:], checksumSHA256[:])
	o += 32
	copy(out[o:], campaignID[:])
	o += 16
	copy(out[o:], firmwareID[:])
	o += 16
	binary.BigEndian.PutUint16(out[o:o+2], uint16(len(du)))
	o += 2
	copy(out[o:], du)
	o += len(du)
	binary.BigEndian.PutUint32(out[o:o+4], expiresUnix)
	return out
}

func sha256HexTo32(s string) ([32]byte, error) {
	var z [32]byte
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != 32 {
		return z, ErrBadFrame
	}
	copy(z[:], b)
	return z, nil
}
