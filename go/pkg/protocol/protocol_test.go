package protocol

import (
	"testing"
)

// testDeviceGid는 테스트 전용 DeviceGid입니다.
// 실제 기기의 GID가 아니며, 테스트에서만 사용됩니다.
var testDeviceGid = [6]byte{0x13, 0xCE, 0x3C, 0xC5, 0x3E, 0x5A}

// TestEncodeTemp는 온도→바이트 인코딩을 테스트합니다.
//
// 테스트 케이스:
//   - 정수 온도: 28.0→28, 33.0→33, 34.0→34, 48.0→48
//   - 소수점 온도: 28.5→156(0x9C), 33.5→161(0xA1), 34.5→162(0xA2)
//   - 범위 초과: 27.0(최저 미만), 49.0(최고 초과)
//   - 0.5단위 아님: 33.3 (오류 발생해야 함)
func TestEncodeTemp(t *testing.T) {
	tests := []struct {
		temp    float64
		want    byte
		wantErr bool
	}{
		{28.0, 28, false},
		{28.5, 156, false},  // 0x9C = 28.5 + 127.5
		{33.0, 33, false},
		{33.5, 161, false},  // 0xA1 = 33.5 + 127.5
		{34.0, 34, false},
		{34.5, 162, false},  // 0xA2 = 34.5 + 127.5
		{48.0, 48, false},
		{27.0, 0, true},     // 최저 온도(28.0) 미만 → 오류
		{49.0, 0, true},     // 최고 온도(48.0) 초과 → 오류
		{33.3, 0, true},     // 0.5°C 단위가 아님 → 오류
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

// TestDecodeTemp는 바이트→온도 디코딩을 테스트합니다.
//
// EncodeTemp의 역변환이 올바르게 동작하는지 검증합니다.
// 바이트 값 > 127이면 소수점 온도(byte - 127.5), 아니면 정수 온도.
func TestDecodeTemp(t *testing.T) {
	tests := []struct {
		b    byte
		want float64
	}{
		{28, 28.0},
		{156, 28.5},  // 0x9C → 156 - 127.5 = 28.5
		{33, 33.0},
		{161, 33.5},  // 0xA1 → 161 - 127.5 = 33.5
		{34, 34.0},
		{162, 34.5},  // 0xA2 → 162 - 127.5 = 34.5
		{48, 48.0},
	}
	for _, tt := range tests {
		got := DecodeTemp(tt.b)
		if got != tt.want {
			t.Errorf("DecodeTemp(%d) = %.1f, want %.1f", tt.b, got, tt.want)
		}
	}
}

// TestEncodeDecode는 모든 유효한 온도에 대해 인코딩→디코딩 왕복(round-trip) 테스트를 수행합니다.
//
// 28.0°C부터 48.0°C까지 0.5°C 단위로 모든 온도를 인코딩한 후
// 다시 디코딩하여 원래 온도와 일치하는지 검증합니다.
// 이 테스트가 통과하면 EncodeTemp와 DecodeTemp가 완벽한 역함수 관계임을 보장합니다.
func TestEncodeDecode(t *testing.T) {
	for temp := TempMin; temp <= TempMax; temp += 0.5 {
		encoded, err := EncodeTemp(temp)
		if err != nil {
			t.Fatalf("EncodeTemp(%.1f) error: %v", temp, err)
		}
		decoded := DecodeTemp(encoded)
		if decoded != temp {
			t.Errorf("왕복 변환 실패: %.1f → %d → %.1f", temp, encoded, decoded)
		}
	}
}

// TestCalcChecksum은 체크섬 계산을 검증합니다.
//
// PROTOCOL.md에 정의된 실제 패킷 예시를 사용하여 체크섬이 올바르게 계산되는지 확인합니다.
// 체크섬 = sum(bytes[1:19]) & 0xFF
func TestCalcChecksum(t *testing.T) {
	// 예시 1: PROTOCOL.md의 왼쪽 33°C→34°C 난방 명령 패킷
	pkt := []byte{
		0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21,
		0x1C, 0x23, 0x22, 0x1C, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0x3A,
	}
	got := CalcChecksum(pkt)
	if got != 0x3A {
		t.Errorf("CalcChecksum = 0x%02X, want 0x3A", got)
	}

	// 예시 2: 왼쪽 33.5°C 설정 패킷 (소수점 온도 포함)
	pkt2 := []byte{
		0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21,
		0x1C, 0x23, 0xA1, 0x1C, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0xB9,
	}
	got2 := CalcChecksum(pkt2)
	if got2 != 0xB9 {
		t.Errorf("CalcChecksum = 0x%02X, want 0xB9", got2)
	}

	// 예시 3: 최초 페어링 핸드셰이크 패킷 (DeviceGid 없음)
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

// TestBuildHandshake는 최초 페어링 핸드셰이크 패킷 생성을 테스트합니다.
//
// 검증 사항:
//   - byte[0] = STX (0xB2)
//   - byte[1] = DirHandshake (0x01)
//   - byte[2..18] = 모두 패딩(0xFE)
//   - byte[19] = 올바른 체크섬 (0xDF)
func TestBuildHandshake(t *testing.T) {
	pkt := BuildHandshake()

	if pkt[0] != STX {
		t.Errorf("STX = 0x%02X, want 0x%02X", pkt[0], STX)
	}
	if pkt[1] != DirHandshake {
		t.Errorf("Dir = 0x%02X, want 0x%02X", pkt[1], DirHandshake)
	}
	// byte[2]~byte[18]은 모두 패딩(0xFE)이어야 함
	for i := 2; i < 19; i++ {
		if pkt[i] != Pad {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, pkt[i], Pad)
		}
	}
	// 체크섬 검증
	if pkt[19] != CalcChecksum(pkt[:]) {
		t.Errorf("체크섬 불일치: got 0x%02X, calc 0x%02X", pkt[19], CalcChecksum(pkt[:]))
	}
	// 알려진 체크섬 값: 0xDF
	if pkt[19] != 0xDF {
		t.Errorf("핸드셰이크 체크섬 = 0x%02X, want 0xDF", pkt[19])
	}
}

// TestBuildHandshakeWithKey는 DeviceGid를 포함한 인증 핸드셰이크 패킷 생성을 테스트합니다.
//
// 검증 사항:
//   - byte[0] = STX, byte[1] = DirHandshake
//   - byte[2..7] = DeviceGid 6바이트가 올바르게 삽입되었는지
//   - byte[19] = 올바른 체크섬
func TestBuildHandshakeWithKey(t *testing.T) {
	gid := testDeviceGid
	pkt := BuildHandshakeWithKey(gid)

	if pkt[0] != STX {
		t.Errorf("STX = 0x%02X", pkt[0])
	}
	if pkt[1] != DirHandshake {
		t.Errorf("Dir = 0x%02X", pkt[1])
	}
	// DeviceGid가 byte[2]~byte[7]에 올바르게 들어갔는지 확인
	for i := 0; i < 6; i++ {
		if pkt[2+i] != gid[i] {
			t.Errorf("gid[%d] = 0x%02X, want 0x%02X", i, pkt[2+i], gid[i])
		}
	}
	// 체크섬 검증
	if pkt[19] != CalcChecksum(pkt[:]) {
		t.Errorf("체크섬 불일치")
	}
}

// TestBuildHeat는 PROTOCOL.md 예시 기반으로 난방 명령 패킷 생성을 테스트합니다.
//
// 시나리오: 왼쪽 매트를 33°C(0x21)에서 34°C(0x22)로 변경
// 오른쪽 매트는 28°C(0x1C) 유지
func TestBuildHeat(t *testing.T) {
	leftCur := byte(0x21)  // 현재 왼쪽 33°C
	rightCur := byte(0x1C) // 현재 오른쪽 28°C
	leftTgt := byte(0x22)  // 목표 왼쪽 34°C
	rightTgt := byte(0x1C) // 목표 오른쪽 28°C (변경 없음)

	pkt := BuildHeat(SideLeft, leftCur, rightCur, leftTgt, rightTgt)

	// PROTOCOL.md에 정의된 기대 패킷과 바이트 단위 비교
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

// TestBuildHeat335는 소수점 온도(33.5°C)를 포함한 난방 명령 패킷을 테스트합니다.
//
// 시나리오: 왼쪽 매트를 33°C에서 33.5°C(0xA1)로 변경
// 33.5 + 127.5 = 161 = 0xA1 인코딩 검증
func TestBuildHeat335(t *testing.T) {
	leftCur := byte(0x21)  // 현재 왼쪽 33°C
	rightCur := byte(0x1C) // 현재 오른쪽 28°C
	leftTgt := byte(0xA1)  // 목표 왼쪽 33.5°C (33.5 + 127.5 = 161 = 0xA1)
	rightTgt := byte(0x1C) // 목표 오른쪽 28°C (변경 없음)

	pkt := BuildHeat(SideLeft, leftCur, rightCur, leftTgt, rightTgt)

	// PROTOCOL.md에 정의된 기대 패킷
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

// TestBuildPowerOn은 전원 켜기 패킷 생성을 테스트합니다.
//
// 검증 사항:
//   - byte[0] = STX (0xB2)
//   - byte[2] = ModePower (0x06)
//   - byte[3] = SideBoth (0x06, 전원은 항상 양쪽)
//   - byte[6] = 0x2B (ON 마커)
//   - 체크섬이 올바른지
func TestBuildPowerOn(t *testing.T) {
	pkt := BuildPowerOn(0x21, 0x1C) // 왼쪽 33°C, 오른쪽 28°C

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
		t.Errorf("ON 마커 = 0x%02X, want 0x2B", pkt[6])
	}
	if pkt[19] != CalcChecksum(pkt[:]) {
		t.Errorf("체크섬 불일치")
	}
}

// TestBuildPowerOff는 전원 끄기 패킷 생성을 테스트합니다.
//
// 검증 사항:
//   - byte[2] = ModePower (0x06)
//   - byte[6] = 0xAB (OFF 마커)
//   - byte[11], byte[13] = 오른쪽 현재 온도 (PROTOCOL.md 사양)
//   - 체크섬이 올바른지
func TestBuildPowerOff(t *testing.T) {
	pkt := BuildPowerOff(0x21, 0x1C) // 왼쪽 33°C, 오른쪽 28°C

	if pkt[0] != STX {
		t.Errorf("STX wrong")
	}
	if pkt[2] != ModePower {
		t.Errorf("Mode wrong")
	}
	if pkt[6] != 0xAB {
		t.Errorf("OFF 마커 = 0x%02X, want 0xAB", pkt[6])
	}
	// PROTOCOL.md에 따르면 오른쪽 현재 온도가 byte[11]과 byte[13]에 들어가야 함
	if pkt[11] != 0x1C {
		t.Errorf("byte[11] = 0x%02X, want 0x1C", pkt[11])
	}
	if pkt[13] != 0x1C {
		t.Errorf("byte[13] = 0x%02X, want 0x1C", pkt[13])
	}
	if pkt[19] != CalcChecksum(pkt[:]) {
		t.Errorf("체크섬 불일치")
	}
}

// TestParseStatus는 STATUS 알림 패킷 파싱을 테스트합니다.
//
// 시뮬레이션 패킷: HEAT 모드, 양쪽 활성, 왼쪽 33°C, 오른쪽 28°C
// 파싱된 Status 구조체의 각 필드가 올바른지 검증합니다.
func TestParseStatus(t *testing.T) {
	// 테스트용 STATUS 패킷 생성: mode=HEAT, side=both, left=33°C, right=28°C
	pkt := []byte{
		0xB2, // [0] STX
		0x00, // [1] 매트→앱 방향 (STATUS)
		0x01, // [2] HEAT 모드
		0x06, // [3] 양쪽(both)
		0x22, // [4] 볼륨(상위4비트=2) + 수위(하위4비트=2=OK)
		0x02, // [5] 모드별 고정값
		0x25, // [6] 모드별 고정값
		0x21, // [7] 왼쪽 현재 온도 = 33°C
		0x1C, // [8] 오른쪽 현재 온도 = 28°C
		0x23, // [9] 모드별 고정값
		0x21, // [10] 왼쪽 목표 온도 = 33°C
		0x1C, // [11] 오른쪽 목표 온도 = 28°C
		0xFE, // [12] 패딩
		0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, // [13..18] 패딩
		0x00, // [19] 체크섬 (아래에서 계산)
	}
	pkt[19] = CalcChecksum(pkt) // 체크섬 계산

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

// TestParseStatusPowerOff는 전원 꺼짐 상태의 STATUS 패킷 파싱을 테스트합니다.
//
// Mode가 POWER(0x06)이고 목표 온도(byte[10], byte[11])가 모두 0이면
// PoweredOff = true로 판단되어야 합니다.
func TestParseStatusPowerOff(t *testing.T) {
	// 전원 꺼짐 상태 패킷: mode=POWER, 목표온도=0/0
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

// TestParseStatusChecksumMismatch는 체크섬이 잘못된 패킷이 거부되는지 테스트합니다.
func TestParseStatusChecksumMismatch(t *testing.T) {
	pkt := []byte{
		0xB2, 0x00, 0x01, 0x06, 0x22, 0x02, 0x25, 0x21,
		0x1C, 0x23, 0x21, 0x1C, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFE, 0xFE, 0xFE, 0x00,
	}
	pkt[19] = CalcChecksum(pkt) + 1 // 의도적으로 체크섬을 틀리게 설정
	_, err := ParseStatus(pkt)
	if err == nil {
		t.Error("체크섬 불일치 시 오류가 발생해야 함")
	}
}

// TestParseStatusInvalid는 잘못된 STATUS 패킷에 대한 오류 처리를 테스트합니다.
//
// 테스트 케이스:
//  1. 패킷이 20바이트 미만 → "packet too short" 오류
//  2. STX가 0xB2가 아님 → "invalid STX" 오류
//  3. 방향 바이트가 0x00(매트→앱)이 아님 → "not a STATUS packet" 오류
func TestParseStatusInvalid(t *testing.T) {
	// 케이스 1: 패킷이 너무 짧음
	_, err := ParseStatus([]byte{0xB2, 0x00})
	if err == nil {
		t.Error("짧은 패킷에서 오류가 발생해야 함")
	}

	// 케이스 2: STX가 잘못됨 (0xAA ≠ 0xB2)
	pkt := make([]byte, 20)
	pkt[0] = 0xAA
	_, err = ParseStatus(pkt)
	if err == nil {
		t.Error("잘못된 STX에서 오류가 발생해야 함")
	}

	// 케이스 3: STATUS가 아닌 방향 (0x80 = 앱→매트)
	pkt[0] = STX
	pkt[1] = DirAppToMat // 0x80 — STATUS는 0x00이어야 함
	_, err = ParseStatus(pkt)
	if err == nil {
		t.Error("STATUS가 아닌 방향에서 오류가 발생해야 함")
	}
}

// TestParseAuthResponse는 B2F1 인증 응답 패킷 파싱을 테스트합니다.
//
// 시뮬레이션: 인증 완료 응답 (authType = 0x02)
// byte[0]=0xB2, byte[1]=0xF1(인증 방향), byte[2]=0x02(인증 완료)
func TestParseAuthResponse(t *testing.T) {
	// 인증 완료 응답 패킷
	pkt := []byte{
		0xB2, 0xF1, 0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0x00,
	}
	pkt[19] = CalcChecksum(pkt)

	authType, err := ParseAuthResponse(pkt)
	if err != nil {
		t.Fatalf("ParseAuthResponse error: %v", err)
	}
	if authType != 0x02 {
		t.Errorf("auth type = 0x%02X, want 0x02 (인증 완료)", authType)
	}
}

// TestParseDeviceGid는 페어링 응답에서 DeviceGid 추출을 테스트합니다.
//
// 시뮬레이션: 페어링 응답 (authType = 0x01)
// byte[3..8]에 DeviceGid 6바이트가 포함되어 있어야 합니다.
func TestParseDeviceGid(t *testing.T) {
	// 페어링 응답 패킷: authType=0x01, DeviceGid=13CE3CC53E5A
	pkt := []byte{
		0xB2, 0xF1, 0x01, 0x13, 0xCE, 0x3C, 0xC5, 0x3E,
		0x5A, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0x00,
	}
	pkt[19] = CalcChecksum(pkt)

	gid, err := ParseDeviceGid(pkt)
	if err != nil {
		t.Fatalf("ParseDeviceGid error: %v", err)
	}
	if gid != testDeviceGid {
		t.Errorf("gid = %X, want %X", gid, testDeviceGid)
	}
}

// TestParseDeviceGidHex는 16진수 문자열에서 DeviceGid 파싱을 테스트합니다.
//
// 테스트 케이스:
//   - 정상: "13CE3CC53E5A" → [6]byte{0x13, 0xCE, 0x3C, 0xC5, 0x3E, 0x5A}
//   - 유효하지 않은 16진수: "ZZZZ" → 오류
//   - 길이 오류: "AABB" (4자리 = 2바이트, 6바이트여야 함) → 오류
func TestParseDeviceGidHex(t *testing.T) {
	gid, err := ParseDeviceGidHex("13CE3CC53E5A")
	if err != nil {
		t.Fatalf("ParseDeviceGidHex error: %v", err)
	}
	if gid != testDeviceGid {
		t.Errorf("gid = %X, want %X", gid, testDeviceGid)
	}

	// 유효하지 않은 16진수 문자
	_, err = ParseDeviceGidHex("ZZZZ")
	if err == nil {
		t.Error("유효하지 않은 16진수에서 오류가 발생해야 함")
	}
	// 길이가 6바이트가 아님 (2바이트만 입력)
	_, err = ParseDeviceGidHex("AABB")
	if err == nil {
		t.Error("잘못된 길이에서 오류가 발생해야 함")
	}
}

// TestFormatDeviceGid는 DeviceGid를 hex 문자열로 포맷하는 것을 테스트합니다.
func TestFormatDeviceGid(t *testing.T) {
	gid := testDeviceGid
	got := FormatDeviceGid(gid)
	want := "13CE3CC53E5A"
	if got != want {
		t.Errorf("FormatDeviceGid = %q, want %q", got, want)
	}
}

// TestFormatPacket은 패킷의 16진수 포맷팅을 테스트합니다.
func TestFormatPacket(t *testing.T) {
	pkt := []byte{0xB2, 0x01, 0xFE}
	got := FormatPacket(pkt)
	want := "B2 01 FE"
	if got != want {
		t.Errorf("FormatPacket = %q, want %q", got, want)
	}
}

// TestParseAuthResponseChecksumMismatch는 체크섬이 잘못된 인증 응답 패킷에 대해
// ParseAuthResponse 자체의 체크섬 검증 로직이 오류를 반환하는지만 테스트합니다.
//
// 체크섬 불일치 오류가 ParseDeviceGid까지 전파되는지 여부는
// 아래 TestParseDeviceGidChecksumMismatch에서 별도로 검증합니다.
func TestParseAuthResponseChecksumMismatch(t *testing.T) {
	// 인증 완료 응답 패킷 (authType=0x02) — 체크섬을 의도적으로 틀리게 설정
	pkt := []byte{
		0xB2, 0xF1, 0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0x00,
	}
	pkt[19] = CalcChecksum(pkt) + 1 // 의도적으로 체크섬을 틀리게 설정
	_, err := ParseAuthResponse(pkt)
	if err == nil {
		t.Error("체크섬 불일치 시 오류가 발생해야 함")
	}
}

// TestParseDeviceGidChecksumMismatch는 체크섬이 잘못된 페어링 응답 패킷에서
// ParseDeviceGid가 오류를 올바르게 전파하는지 테스트합니다.
//
// ParseDeviceGid는 내부적으로 ParseAuthResponse를 호출하므로,
// 체크섬 불일치 오류가 호출 체인을 통해 전파되어야 합니다.
func TestParseDeviceGidChecksumMismatch(t *testing.T) {
	// 페어링 응답 패킷 (authType=0x01, DeviceGid 포함) — 체크섬을 의도적으로 틀리게 설정
	pkt := []byte{
		0xB2, 0xF1, 0x01, 0x13, 0xCE, 0x3C, 0xC5, 0x3E,
		0x5A, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0x00,
	}
	pkt[19] = CalcChecksum(pkt) + 1 // 의도적으로 체크섬을 틀리게 설정
	_, err := ParseDeviceGid(pkt)
	if err == nil {
		t.Error("체크섬 불일치 시 오류가 발생해야 함 (ParseAuthResponse 오류 전파 검증)")
	}
}
