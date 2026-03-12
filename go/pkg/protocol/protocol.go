// Package protocol implements BLE packet building and parsing
// for the KDO_HotWaterMat (온수매트) device.
package protocol

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	PacketSize = 20

	STX          byte = 0xB2
	DirAppToMat  byte = 0x80
	DirMatToApp  byte = 0x00
	DirHandshake byte = 0x01
	DirAuth      byte = 0xF1

	ServiceUUID = "00001c0d-d102-11e1-9b23-2ce2a80000dd"
	Char1UUID   = "00001c0d-d102-11e1-9b23-2ce2a80100dd" // STATUS notifications
	Char2UUID   = "00001c0d-d102-11e1-9b23-2ce2a80200dd" // COMMAND writes + auth
	Char3UUID   = "00001c0d-d102-11e1-9b23-2ce2a80400dd" // TBD

	DefaultBLEAddress = "FD319CFA-2E62-116D-D348-5B9FEEE95D2F"

	// Mode values (byte[2])
	ModeHeat     byte = 0x01
	ModeTimerOff byte = 0x02
	ModeSleep    byte = 0x03
	ModePower    byte = 0x06
	ModeFastheat byte = 0x07
	ModeIoncare  byte = 0x08

	// Side values (byte[3])
	SideLeft  byte = 0x02
	SideRight byte = 0x04
	SideBoth  byte = 0x06

	// Temperature range
	TempMin float64 = 28.0
	TempMax float64 = 48.0

	Pad byte = 0xFE
)

// DefaultDeviceGid is the default 6-byte authentication key.
var DefaultDeviceGid = [6]byte{0x13, 0xCE, 0x3C, 0xC5, 0x3E, 0x5A}

// ModeNames maps mode byte values to human-readable names.
var ModeNames = map[byte]string{
	ModeHeat:     "HEAT",
	ModeTimerOff: "TIMER_OFF",
	ModeSleep:    "SLEEP",
	ModePower:    "POWER",
	ModeFastheat: "FASTHEAT",
	ModeIoncare:  "IONCARE",
}

// EncodeTemp converts a temperature (0.5°C precision) to its BLE byte value.
func EncodeTemp(temp float64) (byte, error) {
	if temp < TempMin || temp > TempMax {
		return 0, fmt.Errorf("temperature %.1f out of range [%.1f, %.1f]", temp, TempMin, TempMax)
	}
	// Validate 0.5 step
	doubled := temp * 2
	if doubled != float64(int(doubled)) {
		return 0, fmt.Errorf("temperature %.1f must be in 0.5°C increments", temp)
	}

	intPart := int(temp)
	if temp != float64(intPart) {
		// Has decimal (.5) — encode as int(temp + 127.5)
		return byte(int(temp + 127.5)), nil
	}
	return byte(intPart), nil
}

// DecodeTemp converts a BLE byte value back to temperature.
func DecodeTemp(b byte) float64 {
	if b > 127 {
		return float64(b) - 127.5
	}
	return float64(b)
}

// CalcChecksum computes the checksum for a 20-byte packet.
// It sums bytes[1:19] and returns the low byte.
func CalcChecksum(pkt []byte) byte {
	if len(pkt) < PacketSize {
		return 0
	}
	var total uint16
	for i := 1; i < 19; i++ {
		total += uint16(pkt[i])
	}
	// "last 2 hex digits" of the 16-bit sum = low byte
	return byte(total & 0xFF)
}

// BuildHandshake creates the initial pairing handshake (no DeviceGid).
func BuildHandshake() [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirHandshake
	for i := 2; i < 19; i++ {
		pkt[i] = Pad
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// BuildHandshakeWithKey creates an authentication handshake using the given DeviceGid.
func BuildHandshakeWithKey(gid [6]byte) [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirHandshake
	// DeviceGid at positions [2..7]
	copy(pkt[2:8], gid[:])
	// Padding at [8..9]
	pkt[8] = Pad
	pkt[9] = Pad
	// Fill rest with padding
	for i := 10; i < 19; i++ {
		pkt[i] = Pad
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// BuildHeat creates a HEAT command packet to set temperature.
// leftCur/rightCur: current temps from latest STATUS (raw bytes).
// leftTarget/rightTarget: new target temps (encoded via EncodeTemp).
func BuildHeat(side byte, leftCur, rightCur, leftTarget, rightTarget byte) [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirAppToMat
	pkt[2] = ModeHeat
	pkt[3] = side
	pkt[4] = 0x00 // vol/water
	pkt[5] = 0x02 // HEAT field5
	pkt[6] = 0x25 // HEAT field6
	pkt[7] = leftCur
	pkt[8] = rightCur
	pkt[9] = 0x23 // HEAT field9
	pkt[10] = leftTarget
	pkt[11] = rightTarget
	pkt[12] = Pad
	for i := 13; i < 19; i++ {
		pkt[i] = Pad
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// BuildPowerOn creates a power-on command packet.
// leftCur/rightCur: current temps from latest STATUS.
func BuildPowerOn(leftCur, rightCur byte) [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirAppToMat
	pkt[2] = ModePower
	pkt[3] = SideBoth
	pkt[4] = 0x00
	pkt[5] = Pad
	pkt[6] = 0x2B // ON
	pkt[7] = leftCur
	pkt[8] = rightCur
	pkt[9] = Pad
	for i := 10; i < 16; i++ {
		pkt[i] = 0x00
	}
	for i := 16; i < 19; i++ {
		pkt[i] = Pad
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// BuildPowerOff creates a power-off command packet.
// leftCur/rightCur: current temps from latest STATUS.
func BuildPowerOff(leftCur, rightCur byte) [PacketSize]byte {
	var pkt [PacketSize]byte
	pkt[0] = STX
	pkt[1] = DirAppToMat
	pkt[2] = ModePower
	pkt[3] = SideRight // 0x04 per PROTOCOL.md
	pkt[4] = 0x00
	pkt[5] = Pad
	pkt[6] = 0xAB // OFF
	pkt[7] = leftCur
	pkt[8] = rightCur
	pkt[9] = Pad
	pkt[10] = 0x00
	pkt[11] = rightCur
	pkt[12] = Pad
	pkt[13] = rightCur
	for i := 14; i < 19; i++ {
		pkt[i] = Pad
	}
	pkt[19] = CalcChecksum(pkt[:])
	return pkt
}

// Status represents parsed data from a STATUS notification packet.
type Status struct {
	Mode           byte
	ModeName       string
	Side           byte
	Volume         int
	WaterLevel     int
	Field5         byte
	Field6         byte
	LeftCurrentRaw byte
	RightCurrentRaw byte
	LeftCurrent    float64
	RightCurrent   float64
	LeftTarget     float64
	RightTarget    float64
	LeftHeating    bool
	RightHeating   bool
	SubCmd         byte
	Raw            [PacketSize]byte
	PoweredOff     bool
}

// ParseStatus parses a 20-byte STATUS notification from the mat.
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

	mode := data[2]
	modeName := ModeNames[mode]
	if modeName == "" {
		modeName = fmt.Sprintf("UNKNOWN(0x%02X)", mode)
	}

	vol := int(data[4]>>4) & 0x0F
	wlv := int(data[4]) & 0x0F

	leftCurRaw := data[7]
	rightCurRaw := data[8]

	leftTarget := DecodeTemp(data[10])
	rightTarget := DecodeTemp(data[11])

	// Temperature bytes use 0.5°C encoding
	leftCur := DecodeTemp(leftCurRaw)
	rightCur := DecodeTemp(rightCurRaw)

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
		LeftHeating:     leftCurRaw&0x80 != 0,
		RightHeating:    rightCurRaw&0x80 != 0,
		SubCmd:          data[12],
		Raw:             raw,
		PoweredOff:      poweredOff,
	}, nil
}

// ParseAuthResponse checks if a received packet is a B2F1 authentication response.
// Returns the auth type byte: 0x01 = pairing response, 0x02 = authenticated.
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
	return data[2], nil
}

// ParseDeviceGid extracts the 6-byte DeviceGid from a pairing response.
func ParseDeviceGid(data []byte) ([6]byte, error) {
	var gid [6]byte
	authType, err := ParseAuthResponse(data)
	if err != nil {
		return gid, err
	}
	if authType != 0x01 {
		return gid, fmt.Errorf("not a pairing response (type=0x%02X)", authType)
	}
	copy(gid[:], data[3:9])
	return gid, nil
}

// FormatPacket returns a hex dump of a packet for debugging.
func FormatPacket(pkt []byte) string {
	parts := make([]string, len(pkt))
	for i, b := range pkt {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, " ")
}

// ParseDeviceGidHex parses a hex string (e.g. "13CE3CC53E5A") into a DeviceGid.
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

// SideName returns a human-readable name for a side byte value.
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

// WaterLevelName returns a human-readable name for a water level value.
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
