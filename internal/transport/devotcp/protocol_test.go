package devotcp

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
)

func TestProtocolAuthRoundTrip(t *testing.T) {
	token := "  my-device-token  "
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint16(len(token)))
	b.WriteString(token)
	got, err := decodeAuthPayload(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-device-token" {
		t.Fatalf("got %q", got)
	}
}

func TestProtocolPollResponseLayout(t *testing.T) {
	cid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	var sum [32]byte
	for i := range sum {
		sum[i] = byte(i)
	}
	body := encodePollResponse(true, "1.2.3", "https://x/f?token=abc", sum, cid, fid, 1700000000)
	if len(body) < 1+2+5+32+16+16+2+len("https://x/f?token=abc")+4 {
		t.Fatalf("short body %d", len(body))
	}
	if body[0] != 1 {
		t.Fatalf("flag %d", body[0])
	}
}

func TestProtocolReportRoundTrip(t *testing.T) {
	cid := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	var buf bytes.Buffer
	buf.Write(cid[:])
	buf.WriteByte(1) // downloaded
	_ = binary.Write(&buf, binary.BigEndian, uint16(42))
	_ = binary.Write(&buf, binary.BigEndian, uint16(4))
	buf.WriteString("oops")
	_ = binary.Write(&buf, binary.BigEndian, uint16(5))
	buf.WriteString("0.9.0")

	gotCID, st, ec, em, cv, err := decodeReportPayload(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if gotCID != cid || st != 1 || ec != 42 || em != "oops" || cv != "0.9.0" {
		t.Fatalf("got %v %d %d %q %q", gotCID, st, ec, em, cv)
	}
}
