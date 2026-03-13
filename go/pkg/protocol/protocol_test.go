package protocol

import (
	"testing"
)

// testDeviceGid is a test-only device GID.
var testDeviceGid = [6]byte{0x13, 0xCE, 0x3C, 0xC5, 0x3E, 0x5A}

func TestEncodeTemp(t *testing.T) {
	tests := []struct {
		temp    float64
		want    byte
		wantErr bool
	}{
		{28.0, 28, false},
		{28.5, 156, false},  // 0x9C
		{33.0, 33, false},
		{33.5, 161, false},  // 0xA1
		{34.0, 34, false},
		{34.5, 162, false},  // 0xA2
		{48.0, 48, false},
		{27.0, 0, true},     // below min
		{49.0, 0, true},     // above max
		{33.3, 0, true},     // not 0.5 step
	}
	for _, tt := range tests {
		got, err := EncodeTemp(tt.temp)
		if tt.wantErr {
			if err == nil {
				t.Errorf("EncodeTemp(%.1f) should error", tt.temp)
			}
			continue
		}
		if err != nil {
			t.Errorf("EncodeTemp(%.1f) unexpected error: %v", tt.temp, err)
			continue
		}
		if got != tt.want {
			t.Errorf("EncodeTemp(%.1f) = %d (0x%02X), want %d (0x%02X)", tt.temp, got, got, tt.want, tt.want)
		}
	}
}

func TestDecodeTemp(t *testing.T) {
	tests := []struct {
		b    byte
		want float64
	}{
		{28, 28.0},
		{156, 28.5},  // 0x9C
		{33, 33.0},
		{161, 33.5},  // 0xA1
		{34, 34.0},
		{162, 34.5},  // 0xA2
		{48, 48.0},
	}
	for _, tt := range tests {
		got := DecodeTemp(tt.b)
		if got != tt.want {
			t.Errorf("DecodeTemp(%d) = %.1f, want %.1f", tt.b, got, tt.want)
		}
	}
}

func TestEncodeDecode(t *testing.T) {
	// Round-trip test for all valid temperatures
	for temp := TempMin; temp <= TempMax; temp += 0.5 {
		encoded, err := EncodeTemp(temp)
		if err != nil {
			t.Fatalf("EncodeTemp(%.1f) error: %v", temp, err)
		}
		decoded := DecodeTemp(encoded)
		if decoded != temp {
			t.Errorf("Round-trip failed: %.1f -> %d -> %.1f", temp, encoded, decoded)
		}
	}
}

func TestCalcChecksum(t *testing.T) {
	// Example from PROTOCOL.md: heat left 33→34°C
	pkt := []byte{
		0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21,
		0x1C, 0x23, 0x22, 0x1C, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0x3A,
	}
	got := CalcChecksum(pkt)
	if got != 0x3A {
		t.Errorf("CalcChecksum = 0x%02X, want 0x3A", got)
	}

	// Handshake with 33.5°C example
	pkt2 := []byte{
		0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21,
		0x1C, 0x23, 0xA1, 0x1C, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0xB9,
	}
	got2 := CalcChecksum(pkt2)
	if got2 != 0xB9 {
		t.Errorf("CalcChecksum = 0x%02X, want 0xB9", got2)
	}

	// Initial pairing handshake
	pkt3 := []byte{
		0xB2, 0x01, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0xDF,
	}
	got3 := CalcChecksum(pkt3)
	if got3 != 0xDF {
		t.Errorf("CalcChecksum = 0x%02X, want 0xDF", got3)
	}
}

func TestBuildHandshake(t *testing.T) {
	pkt := BuildHandshake()

	if pkt[0] != STX {
		t.Errorf("STX = 0x%02X, want 0x%02X", pkt[0], STX)
	}
	if pkt[1] != DirHandshake {
		t.Errorf("Dir = 0x%02X, want 0x%02X", pkt[1], DirHandshake)
	}
	for i := 2; i < 19; i++ {
		if pkt[i] != Pad {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, pkt[i], Pad)
		}
	}
	// Verify checksum
	if pkt[19] != CalcChecksum(pkt[:]) {
		t.Errorf("Checksum mismatch: got 0x%02X, calc 0x%02X", pkt[19], CalcChecksum(pkt[:]))
	}
	// Known value: 0xDF
	if pkt[19] != 0xDF {
		t.Errorf("Handshake checksum = 0x%02X, want 0xDF", pkt[19])
	}
}

func TestBuildHandshakeWithKey(t *testing.T) {
	gid := testDeviceGid
	pkt := BuildHandshakeWithKey(gid)

	if pkt[0] != STX {
		t.Errorf("STX = 0x%02X", pkt[0])
	}
	if pkt[1] != DirHandshake {
		t.Errorf("Dir = 0x%02X", pkt[1])
	}
	// Check DeviceGid at positions [2..7]
	for i := 0; i < 6; i++ {
		if pkt[2+i] != gid[i] {
			t.Errorf("gid[%d] = 0x%02X, want 0x%02X", i, pkt[2+i], gid[i])
		}
	}
	// Checksum valid
	if pkt[19] != CalcChecksum(pkt[:]) {
		t.Errorf("Checksum mismatch")
	}
}

func TestBuildHeat(t *testing.T) {
	// Heat left 33°C → 34°C (from PROTOCOL.md example)
	leftCur := byte(0x21)  // 33°C
	rightCur := byte(0x1C) // 28°C
	leftTgt := byte(0x22)  // 34°C
	rightTgt := byte(0x1C) // 28°C

	pkt := BuildHeat(SideLeft, leftCur, rightCur, leftTgt, rightTgt)

	expected := []byte{
		0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21,
		0x1C, 0x23, 0x22, 0x1C, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0x3A,
	}

	for i := 0; i < PacketSize; i++ {
		if pkt[i] != expected[i] {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, pkt[i], expected[i])
		}
	}
}

func TestBuildHeat335(t *testing.T) {
	// Heat left → 33.5°C (from PROTOCOL.md example)
	leftCur := byte(0x21)  // 33°C
	rightCur := byte(0x1C) // 28°C
	leftTgt := byte(0xA1)  // 33.5°C
	rightTgt := byte(0x1C) // 28°C

	pkt := BuildHeat(SideLeft, leftCur, rightCur, leftTgt, rightTgt)

	expected := []byte{
		0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21,
		0x1C, 0x23, 0xA1, 0x1C, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0xB9,
	}

	for i := 0; i < PacketSize; i++ {
		if pkt[i] != expected[i] {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, pkt[i], expected[i])
		}
	}
}

func TestBuildPowerOn(t *testing.T) {
	pkt := BuildPowerOn(0x21, 0x1C)

	if pkt[0] != STX {
		t.Errorf("STX wrong")
	}
	if pkt[2] != ModePower {
		t.Errorf("Mode = 0x%02X, want 0x%02X", pkt[2], ModePower)
	}
	if pkt[3] != SideBoth {
		t.Errorf("Side = 0x%02X, want 0x%02X", pkt[3], SideBoth)
	}
	if pkt[6] != 0x2B {
		t.Errorf("ON marker = 0x%02X, want 0x2B", pkt[6])
	}
	if pkt[19] != CalcChecksum(pkt[:]) {
		t.Errorf("Checksum mismatch")
	}
}

func TestBuildPowerOff(t *testing.T) {
	pkt := BuildPowerOff(0x21, 0x1C)

	if pkt[0] != STX {
		t.Errorf("STX wrong")
	}
	if pkt[2] != ModePower {
		t.Errorf("Mode wrong")
	}
	if pkt[6] != 0xAB {
		t.Errorf("OFF marker = 0x%02X, want 0xAB", pkt[6])
	}
	// Right temp should appear at [11], [13] per PROTOCOL.md
	if pkt[11] != 0x1C {
		t.Errorf("byte[11] = 0x%02X, want 0x1C", pkt[11])
	}
	if pkt[13] != 0x1C {
		t.Errorf("byte[13] = 0x%02X, want 0x1C", pkt[13])
	}
	if pkt[19] != CalcChecksum(pkt[:]) {
		t.Errorf("Checksum mismatch")
	}
}

func TestParseStatus(t *testing.T) {
	// Simulate a STATUS packet: mode=HEAT, side=both, left=33°C, right=28°C
	pkt := []byte{
		0xB2, 0x00, 0x01, 0x06, 0x22, 0x02, 0x25, 0x21,
		0x1C, 0x23, 0x21, 0x1C, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0x00,
	}
	pkt[19] = CalcChecksum(pkt)

	st, err := ParseStatus(pkt)
	if err != nil {
		t.Fatalf("ParseStatus error: %v", err)
	}

	if st.Mode != ModeHeat {
		t.Errorf("Mode = 0x%02X, want HEAT", st.Mode)
	}
	if st.ModeName != "HEAT" {
		t.Errorf("ModeName = %q", st.ModeName)
	}
	if st.Side != SideBoth {
		t.Errorf("Side = 0x%02X", st.Side)
	}
	if st.LeftCurrent != 33.0 {
		t.Errorf("LeftCurrent = %.1f, want 33.0", st.LeftCurrent)
	}
	if st.RightCurrent != 28.0 {
		t.Errorf("RightCurrent = %.1f, want 28.0", st.RightCurrent)
	}
	if st.PoweredOff {
		t.Error("PoweredOff should be false")
	}
}

func TestParseStatusPowerOff(t *testing.T) {
	pkt := []byte{
		0xB2, 0x00, 0x06, 0x06, 0x00, 0xFE, 0x00, 0x00,
		0x00, 0xFE, 0x00, 0x00, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0x00,
	}
	pkt[19] = CalcChecksum(pkt)

	st, err := ParseStatus(pkt)
	if err != nil {
		t.Fatalf("ParseStatus error: %v", err)
	}
	if !st.PoweredOff {
		t.Error("PoweredOff should be true")
	}
}

func TestParseStatusInvalid(t *testing.T) {
	// Too short
	_, err := ParseStatus([]byte{0xB2, 0x00})
	if err == nil {
		t.Error("should error on short packet")
	}

	// Wrong STX
	pkt := make([]byte, 20)
	pkt[0] = 0xAA
	_, err = ParseStatus(pkt)
	if err == nil {
		t.Error("should error on wrong STX")
	}

	// Not a STATUS (direction != 0x00)
	pkt[0] = STX
	pkt[1] = DirAppToMat
	_, err = ParseStatus(pkt)
	if err == nil {
		t.Error("should error on non-STATUS direction")
	}
}

func TestParseAuthResponse(t *testing.T) {
	// Authenticated response
	pkt := []byte{
		0xB2, 0xF1, 0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xE3,
	}

	authType, err := ParseAuthResponse(pkt)
	if err != nil {
		t.Fatalf("ParseAuthResponse error: %v", err)
	}
	if authType != 0x02 {
		t.Errorf("auth type = 0x%02X, want 0x02", authType)
	}
}

func TestParseDeviceGid(t *testing.T) {
	// Pairing response with DeviceGid
	pkt := []byte{
		0xB2, 0xF1, 0x01, 0x13, 0xCE, 0x3C, 0xC5, 0x3E,
		0x5A, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0x00,
	}

	gid, err := ParseDeviceGid(pkt)
	if err != nil {
		t.Fatalf("ParseDeviceGid error: %v", err)
	}
	if gid != testDeviceGid {
		t.Errorf("gid = %X, want %X", gid, testDeviceGid)
	}
}

func TestParseDeviceGidHex(t *testing.T) {
	gid, err := ParseDeviceGidHex("13CE3CC53E5A")
	if err != nil {
		t.Fatalf("ParseDeviceGidHex error: %v", err)
	}
	if gid != testDeviceGid {
		t.Errorf("gid = %X, want %X", gid, testDeviceGid)
	}

	// Invalid cases
	_, err = ParseDeviceGidHex("ZZZZ")
	if err == nil {
		t.Error("should error on invalid hex")
	}
	_, err = ParseDeviceGidHex("AABB")
	if err == nil {
		t.Error("should error on wrong length")
	}
}

func TestFormatPacket(t *testing.T) {
	pkt := []byte{0xB2, 0x01, 0xFE}
	got := FormatPacket(pkt)
	want := "B2 01 FE"
	if got != want {
		t.Errorf("FormatPacket = %q, want %q", got, want)
	}
}
