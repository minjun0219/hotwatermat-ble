// Package ble는 KDO_HotWaterMat(온수매트) 기기에 BLE(Bluetooth Low Energy)로 연결하는 클라이언트를 제공합니다.
//
// 이 패키지는 tinygo.org/x/bluetooth 라이브러리를 사용하여 실제 BLE 하드웨어와 통신합니다.
// macOS(CoreBluetooth)와 Linux(BlueZ/D-Bus)를 지원하며, 플랫폼별 차이는
// write_darwin.go, write_linux.go, write_unsupported.go 파일에서 처리합니다.
//
// 주요 기능:
//   - BLE 기기 스캔 (Scan)
//   - 연결 및 인증 (Connect)
//   - 상태 조회 (GetStatus)
//   - 온도 설정 (SetTemp)
//   - 전원 제어 (PowerOn, PowerOff)
//
// 연결 및 인증 흐름:
//  1. BLE 어댑터 활성화
//  2. 기기 스캔 및 주소 매칭
//  3. GATT 서비스/특성 탐색
//  4. CHAR1(상태 알림) 구독
//  5. CHAR2(인증 알림) 구독
//  6. CHAR2로 핸드셰이크 패킷 전송
//  7. CHAR2에서 B2F1 인증 완료(authType=0x02) 알림 수신
//  8. 인증 완료 → 명령 전송 가능
package ble

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
	"tinygo.org/x/bluetooth"
)

// adapter는 시스템의 기본 BLE 어댑터입니다.
// tinygo bluetooth 라이브러리가 제공하는 싱글턴 어댑터를 사용합니다.
var adapter = bluetooth.DefaultAdapter

// ScanResult는 BLE 스캔으로 발견된 기기 정보를 담는 구조체입니다.
type ScanResult struct {
	// Address는 BLE 기기의 MAC 주소입니다 (예: "AA:BB:CC:DD:EE:FF").
	Address string

	// Name은 기기가 광고(advertise)하는 이름입니다 (예: "KDO_HotWaterMat").
	Name string

	// RSSI는 수신 신호 강도 지표입니다.
	// 값이 클수록(0에 가까울수록) 신호가 강합니다. 일반적으로 -30(매우 강함) ~ -100(매우 약함).
	RSSI int16
}

// Scan은 주어진 시간 동안 BLE 기기를 검색합니다.
//
// 매개변수:
//   - timeout: 스캔 지속 시간 (예: 5 * time.Second)
//   - nameFilter: 이 이름과 일치하는 기기만 반환. 빈 문자열이면 모든 기기 반환.
//
// 반환값:
//   - 발견된 기기 목록 (중복 포함 가능)
//   - BLE 어댑터 활성화 실패 시 오류
//
// 주의: 같은 기기가 여러 번 광고하면 중복 결과가 포함될 수 있습니다.
// 중복 제거는 호출자 측에서 처리해야 합니다.
func Scan(timeout time.Duration, nameFilter string) ([]ScanResult, error) {
	if err := adapter.Enable(); err != nil {
		return nil, wrapBLEEnableError(err)
	}

	var (
		mu      sync.Mutex   // results 슬라이스 동시 접근 보호용 뮤텍스
		results []ScanResult
	)

	// done 채널로 타임아웃 고루틴 취소를 지원합니다.
	// adapter.Scan이 오류로 일찍 반환되더라도(권한/어댑터 문제 등) 고루틴이
	// 이후 adapter.StopScan()을 호출하지 않도록 합니다.
	// DefaultAdapter는 전역 싱글턴이므로, 누출된 StopScan()이 다음 스캔/연결을
	// 예기치 않게 중단시키는 회귀를 방지합니다.
	done := make(chan struct{})
	timer := time.NewTimer(timeout)
	go func() {
		// 타임아웃 고루틴 종료 시 타이머를 정리합니다.
		defer timer.Stop()

		select {
		case <-timer.C:
			// 타이머 만료 시점에 Scan()이 이미 반환했는지(done closed) 다시 확인합니다.
			// timer.C와 done이 거의 동시에 준비된 경우에도, Scan() 종료 이후에는
			// StopScan()을 호출하지 않도록 경합을 제거합니다.
			select {
			case <-done:
				// Scan()이 이미 종료된 상태 — StopScan()을 호출하지 않고 종료
				return
			default:
				// 여전히 스캔이 진행 중인 경우에만 StopScan()을 호출
				adapter.StopScan()
			}
		case <-done:
			// Scan()이 먼저 반환됨 — 타임아웃 고루틴만 종료
			return
		}
	}()

	// BLE 스캔 시작 — 콜백은 기기가 발견될 때마다 호출됨
	// StopScan() 호출 시 Scan()이 반환됨
	err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		name := result.LocalName()
		// 이름 필터가 설정된 경우, 일치하지 않는 기기는 무시
		if nameFilter != "" && name != nameFilter {
			return
		}
		mu.Lock()
		results = append(results, ScanResult{
			Address: result.Address.String(),
			Name:    name,
			RSSI:    result.RSSI,
		})
		mu.Unlock()
	})

	// Scan() 반환 후 타임아웃 고루틴을 취소합니다.
	// 이 시점부터 results에 콜백이 추가되지 않으므로 안전하게 읽을 수 있습니다.
	close(done)

	// results 슬라이스를 mutex로 보호하여 읽습니다.
	// Scan() 반환 직후에도 콜백이 동시에 실행될 가능성이 있으므로(데이터 레이스 방지)
	// len/복사본 생성까지 임계구역으로 감쌉니다.
	mu.Lock()
	n := len(results)
	out := make([]ScanResult, n)
	copy(out, results)
	mu.Unlock()

	// StopScan()에 의한 중단 시 에러가 반환될 수 있으므로, 결과가 있으면 무시
	if err != nil && n == 0 {
		return nil, fmt.Errorf("scan: %w", err)
	}

	return out, nil
}

// Client는 온수매트 BLE 기기와의 연결을 관리하는 클라이언트입니다.
//
// 사용 순서:
//  1. NewClient로 클라이언트 생성
//  2. Connect로 연결 및 인증
//  3. GetStatus, SetTemp, PowerOn, PowerOff 등으로 기기 제어
//  4. Disconnect로 연결 해제
type Client struct {
	// address는 연결할 BLE 기기의 MAC 주소입니다.
	address string

	// deviceGid는 인증에 사용되는 6바이트 기기 고유 키입니다.
	// 최초 페어링 시 매트로부터 수신하며, 이후 재연결 시 사용합니다.
	deviceGid [6]byte

	// device는 연결된 BLE 기기 객체입니다.
	device bluetooth.Device

	// statusChar는 CHAR1(0100dd) — STATUS 알림 수신용 BLE 특성입니다.
	// 매트의 현재 상태(온도, 모드, 수위 등)를 주기적으로 알려줍니다.
	statusChar bluetooth.DeviceCharacteristic

	// cmdChar는 CHAR2(0200dd) — 명령 쓰기 + 인증 응답용 BLE 특성입니다.
	// 핸드셰이크, 온도 설정, 전원 명령 등 모든 쓰기가 이 특성을 통해 이루어집니다.
	cmdChar bluetooth.DeviceCharacteristic

	// lastStatus는 가장 최근에 수신한 STATUS 데이터입니다.
	// CHAR1 알림을 통해 자동으로 업데이트됩니다.
	lastStatus *protocol.Status

	// mu는 lastStatus 필드의 동시 접근을 보호하는 뮤텍스입니다.
	mu sync.Mutex

	// connected는 현재 연결 상태를 나타냅니다.
	connected bool

	// authDone은 인증 완료를 알리는 채널입니다.
	authDone chan struct{}

	// pairDone은 페어링 완료 시 GID를 전달하는 채널입니다.
	pairDone chan [6]byte

	// debug는 디버그 로그 출력 여부입니다.
	debug bool
}

// NewClient는 새로운 BLE 클라이언트를 생성합니다.
//
// 매개변수:
//   - address: BLE 기기의 MAC 주소 (예: "AA:BB:CC:DD:EE:FF")
//   - deviceGid: 인증용 6바이트 기기 키 (ParseDeviceGidHex로 파싱)
//   - debug: true이면 BLE 패킷 송수신 로그를 콘솔에 출력
//
// 주의: 이 함수는 연결을 수행하지 않습니다. Connect()를 별도로 호출해야 합니다.
func NewClient(address string, deviceGid [6]byte, debug bool) *Client {
	return &Client{
		address:   address,
		deviceGid: deviceGid,
		authDone:  make(chan struct{}),
		pairDone:  make(chan [6]byte, 1),
		debug:     debug,
	}
}

// debugf는 디버그 모드일 때 로그를 출력합니다.
// 디버그 모드가 아니면 아무것도 출력하지 않습니다.
func (c *Client) debugf(format string, args ...any) {
	if c.debug {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

// connectBLE는 BLE 연결 + 서비스/특성 검색 + 알림 구독까지 수행합니다.
// 핸드셰이크는 보내지 않습니다.
//
// 과정:
//  1. BLE 어댑터 활성화
//  2. 지정된 주소 또는 "KDO_HotWaterMat" 이름으로 기기 스캔 (10초 타임아웃)
//  3. 기기에 연결
//  4. 서비스/특성 탐색
//  5. CHAR1/CHAR2 알림 구독
//  6. 300ms 대기 (연결 안정화)
func (c *Client) connectBLE() error {
	if err := adapter.Enable(); err != nil {
		return wrapBLEEnableError(err)
	}

	// 서비스 UUID 파싱
	uuid, err := bluetooth.ParseUUID(protocol.ServiceUUID)
	if err != nil {
		return fmt.Errorf("parse service UUID: %w", err)
	}

	// 2단계: 기기 스캔 — 주소 또는 이름으로 찾기
	c.debugf("Scanning for device %s...", c.address)

	var targetAddr bluetooth.Address
	found := make(chan struct{}) // 기기를 찾으면 이 채널을 닫음

	err = adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
		// MAC 주소 일치 또는 기기 이름이 "KDO_HotWaterMat"이면 대상 기기
		if result.Address.String() == c.address || result.LocalName() == protocol.BLEDeviceName {
			targetAddr = result.Address
			a.StopScan() // 기기를 찾았으므로 스캔 중단
			close(found)
		}
	})
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	// 10초 내에 기기를 찾지 못하면 타임아웃
	select {
	case <-found:
		// 기기를 찾음
	case <-time.After(10 * time.Second):
		adapter.StopScan()
		return errors.New("device not found within timeout")
	}

	// 3단계: BLE 연결 수립
	c.debugf("Connecting to %s...", targetAddr.String())
	device, err := adapter.Connect(targetAddr, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	c.device = device

	// 4단계: GATT 서비스 탐색 — 온수매트 서비스 찾기
	c.debugf("Discovering services...")
	services, err := device.DiscoverServices([]bluetooth.UUID{uuid})
	if err != nil {
		return fmt.Errorf("discover services: %w", err)
	}
	if len(services) == 0 {
		return errors.New("service not found")
	}

	// 5단계: 특성(Characteristic) 탐색 — CHAR1(상태), CHAR2(명령) 찾기
	char1UUID, _ := bluetooth.ParseUUID(protocol.Char1UUID)
	char2UUID, _ := bluetooth.ParseUUID(protocol.Char2UUID)

	chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{char1UUID, char2UUID})
	if err != nil {
		return fmt.Errorf("discover characteristics: %w", err)
	}
	if len(chars) < 2 {
		return errors.New("characteristics not found")
	}

	// 특성 할당 (반환 순서가 보장되지 않으므로 UUID로 구분)
	for i := range chars {
		charUUID := chars[i].UUID().String()
		switch charUUID {
		case protocol.Char1UUID:
			c.statusChar = chars[i] // CHAR1 = STATUS 알림 수신
		case protocol.Char2UUID:
			c.cmdChar = chars[i]    // CHAR2 = 명령 쓰기 + 인증 응답
		}
	}

	// 6단계: CHAR1에 STATUS 알림 구독
	// 매트가 주기적으로 보내는 상태 패킷을 수신하여 lastStatus에 저장합니다.
	c.debugf("Subscribing to STATUS notifications...")
	err = c.statusChar.EnableNotifications(func(buf []byte) {
		c.debugf("STATUS: %s", protocol.FormatPacket(buf))
		st, err := protocol.ParseStatus(buf)
		if err == nil {
			c.mu.Lock()
			c.lastStatus = st
			c.mu.Unlock()
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe CHAR1: %w", err)
	}

	// 7단계: CHAR2에 인증 알림 구독
	// 핸드셰이크 후 매트가 보내는 B2F1 인증 응답을 수신합니다.
	// authType이 0x02(인증 완료)이면 authDone 채널을 닫아 Connect가 계속 진행됩니다.
	c.debugf("Subscribing to AUTH notifications...")
	err = c.cmdChar.EnableNotifications(func(buf []byte) {
		// 주의: raw AUTH 패킷을 그대로 로깅하면 페어링 응답(type=0x01)에
		// 포함된 DeviceGid(인증 키)가 노출됩니다. authType을 먼저 확인한 후
		// 타입별로 안전한 로그를 출력합니다.
		authType, err := protocol.ParseAuthResponse(buf)
		if err != nil {
			return
		}
		switch authType {
		case 0x01:
			// 페어링 응답 — DeviceGid 추출
			// DeviceGid(byte[3:9])는 민감한 인증 키이므로 로그에서 마스킹합니다.
			c.debugf("AUTH: B2 F1 01 ** ** ** ** ** ** ... (GID masked, pairing response)")
			gid, err := protocol.ParseDeviceGid(buf)
			if err == nil {
				c.debugf("Pairing response received")
				select {
				case c.pairDone <- gid:
				default:
				}
			}
		case 0x02:
			// 인증 완료 — DeviceGid 미포함이므로 전체 로그 안전
			c.debugf("AUTH: %s (authenticated)", protocol.FormatPacket(buf))
			c.debugf("Authenticated!")
			// authDone 채널을 안전하게 닫기 (이미 닫힌 경우 패닉 방지)
			select {
			case <-c.authDone:
				// 이미 닫혀있으면 아무것도 안 함
			default:
				close(c.authDone)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe CHAR2: %w", err)
	}

	// 8단계: 연결 안정화를 위해 300ms 대기
	// BLE 연결 직후 바로 쓰기를 하면 실패할 수 있음
	time.Sleep(300 * time.Millisecond)
	return nil
}

// Connect는 BLE 기기에 연결하고 기존 DeviceGid로 인증합니다.
func (c *Client) Connect() error {
	if err := c.connectBLE(); err != nil {
		return err
	}

	// DeviceGid를 포함한 핸드셰이크 패킷을 CHAR2로 전송
	handshake := protocol.BuildHandshakeWithKey(c.deviceGid)
	c.debugf("Sending handshake (GID masked)")
	_, err := writeCharacteristic(c.cmdChar, handshake[:])
	if err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}

	// 10단계: 인증 완료 대기 (최대 5초)
	// 매트가 B2F1 응답(authType=0x02)을 보내면 authDone 채널이 닫힘
	select {
	case <-c.authDone:
		c.debugf("Authentication complete")
	case <-time.After(5 * time.Second):
		return errors.New("authentication timeout (wrong DeviceGid?)")
	}

	c.connected = true
	return nil
}

// Pair은 초기 페어링을 수행하여 기기의 DeviceGid를 획득합니다.
// deviceGid가 빈 값인 Client에서 호출해야 합니다.
func (c *Client) Pair() ([6]byte, error) {
	var zeroGid [6]byte
	if c.deviceGid != zeroGid {
		return [6]byte{}, errors.New("Pair must be called on a Client with an empty DeviceGid")
	}

	if err := c.connectBLE(); err != nil {
		return [6]byte{}, err
	}

	// 초기 페어링 핸드셰이크 전송 (GID 없이)
	handshake := protocol.BuildHandshake()
	c.debugf("Sending pairing handshake: %s", protocol.FormatPacket(handshake[:]))
	_, err := writeCharacteristic(c.cmdChar, handshake[:])
	if err != nil {
		return [6]byte{}, fmt.Errorf("write pairing handshake: %w", err)
	}

	// B2F1 type=0x01 페어링 응답 대기
	select {
	case gid := <-c.pairDone:
		c.debugf("Pairing complete")
		c.deviceGid = gid
		c.connected = true
		return gid, nil
	case <-time.After(10 * time.Second):
		return [6]byte{}, errors.New("pairing timeout - is the mat in pairing mode?")
	}
}

// Disconnect는 BLE 연결을 해제합니다.
// Connect/Pair이 완료되지 않은 상태에서도 안전하게 호출 가능합니다.
func (c *Client) Disconnect() error {
	c.connected = false
	var zeroDevice bluetooth.Device
	if c.device == zeroDevice {
		return nil
	}
	return c.device.Disconnect()
}


// ClearStatus는 캐시된 STATUS 데이터를 초기화합니다.
// verify 등에서 최신 상태를 강제로 다시 수신받을 때 사용합니다.
func (c *Client) ClearStatus() {
	c.mu.Lock()
	c.lastStatus = nil
	c.mu.Unlock()
}
// GetStatus는 매트의 최신 STATUS 데이터를 반환합니다.
//
// CHAR1 알림을 통해 자동으로 수신된 lastStatus를 반환합니다.
// 아직 STATUS를 받지 못한 경우, timeout까지 100ms 간격으로 폴링합니다.
//
// 매개변수:
//   - timeout: STATUS 수신 대기 최대 시간 (예: 5 * time.Second)
//
// 반환값:
//   - 성공: 최신 Status 데이터
//   - 실패: timeout 내에 STATUS를 받지 못하면 오류
func (c *Client) GetStatus(timeout time.Duration) (*protocol.Status, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		st := c.lastStatus
		c.mu.Unlock()
		if st != nil {
			return st, nil
		}
		// STATUS 수신을 기다리며 100ms 간격으로 확인
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("no status received within timeout")
}

// SetTemp는 매트의 목표 온도를 설정합니다.
//
// 현재 STATUS에서 현재 온도를 읽은 후, 지정된 목표 온도로 HEAT 명령을 전송합니다.
//
// 매개변수:
//   - side: 제어할 면 (protocol.SideLeft, SideRight, SideBoth)
//   - leftTemp: 왼쪽 목표 온도 (28.0~48.0°C, 0.5°C 단위)
//   - rightTemp: 오른쪽 목표 온도 (28.0~48.0°C, 0.5°C 단위)
//
// 내부 동작:
//  1. GetStatus로 현재 온도(원시 바이트) 획득 (3초 타임아웃)
//  2. EncodeTemp로 목표 온도를 바이트로 인코딩
//  3. BuildHeat로 HEAT 명령 패킷 생성
//  4. CHAR2(cmdChar)로 패킷 전송
func (c *Client) SetTemp(side byte, leftTemp, rightTemp float64) error {
	// 현재 STATUS에서 현재 온도 원시 바이트를 가져옴
	st, err := c.GetStatus(3 * time.Second)
	if err != nil {
		return fmt.Errorf("get current status: %w", err)
	}

	// 목표 온도를 BLE 바이트로 인코딩
	leftTgt, err := protocol.EncodeTemp(leftTemp)
	if err != nil {
		return fmt.Errorf("encode left temp: %w", err)
	}
	rightTgt, err := protocol.EncodeTemp(rightTemp)
	if err != nil {
		return fmt.Errorf("encode right temp: %w", err)
	}

	// HEAT 명령 패킷 생성 및 전송
	pkt := protocol.BuildHeat(side, st.LeftCurrentRaw, st.RightCurrentRaw, leftTgt, rightTgt)
	c.debugf("HEAT: %s", protocol.FormatPacket(pkt[:]))
	_, err = writeCharacteristic(c.cmdChar, pkt[:])
	if err != nil {
		return fmt.Errorf("write set_temp command: %w", err)
	}
	return nil
}

// PowerOn은 매트의 전원을 켭니다.
//
// 가능하면 현재 STATUS의 온도 데이터를 포함하여 전원 ON 명령을 보냅니다.
// STATUS를 아직 받지 못한 경우(연결 직후 등)에는 온도를 0으로 설정하여 전송합니다.
//
// 동작 과정:
//  1. GetStatus로 현재 온도 조회 시도 (3초 타임아웃)
//  2. 성공: 현재 온도 포함 PowerOn 패킷 전송
//  3. 실패: 온도=0으로 폴백 PowerOn 패킷 전송
func (c *Client) PowerOn() error {
	st, err := c.GetStatus(3 * time.Second)
	if err != nil {
		// STATUS 미수신 시 온도를 0으로 설정하여 폴백 전송
		pkt := protocol.BuildPowerOn(0, 0)
		c.debugf("POWER ON: %s", protocol.FormatPacket(pkt[:]))
		_, err = writeCharacteristic(c.cmdChar, pkt[:])
		if err != nil {
			return fmt.Errorf("write power_on command (fallback, status unavailable): %w", err)
		}
		return nil
	}

	// 현재 온도를 포함하여 전원 ON 명령 전송
	pkt := protocol.BuildPowerOn(st.LeftCurrentRaw, st.RightCurrentRaw)
	c.debugf("POWER ON: %s", protocol.FormatPacket(pkt[:]))
	_, err = writeCharacteristic(c.cmdChar, pkt[:])
	if err != nil {
		return fmt.Errorf("write power_on command: %w", err)
	}
	return nil
}

// PowerOff는 매트의 전원을 끕니다.
//
// 전원을 끈 후에도 매트는 BLE 광고를 계속하므로,
// BLE를 통해 다시 전원을 켤 수 있습니다 (물리 버튼 불필요).
//
// PowerOn과 마찬가지로, STATUS 미수신 시 온도를 0으로 폴백합니다.
func (c *Client) PowerOff() error {
	st, err := c.GetStatus(3 * time.Second)
	if err != nil {
		// STATUS 미수신 시 온도를 0으로 설정하여 폴백 전송
		pkt := protocol.BuildPowerOff(0, 0)
		c.debugf("POWER OFF: %s", protocol.FormatPacket(pkt[:]))
		_, err = writeCharacteristic(c.cmdChar, pkt[:])
		if err != nil {
			return fmt.Errorf("write power_off command (fallback, status unavailable): %w", err)
		}
		return nil
	}

	// 현재 온도를 포함하여 전원 OFF 명령 전송
	pkt := protocol.BuildPowerOff(st.LeftCurrentRaw, st.RightCurrentRaw)
	c.debugf("POWER OFF: %s", protocol.FormatPacket(pkt[:]))
	_, err = writeCharacteristic(c.cmdChar, pkt[:])
	if err != nil {
		return fmt.Errorf("write power_off command: %w", err)
	}
	return nil
}
