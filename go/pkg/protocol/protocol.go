// Package protocol은 KDO_HotWaterMat(온수매트) BLE 패킷 생성 및 파싱을 구현합니다.
//
// 온수매트와 앱 사이의 BLE 통신은 항상 20바이트 고정 길이 패킷으로 이루어집니다.
// 이 패키지는 BLE 라이브러리에 의존하지 않으며, 순수하게 패킷의 인코딩/디코딩만 담당합니다.
//
// 패킷 구조 (20바이트):
//
//	[0]     STX (0xB2) - 시작 바이트, 모든 패킷의 첫 바이트
//	[1]     방향/타입 - 0x80: 앱→매트, 0x00: 매트→앱, 0x01: 핸드셰이크, 0xF1: 인증 응답
//	[2]     모드 - 0x01: 난방, 0x02: 타이머꺼짐, 0x03: 취침, 0x06: 전원, 0x07: 급속난방, 0x08: 이온케어
//	[3]     좌/우 선택 - 0x02: 왼쪽, 0x04: 오른쪽, 0x06: 양쪽
//	[4]     볼륨(상위4비트) + 수위(하위4비트)
//	[5-6]   모드별 고정값
//	[7]     왼쪽 현재 온도 (인코딩된 바이트)
//	[8]     오른쪽 현재 온도 (인코딩된 바이트)
//	[9]     모드별 고정값
//	[10]    왼쪽 목표 온도 (인코딩된 바이트)
//	[11]    오른쪽 목표 온도 (인코딩된 바이트)
//	[12-18] 패딩 또는 추가 데이터
//	[19]    체크섬 - bytes[1:19]의 합을 0xFF로 AND 연산한 값
package protocol

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// PacketSize는 BLE 패킷의 고정 크기입니다. 모든 패킷은 반드시 20바이트여야 합니다.
	PacketSize = 20

	// STX는 모든 패킷의 시작 바이트입니다 (Start of TeXt).
	// 패킷 파싱 시 첫 바이트가 0xB2가 아니면 유효하지 않은 패킷으로 간주합니다.
	STX byte = 0xB2

	// DirAppToMat는 앱에서 매트로 보내는 명령 패킷의 방향 바이트입니다 (byte[1] = 0x80).
	// 온도 설정, 전원 ON/OFF 등의 명령을 보낼 때 사용합니다.
	DirAppToMat byte = 0x80

	// DirMatToApp는 매트에서 앱으로 보내는 상태 알림 패킷의 방향 바이트입니다 (byte[1] = 0x00).
	// CHAR1(0100dd) 특성을 통해 수신되는 STATUS 알림에 사용됩니다.
	DirMatToApp byte = 0x00

	// DirHandshake는 핸드셰이크(연결 초기화) 패킷의 방향 바이트입니다 (byte[1] = 0x01).
	// 최초 페어링 또는 DeviceGid를 사용한 인증 핸드셰이크에 사용됩니다.
	DirHandshake byte = 0x01

	// DirAuth는 매트에서 보내는 인증 응답 패킷의 방향 바이트입니다 (byte[1] = 0xF1).
	// CHAR2(0200dd) 특성을 통해 수신되며, B2F1 형식의 인증 응답을 나타냅니다.
	DirAuth byte = 0xF1

	// ServiceUUID는 온수매트 BLE 서비스의 UUID입니다.
	// 기기 검색 시 이 UUID를 가진 서비스를 찾습니다.
	ServiceUUID = "00001c0d-d102-11e1-9b23-2ce2a80000dd"

	// Char1UUID는 상태 알림 수신용 BLE 특성 UUID입니다 (0100dd).
	// 매트 → 앱 방향으로, 현재 온도/모드/수위 등의 STATUS 알림을 구독(subscribe)합니다.
	// 이 특성에는 쓰기(write)를 하면 안 됩니다.
	Char1UUID = "00001c0d-d102-11e1-9b23-2ce2a80100dd" // 상태 알림 수신

	// Char2UUID는 명령 쓰기 + 인증 응답용 BLE 특성 UUID입니다 (0200dd).
	// 앱 → 매트 방향으로 핸드셰이크, 온도 설정, 전원 제어 등 모든 명령을 씁니다.
	// 또한 매트에서 B2F1 인증 응답을 이 특성을 통해 알림으로 보냅니다.
	// 반드시 write-with-response 모드로 써야 합니다 (WriteWithoutResponse 사용 금지).
	Char2UUID = "00001c0d-d102-11e1-9b23-2ce2a80200dd" // 명령 쓰기 + 인증

	// Char3UUID는 미사용 BLE 특성 UUID입니다 (0400dd).
	// 현재 프로토콜에서 사용되지 않습니다.
	Char3UUID = "00001c0d-d102-11e1-9b23-2ce2a80400dd" // 미사용

	// BLEDeviceName은 온수매트가 BLE 광고(advertise)할 때 사용하는 기기 이름입니다.
	// 스캔 시 이 이름으로 기기를 필터링할 수 있습니다.
	BLEDeviceName = "KDO_HotWaterMat"

	// --- 모드 값 (byte[2]) ---
	// 패킷의 3번째 바이트(인덱스 2)에 들어가는 동작 모드 코드입니다.

	// ModeHeat는 난방 모드입니다 (0x01). 목표 온도를 설정하여 매트를 가열합니다.
	ModeHeat byte = 0x01

	// ModeTimerOff는 타이머 꺼짐 모드입니다 (0x02). 예약 시간 후 자동 꺼짐.
	ModeTimerOff byte = 0x02

	// ModeSleep은 취침 모드입니다 (0x03). 수면에 적합한 온도로 자동 조절됩니다.
	ModeSleep byte = 0x03

	// ModePower는 전원 제어 모드입니다 (0x06). 전원 ON/OFF 명령에 사용됩니다.
	// byte[6]이 0x2B이면 ON, 0xAB이면 OFF입니다.
	ModePower byte = 0x06

	// ModeFastheat는 급속 난방 모드입니다 (0x07). 빠르게 목표 온도까지 가열합니다.
	ModeFastheat byte = 0x07

	// ModeIoncare는 이온케어 모드입니다 (0x08). 살균/탈취 기능을 수행합니다.
	ModeIoncare byte = 0x08

	// --- 좌/우 선택 값 (byte[3]) ---
	// 패킷의 4번째 바이트(인덱스 3)에 들어가는 좌/우측 매트 선택 비트마스크입니다.
	// 듀얼 매트의 경우 왼쪽/오른쪽을 독립적으로 또는 동시에 제어할 수 있습니다.

	// SideLeft는 왼쪽 매트만 선택합니다 (0x02).
	SideLeft byte = 0x02

	// SideRight는 오른쪽 매트만 선택합니다 (0x04).
	SideRight byte = 0x04

	// SideBoth는 양쪽 매트 모두 선택합니다 (0x06 = 0x02 | 0x04).
	SideBoth byte = 0x06

	// --- 온도 범위 ---
	// 온수매트가 지원하는 설정 온도 범위입니다.

	// TempMin은 설정 가능한 최저 온도입니다 (28.0°C).
	TempMin float64 = 28.0

	// TempMax는 설정 가능한 최고 온도입니다 (48.0°C).
	TempMax float64 = 48.0

	// Pad는 패딩 바이트입니다 (0xFE).
	// 패킷에서 사용하지 않는 바이트를 채울 때 사용합니다.
	Pad byte = 0xFE
)


// ModeNames는 모드 바이트 값을 읽기 쉬운 이름으로 매핑합니다.
// 예: 0x01 → "HEAT", 0x06 → "POWER"
var ModeNames = map[byte]string{
	ModeHeat:     "HEAT",
	ModeTimerOff: "TIMER_OFF",
	ModeSleep:    "SLEEP",
	ModePower:    "POWER",
	ModeFastheat: "FASTHEAT",
	ModeIoncare:  "IONCARE",
}

// EncodeTemp는 실제 온도값(float64)을 BLE 패킷에 넣을 바이트로 인코딩합니다.
//
// 온도 인코딩 규칙:
//   - 정수 온도 (예: 33.0°C): 그대로 바이트로 변환 → 33 (0x21)
//   - 소수점 온도 (예: 33.5°C): temp + 127.5 = 161 (0xA1)
//   - 0.5°C 단위만 허용 (33.3°C 같은 값은 오류)
//   - 범위: 28.0°C ~ 48.0°C
//
// 주의: bit7(최상위 비트)은 난방 플래그가 아닙니다.
// 단순히 0.5도 소수점 온도를 구분하기 위한 인코딩입니다.
//
// 예시:
//
//	28.0°C → 28 (0x1C)
//	28.5°C → 156 (0x9C) = 28.5 + 127.5
//	33.0°C → 33 (0x21)
//	33.5°C → 161 (0xA1) = 33.5 + 127.5
//	48.0°C → 48 (0x30)
func EncodeTemp(temp float64) (byte, error) {
	if temp < TempMin || temp > TempMax {
		return 0, fmt.Errorf("temperature %.1f out of range [%.1f, %.1f]", temp, TempMin, TempMax)
	}
	// 0.5도 단위 검증: 온도를 2배 했을 때 정수가 되어야 함
	doubled := temp * 2
	if doubled != float64(int(doubled)) {
		return 0, fmt.Errorf("temperature %.1f must be in 0.5°C increments", temp)
	}

	intPart := int(temp)
	if temp != float64(intPart) {
		// 소수점이 있는 경우 (.5°C) — int(temp + 127.5)로 인코딩
		// 예: 33.5 + 127.5 = 161 = 0xA1
		return byte(int(temp + 127.5)), nil
	}
	// 정수 온도는 그대로 바이트로 변환
	return byte(intPart), nil
}

// DecodeTemp는 BLE 패킷의 온도 바이트를 실제 온도값(float64)으로 디코딩합니다.
//
// 디코딩 규칙:
//   - 바이트 값 > 127: 소수점 온도 → byte - 127.5 (예: 161 → 33.5°C)
//   - 바이트 값 ≤ 127: 정수 온도 → 그대로 (예: 33 → 33.0°C)
//
// EncodeTemp의 역함수입니다. EncodeTemp(DecodeTemp(b)) == b가 항상 성립합니다.
func DecodeTemp(b byte) float64 {
	if b > 127 {
		// 상위 비트가 설정된 경우: 0.5도 소수점 온도
		return float64(b) - 127.5
	}
	// 그 외: 정수 온도
	return float64(b)
}

// CalcChecksum은 20바이트 패킷의 체크섬을 계산합니다.
//
// 계산 방법: bytes[1]부터 bytes[18]까지 (총 18바이트)를 모두 더한 후,
// 하위 8비트만 취합니다 (& 0xFF). 결과는 bytes[19]에 저장됩니다.
//
// 예시: 핸드셰이크 패킷
//
//	B2 01 FE FE FE FE FE FE FE FE FE FE FE FE FE FE FE FE FE [DF]
//	sum(01 + FE*17) = 0x01 + 0x10E2 = 0x10E3 → 0xE3... 아니, 실제로는
//	sum = 0x01 + 17*0xFE = 1 + 4318 = 4319 → 4319 & 0xFF = 0xDF ✓
func CalcChecksum(pkt []byte) byte {
	if len(pkt) < PacketSize {
		return 0
	}
	var total uint16
	for i := 1; i < 19; i++ {
		total += uint16(pkt[i])
	}
	// 16비트 합의 하위 바이트만 반환 (마지막 2자리 16진수)
	return byte(total & 0xFF)
}

// BuildHandshake는 최초 페어링용 핸드셰이크 패킷을 생성합니다.
//
// DeviceGid가 없는 상태에서 처음 기기와 연결할 때 사용합니다.
// 매트는 이 패킷을 받으면 B2F1 페어링 응답으로 DeviceGid를 돌려줍니다.
//
// 패킷 구조:
//
//	[0]=0xB2 [1]=0x01 [2..18]=0xFE(패딩) [19]=체크섬
func BuildHandshake() [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirHandshake
	// byte[2]부터 byte[18]까지 패딩(0xFE)으로 채움
	for i := 2; i < 19; i++ {
		pkt[i] = Pad
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// BuildHandshakeWithKey는 DeviceGid를 사용한 인증 핸드셰이크 패킷을 생성합니다.
//
// 이미 페어링된 기기에 다시 연결할 때 사용합니다.
// DeviceGid는 최초 페어링 시 매트에서 받은 6바이트 인증 키입니다.
//
// 패킷 구조:
//
//	[0]=0xB2 [1]=0x01 [2..7]=DeviceGid(6바이트) [8..18]=0xFE(패딩) [19]=체크섬
//
// 인증 흐름:
//  1. 이 핸드셰이크를 CHAR2로 전송
//  2. 매트가 CHAR2를 통해 B2F1 인증 응답을 알림으로 보냄
//  3. authType이 0x02이면 인증 완료 → 이후 명령 전송 가능
func BuildHandshakeWithKey(gid [6]byte) [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirHandshake
	// DeviceGid를 byte[2]~byte[7] 위치에 복사 (6바이트)
	copy(pkt[2:8], gid[:])
	// byte[8]~byte[9]에 패딩
	pkt[8] = Pad
	pkt[9] = Pad
	// byte[10]~byte[18]에 패딩
	for i := 10; i < 19; i++ {
		pkt[i] = Pad
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// BuildHeat는 난방(HEAT) 명령 패킷을 생성합니다.
//
// 매트의 목표 온도를 설정하는 핵심 명령입니다.
// 반드시 현재 STATUS에서 받은 현재 온도 바이트(raw)를 함께 전달해야 합니다.
//
// 매개변수:
//   - side: 좌/우 선택 (SideLeft=0x02, SideRight=0x04, SideBoth=0x06)
//   - leftCur: 왼쪽 현재 온도 (STATUS 패킷의 byte[7] 원시값)
//   - rightCur: 오른쪽 현재 온도 (STATUS 패킷의 byte[8] 원시값)
//   - leftTarget: 왼쪽 목표 온도 (EncodeTemp로 인코딩한 값)
//   - rightTarget: 오른쪽 목표 온도 (EncodeTemp로 인코딩한 값)
//
// 패킷 구조:
//
//	[0]=0xB2(STX) [1]=0x80(앱→매트) [2]=0x01(HEAT) [3]=side
//	[4]=0x00(볼륨/수위) [5]=0x02(HEAT고정) [6]=0x25(HEAT고정)
//	[7]=leftCur [8]=rightCur [9]=0x23(HEAT고정)
//	[10]=leftTarget [11]=rightTarget [12..18]=0xFE(패딩)
//	[19]=체크섬
//
// 사용 예시:
//
//	// 왼쪽 매트를 33°C에서 34°C로 변경
//	leftTgt, _ := protocol.EncodeTemp(34.0)  // → 34 (0x22)
//	rightTgt, _ := protocol.EncodeTemp(28.0) // → 28 (0x1C)
//	pkt := protocol.BuildHeat(SideLeft, status.LeftCurrentRaw, status.RightCurrentRaw, leftTgt, rightTgt)
func BuildHeat(side byte, leftCur, rightCur, leftTarget, rightTarget byte) [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirAppToMat        // 앱 → 매트 방향
	pkt[2] = ModeHeat            // 난방 모드 (0x01)
	pkt[3] = side                // 좌/우 선택
	pkt[4] = 0x00                // 볼륨/수위 (난방 명령에서는 0)
	pkt[5] = 0x02                // HEAT 모드 고정값
	pkt[6] = 0x25                // HEAT 모드 고정값
	pkt[7] = leftCur             // 왼쪽 현재 온도 (STATUS에서 받은 원시 바이트)
	pkt[8] = rightCur            // 오른쪽 현재 온도 (STATUS에서 받은 원시 바이트)
	pkt[9] = 0x23                // HEAT 모드 고정값
	pkt[10] = leftTarget         // 왼쪽 목표 온도 (EncodeTemp로 인코딩된 값)
	pkt[11] = rightTarget        // 오른쪽 목표 온도 (EncodeTemp로 인코딩된 값)
	pkt[12] = Pad                // 패딩
	for i := 13; i < 19; i++ {
		pkt[i] = Pad             // 나머지 패딩
	}
	pkt[19] = CalcChecksum(pkt[:]) // 체크섬 계산 및 설정
	return pkt
}

// BuildPowerOn은 전원 켜기(Power ON) 명령 패킷을 생성합니다.
//
// 매트의 전원을 켤 때 사용합니다.
// 가능하면 최신 STATUS에서 받은 현재 온도를 전달해야 하지만,
// STATUS를 아직 받지 못한 경우 0을 넣어도 동작합니다.
//
// 매개변수:
//   - leftCur: 왼쪽 현재 온도 (STATUS 패킷의 byte[7] 원시값, 없으면 0)
//   - rightCur: 오른쪽 현재 온도 (STATUS 패킷의 byte[8] 원시값, 없으면 0)
//
// 패킷 구조:
//
//	[0]=0xB2 [1]=0x80 [2]=0x06(POWER) [3]=0x06(양쪽)
//	[4]=0x00 [5]=0xFE [6]=0x2B(ON 마커) [7]=leftCur [8]=rightCur
//	[9]=0xFE [10..15]=0x00 [16..18]=0xFE [19]=체크섬
func BuildPowerOn(leftCur, rightCur byte) [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirAppToMat     // 앱 → 매트 방향
	pkt[2] = ModePower        // 전원 제어 모드 (0x06)
	pkt[3] = SideBoth         // 양쪽 모두 (전원은 항상 양쪽)
	pkt[4] = 0x00
	pkt[5] = Pad
	pkt[6] = 0x2B             // ON 마커 — 이 값이 전원 켜기를 의미함
	pkt[7] = leftCur          // 왼쪽 현재 온도
	pkt[8] = rightCur         // 오른쪽 현재 온도
	pkt[9] = Pad
	for i := 10; i < 16; i++ {
		pkt[i] = 0x00         // 0으로 채움
	}
	for i := 16; i < 19; i++ {
		pkt[i] = Pad          // 패딩
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// BuildPowerOff는 전원 끄기(Power OFF) 명령 패킷을 생성합니다.
//
// 전원을 끈 후에도 매트는 BLE 광고를 계속하므로,
// BLE를 통해 다시 전원을 켤 수 있습니다 (물리 버튼 불필요).
//
// 매개변수:
//   - leftCur: 왼쪽 현재 온도 (STATUS 패킷의 byte[7] 원시값, 없으면 0)
//   - rightCur: 오른쪽 현재 온도 (STATUS 패킷의 byte[8] 원시값, 없으면 0)
//
// 패킷 구조:
//
//	[0]=0xB2 [1]=0x80 [2]=0x06(POWER) [3]=0x04(오른쪽, PROTOCOL.md 기준)
//	[4]=0x00 [5]=0xFE [6]=0xAB(OFF 마커) [7]=leftCur [8]=rightCur
//	[9]=0xFE [10]=0x00 [11]=rightCur [12]=0xFE [13]=rightCur
//	[14..18]=0xFE [19]=체크섬
func BuildPowerOff(leftCur, rightCur byte) [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirAppToMat      // 앱 → 매트 방향
	pkt[2] = ModePower         // 전원 제어 모드 (0x06)
	pkt[3] = SideRight         // 0x04 — PROTOCOL.md 사양에 따른 값
	pkt[4] = 0x00
	pkt[5] = Pad
	pkt[6] = 0xAB              // OFF 마커 — 이 값이 전원 끄기를 의미함
	pkt[7] = leftCur           // 왼쪽 현재 온도
	pkt[8] = rightCur          // 오른쪽 현재 온도
	pkt[9] = Pad
	pkt[10] = 0x00
	pkt[11] = rightCur         // 오른쪽 현재 온도 (반복)
	pkt[12] = Pad
	pkt[13] = rightCur         // 오른쪽 현재 온도 (반복)
	for i := 14; i < 19; i++ {
		pkt[i] = Pad           // 패딩
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// Status는 매트에서 수신한 STATUS 알림 패킷을 파싱한 결과를 담는 구조체입니다.
//
// CHAR1(0100dd) 특성의 알림(notification)을 통해 주기적으로 수신됩니다.
// 매트의 현재 상태(모드, 온도, 수위 등)를 나타냅니다.
type Status struct {
	// Mode는 현재 동작 모드의 바이트 값입니다 (예: 0x01=HEAT, 0x06=POWER).
	Mode byte

	// ModeName은 모드의 사람이 읽을 수 있는 이름입니다 (예: "HEAT", "POWER").
	ModeName string

	// Side는 현재 활성화된 좌/우 선택값입니다 (0x02=왼쪽, 0x04=오른쪽, 0x06=양쪽).
	Side byte

	// Volume은 볼륨 값입니다 (byte[4]의 상위 4비트).
	Volume int

	// WaterLevel은 수위 값입니다 (byte[4]의 하위 4비트).
	// 1=LOW(부족), 2=OK(정상), 3=FULL(가득참).
	WaterLevel int

	// Field5는 byte[5]의 원시값입니다 (모드별로 의미가 다름).
	Field5 byte

	// Field6은 byte[6]의 원시값입니다 (모드별로 의미가 다름).
	Field6 byte

	// LeftCurrentRaw는 왼쪽 현재 온도의 인코딩된 원시 바이트입니다.
	// 명령 패킷 생성 시 이 값을 그대로 전달해야 합니다.
	LeftCurrentRaw byte

	// RightCurrentRaw는 오른쪽 현재 온도의 인코딩된 원시 바이트입니다.
	// 명령 패킷 생성 시 이 값을 그대로 전달해야 합니다.
	RightCurrentRaw byte

	// LeftCurrent는 왼쪽 현재 온도입니다 (디코딩된 실제 °C 값).
	LeftCurrent float64

	// RightCurrent는 오른쪽 현재 온도입니다 (디코딩된 실제 °C 값).
	RightCurrent float64

	// LeftTarget은 왼쪽 목표 온도입니다 (디코딩된 실제 °C 값).
	LeftTarget float64

	// RightTarget은 오른쪽 목표 온도입니다 (디코딩된 실제 °C 값).
	RightTarget float64

	// SubCmd는 byte[12]의 값으로, 하위 명령 코드입니다.
	SubCmd byte

	// Raw는 원본 20바이트 패킷 데이터입니다 (디버깅용).
	Raw [PacketSize]byte

	// PoweredOff는 매트의 전원이 꺼져 있는지 여부입니다.
	// Mode가 POWER이고 목표 온도가 양쪽 모두 0이면 전원이 꺼진 것으로 판단합니다.
	PoweredOff bool
}

// ParseStatus는 매트에서 수신한 20바이트 상태 알림 패킷을 파싱합니다.
//
// CHAR1(0100dd) 특성의 알림을 통해 받은 데이터를 Status 구조체로 변환합니다.
//
// 검증 사항:
//   - 패킷 길이가 20바이트 이상인지 확인
//   - byte[0]이 STX(0xB2)인지 확인
//   - byte[1]이 DirMatToApp(0x00)인지 확인 (매트→앱 방향)
//
// 전원 꺼짐 판단: Mode가 POWER(0x06)이고 byte[10], byte[11]이 모두 0이면 전원 OFF로 판단합니다.
func ParseStatus(data []byte) (*Status, error) {
	if len(data) < PacketSize {
		return nil, errors.New("packet too short")
	}
	if data[0] != STX {
		return nil, fmt.Errorf("invalid STX: 0x%02X", data[0])
	}
	if data[1] != DirMatToApp {
		return nil, fmt.Errorf("not a STATUS packet (direction=0x%02X)", data[1])
	}

	// 모드 이름 조회 (알 수 없는 모드는 "UNKNOWN(0xXX)" 형식)
	mode := data[2]
	modeName := ModeNames[mode]
	if modeName == "" {
		modeName = fmt.Sprintf("UNKNOWN(0x%02X)", mode)
	}

	// byte[4]에서 볼륨(상위 4비트)과 수위(하위 4비트) 추출
	vol := int(data[4]>>4) & 0x0F
	wlv := int(data[4]) & 0x0F

	// 현재 온도 원시 바이트 (명령 전송 시 그대로 사용)
	leftCurRaw := data[7]
	rightCurRaw := data[8]

	// 목표 온도 디코딩 (byte[10], byte[11])
	leftTarget := DecodeTemp(data[10])
	rightTarget := DecodeTemp(data[11])

	// 현재 온도 디코딩 (0.5°C 인코딩 적용)
	leftCur := DecodeTemp(leftCurRaw)
	rightCur := DecodeTemp(rightCurRaw)

	// 전원 꺼짐 판단: POWER 모드이고 목표 온도가 양쪽 모두 0이면 OFF
	poweredOff := mode == ModePower && data[10] == 0 && data[11] == 0

	var raw [PacketSize]byte
	copy(raw[:], data[:PacketSize])

	return &Status{
		Mode:            mode,
		ModeName:        modeName,
		Side:            data[3],
		Volume:          vol,
		WaterLevel:      wlv,
		Field5:          data[5],
		Field6:          data[6],
		LeftCurrentRaw:  leftCurRaw,
		RightCurrentRaw: rightCurRaw,
		LeftCurrent:     leftCur,
		RightCurrent:    rightCur,
		LeftTarget:      leftTarget,
		RightTarget:     rightTarget,
		SubCmd:          data[12],
		Raw:             raw,
		PoweredOff:      poweredOff,
	}, nil
}

// ParseAuthResponse는 수신된 패킷이 B2F1 인증 응답인지 확인합니다.
//
// CHAR2(0200dd) 특성의 알림을 통해 받은 데이터를 검사합니다.
// B2F1로 시작하는 패킷은 인증 관련 응답입니다.
//
// 반환값 (인증 타입 바이트):
//   - 0x01: 페어링 응답 — DeviceGid가 포함되어 있음 (ParseDeviceGid로 추출)
//   - 0x02: 인증 완료 — 핸드셰이크 성공, 이후 명령 전송 가능
func ParseAuthResponse(data []byte) (byte, error) {
	if len(data) < PacketSize {
		return 0, errors.New("packet too short")
	}
	if data[0] != STX {
		return 0, fmt.Errorf("invalid STX: 0x%02X", data[0])
	}
	if data[1] != DirAuth {
		return 0, errors.New("not an auth response")
	}
	// byte[2]가 인증 타입을 나타냄
	return data[2], nil
}

// ParseDeviceGid는 페어링 응답 패킷에서 6바이트 DeviceGid를 추출합니다.
//
// 최초 페어링(BuildHandshake) 후 매트가 보내는 B2F1 응답(authType=0x01)에서
// DeviceGid를 추출합니다. 이 값은 이후 재연결 시 BuildHandshakeWithKey에 사용됩니다.
//
// DeviceGid 위치: byte[3]~byte[8] (6바이트)
//
// 사용 예시:
//
//	// 페어링 응답 수신 후
//	gid, err := protocol.ParseDeviceGid(responseData)
//	// gid를 저장해 두고, 이후 연결 시:
//	handshake := protocol.BuildHandshakeWithKey(gid)
func ParseDeviceGid(data []byte) ([6]byte, error) {
	var gid [6]byte
	authType, err := ParseAuthResponse(data)
	if err != nil {
		return gid, err
	}
	if authType != 0x01 {
		return gid, fmt.Errorf("not a pairing response (type=0x%02X)", authType)
	}
	// byte[3]~byte[8]에서 DeviceGid 6바이트 추출
	copy(gid[:], data[3:9])
	return gid, nil
}

// FormatPacket은 패킷을 16진수 문자열로 포맷합니다 (디버깅용).
//
// 각 바이트를 2자리 대문자 16진수로 변환하고 공백으로 구분합니다.
// 예: []byte{0xB2, 0x01, 0xFE} → "B2 01 FE"
func FormatPacket(pkt []byte) string {
	parts := make([]string, len(pkt))
	for i, b := range pkt {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, " ")
}

// FormatDeviceGid는 6바이트 DeviceGid를 대문자 hex 문자열로 포맷합니다.
func FormatDeviceGid(gid [6]byte) string {
	return fmt.Sprintf("%02X%02X%02X%02X%02X%02X", gid[0], gid[1], gid[2], gid[3], gid[4], gid[5])
}

// ParseDeviceGidHex는 16진수 문자열(예: "13CE3CC53E5A")을 6바이트 DeviceGid로 파싱합니다.
//
// CLI나 환경변수에서 DeviceGid를 문자열로 입력받을 때 사용합니다.
// 정확히 12자리 16진수 문자열(= 6바이트)이어야 합니다.
//
// 예시: "13CE3CC53E5A" → [6]byte{0x13, 0xCE, 0x3C, 0xC5, 0x3E, 0x5A}
func ParseDeviceGidHex(s string) ([6]byte, error) {
	var gid [6]byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return gid, fmt.Errorf("invalid hex: %w", err)
	}
	if len(b) != 6 {
		return gid, fmt.Errorf("DeviceGid must be 6 bytes, got %d", len(b))
	}
	copy(gid[:], b)
	return gid, nil
}

// SideName은 좌/우 선택 바이트를 사람이 읽을 수 있는 이름으로 변환합니다.
//
// 예: 0x02 → "left", 0x04 → "right", 0x06 → "both"
func SideName(side byte) string {
	switch side {
	case SideLeft:
		return "left"
	case SideRight:
		return "right"
	case SideBoth:
		return "both"
	default:
		return fmt.Sprintf("unknown(0x%02X)", side)
	}
}

// WaterLevelName은 수위 값을 사람이 읽을 수 있는 이름으로 변환합니다.
//
// 수위 값은 STATUS 패킷의 byte[4] 하위 4비트에서 추출됩니다.
// 1=LOW(부족/보충필요), 2=OK(정상), 3=FULL(가득참)
func WaterLevelName(wlv int) string {
	switch wlv {
	case 1:
		return "LOW"
	case 2:
		return "OK"
	case 3:
		return "FULL"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", wlv)
	}
}
