// 온수매트 BLE 제어용 CLI(명령줄 인터페이스) 도구입니다.
//
// 사용 가능한 명령:
//   - scan: 주변 BLE 온수매트 기기를 검색합니다
//   - status: 매트의 현재 상태(온도, 모드, 수위)를 표시합니다
//   - temp: 매트의 목표 온도를 설정합니다
//   - on: 매트 전원을 켭니다
//   - off: 매트 전원을 끕니다
//
// 사용 예시:
//
//	# 기기 검색
//	hotwatermat-ble scan
//
//	# 상태 확인
//	hotwatermat-ble status --address AA:BB:CC:DD:EE:FF --device-gid 13CE3CC53E5A
//
//	# 양쪽 매트 온도 35°C로 설정
//	hotwatermat-ble temp --left 35 --right 35
//
//	# 환경변수 사용
//	export HOTWATERMAT_ADDRESS=AA:BB:CC:DD:EE:FF
//	export HOTWATERMAT_DEVICE_GID=13CE3CC53E5A
//	hotwatermat-ble status
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/minjun0219/hotwatermat-ble/pkg/ble"
	"github.com/minjun0219/hotwatermat-ble/pkg/config"
	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
	"github.com/spf13/cobra"
)

// scanTimeout은 BLE 기기 스캔 시간입니다.
const scanTimeout = 5 * time.Second

// 전역 CLI 플래그 변수들
var (
	address    string  // BLE 기기 주소 (--address 또는 HOTWATERMAT_ADDRESS 환경변수)
	deviceGid  string  // 기기 인증 키 (--device-gid 또는 HOTWATERMAT_DEVICE_GID 환경변수)
	debug      bool    // 디버그 출력 활성화 (--debug)
	leftTemp   float64 // 왼쪽 목표 온도 (--left, temp 명령에서 사용)
	rightTemp  float64 // 오른쪽 목표 온도 (--right, temp 명령에서 사용)
	resetSetup bool    // 설정 초기화 (--reset-setup)
)

// main은 CLI의 진입점입니다.
// cobra 라이브러리를 사용하여 명령어를 파싱하고 실행합니다.
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// rootCmd는 CLI의 루트 명령입니다.
// 하위 명령(scan, status, temp, on, off)이 등록됩니다.
var rootCmd = &cobra.Command{
	Use:   "hotwatermat-ble",
	Short: "Control a BLE hot water mat",
	Long:  "CLI tool to control a " + protocol.BLEDeviceName + " device via Bluetooth Low Energy.",
}

// scanCmd는 BLE 기기 검색 명령입니다.
//
// 5초 동안 주변의 "KDO_HotWaterMat" 기기를 검색하고
// 발견된 기기의 주소, 이름, 신호 강도(RSSI)를 출력합니다.
// 연결이나 인증은 수행하지 않으므로 --device-gid가 필요 없습니다.
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for BLE hot water mat devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Scanning for devices (%s)...\n", scanTimeout)
		results, err := ble.Scan(scanTimeout, protocol.BLEDeviceName)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			fmt.Println("No devices found.")
			return nil
		}
		// 발견된 기기 목록 출력
		for _, r := range results {
			fmt.Printf("  %s  %s  (RSSI: %d)\n", r.Address, r.Name, r.RSSI)
		}
		return nil
	},
}

// statusCmd는 매트 상태 조회 명령입니다.
//
// BLE로 기기에 연결하고 인증한 후, STATUS 알림을 수신하여 현재 상태를 표시합니다.
// 출력 정보: 모드, 좌/우 선택, 수위, 왼쪽/오른쪽 현재→목표 온도
// 전원이 꺼져 있으면 "Power: OFF"만 표시합니다.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current mat status",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 기기에 연결 및 인증
		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect() // 함수 종료 시 연결 해제

		// STATUS 알림 수신 (최대 5초 대기)
		st, err := client.GetStatus(5 * time.Second)
		if err != nil {
			return err
		}

		// 전원 꺼짐 상태면 간단히 표시
		if st.PoweredOff {
			fmt.Println("Power: OFF")
			return nil
		}

		// 현재 상태 출력
		fmt.Printf("Mode:    %s\n", st.ModeName)                                    // 동작 모드
		fmt.Printf("Side:    %s\n", protocol.SideName(st.Side))                      // 좌/우 선택
		fmt.Printf("Water:   %s\n", protocol.WaterLevelName(st.WaterLevel))          // 수위
		fmt.Printf("Left:    %.1f°C → %.1f°C\n", st.LeftCurrent, st.LeftTarget)     // 왼쪽: 현재→목표
		fmt.Printf("Right:   %.1f°C → %.1f°C\n", st.RightCurrent, st.RightTarget)   // 오른쪽: 현재→목표
		return nil
	},
}

// tempCmd는 온도 설정 명령입니다.
//
// --left와 --right 플래그로 목표 온도를 지정합니다.
// 한쪽만 지정하면 해당 면만 제어하고, 양쪽 모두 지정하면 동시 제어합니다.
//
// 자동 보정 동작:
//   - --left만 지정: side=왼쪽, 오른쪽 온도는 왼쪽과 동일하게 설정
//   - --right만 지정: side=오른쪽, 왼쪽 온도는 오른쪽과 동일하게 설정
//   - 양쪽 모두 지정: side=양쪽
//   - 양쪽 모두 미지정: 오류 반환
var tempCmd = &cobra.Command{
	Use:   "temp",
	Short: "Set target temperature",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect()

		// 좌/우 선택 결정
		side := protocol.SideBoth
		if leftTemp > 0 && rightTemp == 0 {
			// 왼쪽만 지정된 경우
			side = protocol.SideLeft
			rightTemp = leftTemp // 오른쪽도 같은 온도로 설정 (패킷 구성에 필요)
		} else if rightTemp > 0 && leftTemp == 0 {
			// 오른쪽만 지정된 경우
			side = protocol.SideRight
			leftTemp = rightTemp // 왼쪽도 같은 온도로 설정 (패킷 구성에 필요)
		}

		// 양쪽 모두 미지정 시 오류
		if leftTemp == 0 && rightTemp == 0 {
			return fmt.Errorf("specify --left and/or --right temperature")
		}

		// 온도 설정 명령 전송
		err = client.SetTemp(side, leftTemp, rightTemp)
		if err != nil {
			return err
		}
		fmt.Printf("Temperature set: left=%.1f°C right=%.1f°C\n", leftTemp, rightTemp)
		return nil
	},
}

// onCmd는 전원 켜기 명령입니다.
//
// BLE를 통해 매트 전원을 켭니다.
// 기기가 이미 켜져 있는 상태에서 보내도 오류는 발생하지 않습니다.
var onCmd = &cobra.Command{
	Use:   "on",
	Short: "Power on the mat",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect()

		if err := client.PowerOn(); err != nil {
			return err
		}
		fmt.Println("Power ON sent.")
		return nil
	},
}

// offCmd는 전원 끄기 명령입니다.
//
// 전원을 끈 후에도 매트는 BLE 광고를 계속하므로,
// 'on' 명령으로 다시 전원을 켤 수 있습니다.
var offCmd = &cobra.Command{
	Use:   "off",
	Short: "Power off the mat",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect()

		if err := client.PowerOff(); err != nil {
			return err
		}
		fmt.Println("Power OFF sent.")
		return nil
	},
}

// setupCmd는 대화형 초기 설정 명령입니다.
// 스캔, 페어링, 연결 검증, 설정 저장을 순서대로 수행합니다.
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup: scan, pair, and save device config",
	Long:  "Scans for devices, pairs to acquire DeviceGid, verifies connection, and saves config to the platform-specific config directory used by this tool",
	RunE: func(cmd *cobra.Command, args []string) error {
		// --reset 처리
		if resetSetup {
			if err := config.Delete(); err != nil {
				return fmt.Errorf("failed to delete config: %w", err)
			}
			if path, pathErr := config.ConfigPath(); pathErr == nil {
				fmt.Printf("Config deleted: %s\n", path)
			} else {
				fmt.Println("Config deleted.")
			}
			return nil
		}

		scanner := bufio.NewScanner(os.Stdin)

		// 기존 설정 확인
		existing, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load existing config: %w", err)
		}
		if existing != nil {
			fmt.Printf("Existing config found (address: %s, GID: %s)\n", existing.Address, maskDeviceGid(existing.DeviceGid))
			fmt.Print("Overwrite? [y/N]: ")
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("failed to read input: %w", err)
				}
				fmt.Println("Setup cancelled.")
				return nil
			}
			answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if answer != "y" && answer != "yes" {
				fmt.Println("Setup cancelled.")
				return nil
			}
		}

		// [1/4] BLE 스캔
		fmt.Println("\n[1/4] Scanning for devices...")
		results, err := ble.Scan(scanTimeout, protocol.BLEDeviceName)
		if err != nil {
			return fmt.Errorf("scan failed (check BLE permissions): %w", err)
		}

		// 중복 제거 (RSSI 기준)
		best := make(map[string]ble.ScanResult)
		for _, r := range results {
			if prev, ok := best[r.Address]; !ok || r.RSSI > prev.RSSI {
				best[r.Address] = r
			}
		}
		var unique []ble.ScanResult
		for _, r := range best {
			unique = append(unique, r)
		}
		sort.Slice(unique, func(i, j int) bool {
			if unique[i].RSSI != unique[j].RSSI {
				return unique[i].RSSI > unique[j].RSSI
			}
			return unique[i].Address < unique[j].Address
		})

		if len(unique) == 0 {
			return fmt.Errorf("no %s device found. Make sure the mat is powered on and in range", protocol.BLEDeviceName)
		}

		// 기기 선택
		var selectedAddr string
		if len(unique) == 1 {
			selectedAddr = unique[0].Address
			fmt.Printf("Found device: %s (RSSI: %d)\n", selectedAddr, unique[0].RSSI)
		} else {
			fmt.Printf("Found %d devices:\n", len(unique))
			for i, r := range unique {
				fmt.Printf("  [%d] %s (RSSI: %d)\n", i+1, r.Address, r.RSSI)
			}
			fmt.Print("Select device number: ")
			if !scanner.Scan() {
				return fmt.Errorf("no input received")
			}
			var idx int
			if _, err := fmt.Sscanf(scanner.Text(), "%d", &idx); err != nil || idx < 1 || idx > len(unique) {
				return fmt.Errorf("invalid selection: %s", scanner.Text())
			}
			selectedAddr = unique[idx-1].Address
		}

		// [2/4] DeviceGid 취득 방식 선택
		fmt.Println("\n[2/4] DeviceGid acquisition")
		fmt.Println("  [1] Enter existing GID (from official app)")
		fmt.Println("  [2] New pairing (mat must be in pairing mode)")
		fmt.Print("Select [1/2]: ")

		if !scanner.Scan() {
			return fmt.Errorf("no input received")
		}
		choice := strings.TrimSpace(scanner.Text())

		var gid [6]byte
		switch choice {
		case "1":
			// 사용자가 직접 GID 입력
			fmt.Print("Enter DeviceGid (12-char hex, e.g. 13CE3CC53E5A): ")
			if !scanner.Scan() {
				return fmt.Errorf("no input received")
			}
			gidHex := strings.TrimSpace(scanner.Text())
			gid, err = protocol.ParseDeviceGidHex(gidHex)
			if err != nil {
				return fmt.Errorf("invalid GID: %w", err)
			}

		case "2":
			// BLE 페어링으로 GID 자동 취득
			fmt.Println("Starting pairing... (make sure mat is in pairing mode)")
			pairClient := ble.NewClient(selectedAddr, [6]byte{}, debug)
			gid, err = pairClient.Pair()
			pairClient.Disconnect()
			if err != nil {
				return fmt.Errorf("pairing failed: %w", err)
			}
			fmt.Printf("DeviceGid acquired: %s\n", maskDeviceGid(protocol.FormatDeviceGid(gid)))

		default:
			return fmt.Errorf("invalid choice: %s", choice)
		}

		// [3/4] 연결 검증
		fmt.Println("\n[3/4] Verifying connection...")
		client := ble.NewClient(selectedAddr, gid, debug)
		defer client.Disconnect()
		if err := client.Connect(); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

		st, err := client.GetStatus(5 * time.Second)
		if err != nil {
			return fmt.Errorf("status check failed: %w", err)
		}

		if st.PoweredOff {
			fmt.Println("Connection verified! (mat is powered off)")
		} else {
			fmt.Printf("Connection verified! Mode: %s, Left: %.1f°C, Right: %.1f°C\n",
				st.ModeName, st.LeftCurrent, st.RightCurrent)
		}

		// [4/4] 설정 저장
		fmt.Println("\n[4/4] Saving config...")
		cfg := &config.DeviceConfig{
			Address:   selectedAddr,
			DeviceGid: protocol.FormatDeviceGid(gid),
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config failed: %w", err)
		}
		if path, pathErr := config.ConfigPath(); pathErr == nil {
			fmt.Printf("Config saved to: %s\n", path)
		} else {
			fmt.Println("Config saved.")
		}
		fmt.Println("\nSetup complete! You can now use commands without --address and --device-gid.")
		return nil
	},
}

// connect는 CLI 플래그, 환경변수, 설정 파일을 기반으로 BLE 기기에 연결합니다.
func connect() (*ble.Client, error) {
	// 기기 주소 결정: flag → env → config → auto-scan
	if address == "" {
		address = os.Getenv("HOTWATERMAT_ADDRESS")
	}

	// config 파일에서 캐시된 설정 로드 (항상 시도)
	var cachedConfig *config.DeviceConfig
	var cachedConfigErr error
	cachedConfig, cachedConfigErr = config.Load()
	if cachedConfigErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: config load failed: %v\n", cachedConfigErr)
	}

	if address == "" && cachedConfig != nil && cachedConfig.Address != "" {
		address = cachedConfig.Address
		fmt.Fprintf(os.Stderr, "Using cached address: %s\n", address)
	}

	if address == "" {
		// 자동 스캔으로 기기 검색 시도
		fmt.Println("No address specified, scanning for devices...")
		results, err := ble.Scan(scanTimeout, protocol.BLEDeviceName)
		if err != nil {
			return nil, fmt.Errorf("auto-scan failed: %w", err)
		}

		// 중복 제거: 같은 주소의 기기 중 RSSI가 가장 강한 것만 유지
		best := make(map[string]ble.ScanResult)
		for _, r := range results {
			if prev, ok := best[r.Address]; !ok || r.RSSI > prev.RSSI {
				best[r.Address] = r
			}
		}

		// 맵을 슬라이스로 변환
		var unique []ble.ScanResult
		for _, r := range best {
			unique = append(unique, r)
		}

		// RSSI 내림차순 정렬 (신호가 강한 기기 우선)
		sort.Slice(unique, func(i, j int) bool {
			if unique[i].RSSI != unique[j].RSSI {
				return unique[i].RSSI > unique[j].RSSI // 신호 강한 순
			}
			return unique[i].Address < unique[j].Address // 동일 RSSI면 주소 순
		})

		if len(unique) == 0 {
			return nil, fmt.Errorf("no %s device found. Make sure the mat is powered on and in range", protocol.BLEDeviceName)
		}
		if len(unique) == 1 {
			// 기기가 1개만 발견되면 자동 선택
			address = unique[0].Address
			fmt.Printf("Found device: %s (RSSI: %d)\n", address, unique[0].RSSI)
		} else {
			// 기기가 여러 개 발견되면 사용자에게 선택 요청
			fmt.Println("Multiple devices found:")
			for i, r := range unique {
				fmt.Printf("  [%d] %s (RSSI: %d)\n", i+1, r.Address, r.RSSI)
			}
			fmt.Println("Please specify --address to select a device.")
			return nil, fmt.Errorf("multiple devices found, specify --address")
		}
	}

	// DeviceGid 결정: flag → env → config → error
	var gid [6]byte
	if deviceGid != "" {
		// --device-gid 플래그에서 파싱
		var err error
		gid, err = protocol.ParseDeviceGidHex(deviceGid)
		if err != nil {
			return nil, fmt.Errorf("invalid device-gid: %w", err)
		}
	} else if envGid := os.Getenv("HOTWATERMAT_DEVICE_GID"); envGid != "" {
		// 환경변수에서 파싱
		var err error
		gid, err = protocol.ParseDeviceGidHex(envGid)
		if err != nil {
			return nil, fmt.Errorf("invalid HOTWATERMAT_DEVICE_GID: %w", err)
		}
	} else if cachedConfig != nil && cachedConfig.DeviceGid != "" {
		var err error
		gid, err = protocol.ParseDeviceGidHex(cachedConfig.DeviceGid)
		if err != nil {
			return nil, fmt.Errorf("invalid cached device_gid: %w", err)
		}
		if debug {
			fmt.Fprintf(os.Stderr, "Using cached device GID: %s\n", cachedConfig.DeviceGid)
		}
	} else {
		return nil, fmt.Errorf("no device GID specified. Run 'hotwatermat-ble setup' to pair, or set HOTWATERMAT_DEVICE_GID / --device-gid")
	}

	// BLE 클라이언트 생성 및 연결
	client := ble.NewClient(address, gid, debug)
	if err := client.Connect(); err != nil {
		return nil, err
	}

	// config 파일이 존재하지 않았으면 자동 저장 (Load가 nil,nil 반환 = 파일 없음)
	if cachedConfig == nil && cachedConfigErr == nil {
		if saveErr := config.Save(&config.DeviceConfig{
			Address:   address,
			DeviceGid: protocol.FormatDeviceGid(gid),
		}); saveErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to auto-save config: %v\n", saveErr)
		}
	}

	return client, nil
}

// maskDeviceGid는 DeviceGid(인증 키)를 마스킹하여 로그/터미널 노출을 최소화합니다.
// 앞 2자리 + **** + 뒤 2자리 형태로 표시합니다.
func maskDeviceGid(gid string) string {
	if len(gid) <= 4 {
		return "****"
	}
	return gid[:2] + "****" + gid[len(gid)-2:]
}

// init은 cobra 명령어의 플래그와 하위 명령을 등록합니다.
func init() {
	rootCmd.PersistentFlags().StringVar(&address, "address", "", "BLE device address (or HOTWATERMAT_ADDRESS env)")
	rootCmd.PersistentFlags().StringVar(&deviceGid, "device-gid", "", "Device authentication key hex (or HOTWATERMAT_DEVICE_GID env)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug output")

	tempCmd.Flags().Float64Var(&leftTemp, "left", 0, "Left side target temperature (28.0-48.0)")
	tempCmd.Flags().Float64Var(&rightTemp, "right", 0, "Right side target temperature (28.0-48.0)")

	setupCmd.Flags().BoolVar(&resetSetup, "reset", false, "Clear cached device config")

	// 루트 명령에 하위 명령 등록
	rootCmd.AddCommand(scanCmd, statusCmd, tempCmd, onCmd, offCmd, setupCmd)
}
