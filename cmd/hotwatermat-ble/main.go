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
//	hotwatermat-ble status --address AA:BB:CC:DD:EE:FF --device-gid A1B2C3D4E5F6
//
//	# 양쪽 매트 온도 35°C로 설정
//	hotwatermat-ble temp --left 35 --right 35
//
//	# 환경변수 사용
//	export HOTWATERMAT_ADDRESS=AA:BB:CC:DD:EE:FF
//	export HOTWATERMAT_DEVICE_GID=A1B2C3D4E5F6
//	hotwatermat-ble status
//
//	# 다른 프로그램이 읽을 JSON 출력
//	hotwatermat-ble status --json
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/minjun0219/hotwatermat-ble/pkg/ble"
	"github.com/minjun0219/hotwatermat-ble/pkg/config"
	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
	"github.com/spf13/cobra"
)

// version은 빌드 시 ldflags로 주입됩니다.
// 예: go build -ldflags="-X main.version=v0.1.0"
var version = "dev"

// scanTimeout은 BLE 기기 스캔 시간입니다.
const scanTimeout = 5 * time.Second

// postCommandDelay는 명령 전송 후 매트가 처리할 시간을 확보하기 위한 대기 시간입니다.
const postCommandDelay = 2 * time.Second

// 전역 CLI 플래그 변수들
var (
	address    string  // BLE 기기 주소 (--address 또는 HOTWATERMAT_ADDRESS 환경변수)
	deviceGid  string  // 기기 인증 키 (--device-gid 또는 HOTWATERMAT_DEVICE_GID 환경변수)
	debug      bool    // 디버그 출력 활성화 (--debug)
	leftTemp   float64 // 왼쪽 목표 온도 (--left, temp 명령에서 사용)
	rightTemp  float64 // 오른쪽 목표 온도 (--right, temp 명령에서 사용)
	resetSetup bool    // 설정 초기화 (--reset-setup)
	verify     bool    // 명령 실행 후 상태 확인 (--verify)
	standbyL   bool    // 왼쪽을 대기로 (--left, standby 명령에서 사용)
	standbyR   bool    // 오른쪽을 대기로 (--right, standby 명령에서 사용)
	jsonOutput bool    // 결과와 오류를 JSON으로 stdout에 출력 (--json)
)

// main은 CLI의 진입점입니다.
// cobra 라이브러리를 사용하여 명령어를 파싱하고 실행합니다.
func main() {
	disconnectOnSignal()
	err := rootCmd.Execute()
	// 시그널을 받아 연결을 해제하는 중이면 핸들러가 프로세스를 끝낼 때까지 기다림.
	// 먼저 끝나 버리면 해제가 끝나기 전에 종료되고, 시그널 대신 exit 코드로 끝남
	select {
	case <-interrupted:
		time.Sleep(signalDisconnectTimeout + time.Second)
	default:
	}
	if err != nil {
		// --json이면 오류도 JSON으로 stdout에 출력 (cobra의 Error: 출력은 PersistentPreRun에서 끔).
		// --json보다 앞의 플래그에서 파싱이 실패하면 jsonOutput이 설정되지 않으므로 인자도 확인
		if jsonOutput || slices.Contains(os.Args[1:], "--json") {
			_ = printJSON(errorJSON{Code: errorCode(err), Error: err.Error()})
		}
		os.Exit(1)
	}
}

// rootCmd는 CLI의 루트 명령입니다.
// 하위 명령(scan, status, temp, on, off)이 등록됩니다.
var rootCmd = &cobra.Command{
	Use:   "hotwatermat-ble",
	Short: "Control a BLE hot water mat",
	Long:  "CLI tool to control a " + protocol.BLEDeviceName + " device via Bluetooth Low Energy.",
	// Version을 지정하면 cobra가 --version 플래그를 만들어 줍니다 (install.sh가 기존 버전 확인에 사용).
	Version: version,
	// --json이면 cobra가 Error:와 사용법을 출력하지 않게 함 — 오류는 main이 JSON으로 출력
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if jsonOutput {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
		}
	},
}

// versionCmd는 버전 정보를 출력합니다.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hotwatermat-ble %s\n", version)
	},
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
		// --json이면 진행 안내를 stderr로 보내 stdout에는 JSON만 남김
		progress := os.Stdout
		if jsonOutput {
			progress = os.Stderr
		}
		fmt.Fprintf(progress, "Scanning for devices (%s)...\n", scanTimeout)
		results, err := ble.Scan(scanTimeout, protocol.BLEDeviceName)
		if err != nil {
			return err
		}
		devices := uniqueDevices(results)
		if jsonOutput {
			return printJSON(newScanJSON(devices))
		}
		if len(devices) == 0 {
			fmt.Println("No devices found.")
			return nil
		}
		// 발견된 기기 목록 출력 (신호가 강한 순)
		for _, r := range devices {
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
		auditAfter(st)

		if jsonOutput {
			return printJSON(newStatusJSON(st))
		}
		printStatus(st)
		return nil
	},
}

// tempCmd는 온도 설정 명령입니다.
//
// --left와 --right 플래그로 목표 온도를 지정합니다.
// 한쪽만 지정하면 반대쪽은 현재 목표 온도를 유지하고, 양쪽 모두 지정하면 동시 제어합니다.
//
// 동작:
//   - --left만 지정: side=왼쪽, 오른쪽은 현재 목표 온도 유지
//   - --right만 지정: side=오른쪽, 왼쪽은 현재 목표 온도 유지
//   - 양쪽 모두 지정: side=양쪽
//   - 양쪽 모두 미지정: 오류 반환
var tempCmd = &cobra.Command{
	Use:   "temp",
	Short: "Set target temperature",
	RunE: func(cmd *cobra.Command, args []string) error {
		leftChanged := cmd.Flags().Changed("left")
		rightChanged := cmd.Flags().Changed("right")

		// 양쪽 모두 미지정 시 오류
		if !leftChanged && !rightChanged {
			return withCode(codeInvalidArgument, fmt.Errorf("specify --left and/or --right temperature"))
		}

		// 범위(28.0~48.0)와 0.5°C 단위를 연결 전에 검증 — 잘못된 값이면 BLE 스캔·연결을 기다리지 않고 바로 실패
		if leftChanged {
			if _, err := protocol.EncodeTemp(leftTemp); err != nil {
				return withCode(codeInvalidArgument, fmt.Errorf("invalid --left: %w", err))
			}
		}
		if rightChanged {
			if _, err := protocol.EncodeTemp(rightTemp); err != nil {
				return withCode(codeInvalidArgument, fmt.Errorf("invalid --right: %w", err))
			}
		}

		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect()

		// 한쪽만 지정된 경우, 현재 상태를 조회하여 반대쪽 목표 온도 유지
		side := protocol.SideBoth
		if leftChanged != rightChanged {
			st, err := client.GetStatus(5 * time.Second)
			if err != nil {
				return fmt.Errorf("failed to get current status: %w", err)
			}
			if leftChanged && !rightChanged {
				side = protocol.SideLeft
				rightTemp = st.RightTarget
				if rightTemp == 0 {
					rightTemp = leftTemp // 전원 OFF 상태면 동일 온도 사용
				}
			} else {
				side = protocol.SideRight
				leftTemp = st.LeftTarget
				if leftTemp == 0 {
					leftTemp = rightTemp // 전원 OFF 상태면 동일 온도 사용
				}
			}
		}

		// 온도 설정 명령 전송
		auditSent(map[string]any{"side": protocol.SideName(side), "left": leftTemp, "right": rightTemp})
		err = client.SetTemp(side, leftTemp, rightTemp)
		if err != nil {
			return err
		}
		time.Sleep(postCommandDelay) // 매트가 명령을 처리할 시간 확보

		// --verify: 지정한 쪽의 목표 온도가 실제로 바뀌었는지 확인 (대기인 쪽은 명령을 무시할 수 있음, #69)
		check := func(st *protocol.Status) error {
			return checkTempApplied(st, leftChanged, rightChanged, leftTemp, rightTemp)
		}
		if jsonOutput {
			return printCommandResult(client, commandJSON{Sent: "temp", Left: leftTemp, Right: rightTemp}, check)
		}
		fmt.Printf("Temperature set: left=%.1f°C right=%.1f°C\n", leftTemp, rightTemp)

		if verify {
			return verifyStatus(client, check)
		}
		return nil
	},
}

// standbyCmd는 한쪽 매트를 '운전 대기'로 바꾸는 명령입니다.
//
// --left 또는 --right 중 하나를 지정합니다. 반대쪽은 지금 목표 온도로 계속 운전합니다.
// 양쪽을 동시에 대기로 둘 수는 없으므로(공식 앱도 막음), 반대쪽이 이미 대기면 오류를 반환합니다.
// 대기인 쪽을 다시 운전하려면 temp로 그쪽 온도를 정합니다.
var standbyCmd = &cobra.Command{
	Use:   "standby",
	Short: "Put one side in standby (the other keeps running)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if standbyL == standbyR {
			return withCode(codeInvalidArgument, fmt.Errorf("specify exactly one of --left or --right"))
		}
		side, name := protocol.SideLeft, "left"
		if standbyR {
			side, name = protocol.SideRight, "right"
		}

		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect()

		if err := client.SetStandby(side); err != nil {
			if errors.Is(err, ble.ErrPoweredOff) || errors.Is(err, ble.ErrOtherSideStandby) {
				return withCode(codeInvalidArgument, err)
			}
			return err
		}
		time.Sleep(postCommandDelay) // 매트가 명령을 처리할 시간 확보

		// --verify: 지정한 쪽이 실제로 대기가 됐는지 확인
		check := func(st *protocol.Status) error {
			standby := st.LeftStandby
			if side == protocol.SideRight {
				standby = st.RightStandby
			}
			if !standby {
				return withCode(codeNotApplied, fmt.Errorf("not applied: %s side is not in standby", name))
			}
			return nil
		}
		if jsonOutput {
			return printCommandResult(client, commandJSON{Sent: "standby", Side: name}, check)
		}
		fmt.Printf("Standby set: %s\n", name)

		if verify {
			return verifyStatus(client, check)
		}
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

		auditSent(map[string]any{"power": "on"})
		if err := client.PowerOn(); err != nil {
			return err
		}
		time.Sleep(postCommandDelay) // 매트가 명령을 처리할 시간 확보
		if jsonOutput {
			return printCommandResult(client, commandJSON{Sent: "on"}, nil)
		}
		fmt.Println("Power ON sent.")

		if verify {
			return verifyStatus(client, nil)
		}
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

		auditSent(map[string]any{"power": "off"})
		if err := client.PowerOff(); err != nil {
			return err
		}
		time.Sleep(postCommandDelay) // 매트가 명령을 처리할 시간 확보
		if jsonOutput {
			return printCommandResult(client, commandJSON{Sent: "off"}, nil)
		}
		fmt.Println("Power OFF sent.")

		if verify {
			return verifyStatus(client, nil)
		}
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

		unique := uniqueDevices(results)
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
			fmt.Print("Enter DeviceGid (12-char hex, e.g. A1B2C3D4E5F6): ")
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
			pairClient := newClient(selectedAddr, [6]byte{})
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
		client := newClient(selectedAddr, gid)
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

// printStatus는 매트 상태를 출력합니다.
// statusCmd와 --verify 옵션에서 공통으로 사용됩니다.
func printStatus(st *protocol.Status) {
	if st.PoweredOff {
		fmt.Println("Power: OFF")
		return
	}
	fmt.Printf("Mode:    %s\n", st.ModeName)
	fmt.Printf("Side:    %s\n", protocol.SideName(st.Side))
	fmt.Printf("Water:   %s\n", protocol.WaterLevelName(st.WaterLevel))
	printSide("Left:    ", st.LeftCurrent, st.LeftTarget, st.LeftStandby)
	printSide("Right:   ", st.RightCurrent, st.RightTarget, st.RightStandby)
}

// printSide는 한쪽 매트의 온도를 "현재 → 목표" 형식으로 출력합니다.
// 대기 중에는 목표 온도 바이트에 예전 설정값이 남아 있으므로 현재 온도만 출력합니다.
func printSide(label string, current, target float64, standby bool) {
	if standby {
		fmt.Printf("%s%.1f°C (standby)\n", label, current)
		return
	}
	fmt.Printf("%s%.1f°C → %.1f°C\n", label, current, target)
}

// verifyStatus는 명령 실행 후 상태를 다시 조회하여 출력합니다.
// --verify 플래그가 활성화된 경우에만 호출됩니다. check가 있으면 출력한 상태로 명령이 반영됐는지 확인합니다.
func verifyStatus(client *ble.Client, check func(*protocol.Status) error) error {
	st, err := readStatusAfterCommand(client)
	if err != nil {
		return err
	}
	fmt.Println("\n[Verify]")
	printStatus(st)
	if check != nil {
		return check(st)
	}
	return nil
}

// checkTempApplied는 temp 명령 뒤 다시 읽은 상태에서, 사용자가 지정한 쪽의 목표 온도가 반영됐는지 확인합니다.
// 대기인 쪽은 HEAT 명령을 받아도 대기를 유지하고 설정값도 바꾸지 않을 수 있어서(#69),
// 대기이거나 목표 온도가 다르면 not_applied 오류를 반환합니다. 지정하지 않은 쪽은 확인하지 않습니다.
func checkTempApplied(st *protocol.Status, leftChanged, rightChanged bool, left, right float64) error {
	if st.PoweredOff {
		return withCode(codeNotApplied, errors.New("not applied: the mat is powered off"))
	}
	check := func(name string, standby bool, got, want float64) error {
		if standby {
			return withCode(codeNotApplied, fmt.Errorf("not applied: %s side is still in standby (check it in the app)", name))
		}
		if got != want {
			return withCode(codeNotApplied, fmt.Errorf("not applied: %s target is %.1f°C, expected %.1f°C", name, got, want))
		}
		return nil
	}
	if leftChanged {
		if err := check("left", st.LeftStandby, st.LeftTarget, left); err != nil {
			return err
		}
	}
	if rightChanged {
		if err := check("right", st.RightStandby, st.RightTarget, right); err != nil {
			return err
		}
	}
	return nil
}

// readStatusAfterCommand는 명령 실행 후 1초 대기한 뒤 새로 받은 상태를 반환합니다.
func readStatusAfterCommand(client *ble.Client) (*protocol.Status, error) {
	time.Sleep(1 * time.Second)
	client.ClearStatus() // 캐시된 상태 제거 → 새 STATUS 패킷 대기
	st, err := client.GetStatus(5 * time.Second)
	if err != nil {
		return nil, withCode(codeVerifyFailed, fmt.Errorf("verify failed: %w", err))
	}
	auditAfter(st)
	return st, nil
}

// uniqueDevices는 스캔 결과에서 주소가 같은 중복을 없애고 신호가 강한 순으로 정렬합니다.
// 같은 기기는 광고할 때마다 결과에 다시 들어오므로 주소마다 RSSI가 가장 강한 결과만 남기고,
// RSSI가 같으면 주소 순으로 정렬해 출력 순서를 고정합니다.
func uniqueDevices(results []ble.ScanResult) []ble.ScanResult {
	best := make(map[string]ble.ScanResult)
	for _, r := range results {
		if prev, ok := best[r.Address]; !ok || r.RSSI > prev.RSSI {
			best[r.Address] = r
		}
	}
	unique := make([]ble.ScanResult, 0, len(best))
	for _, r := range best {
		unique = append(unique, r)
	}
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].RSSI != unique[j].RSSI {
			return unique[i].RSSI > unique[j].RSSI // 신호 강한 순
		}
		return unique[i].Address < unique[j].Address // 동일 RSSI면 주소 순
	})
	return unique
}

// connect는 CLI 플래그, 환경변수, 설정 파일을 기반으로 BLE 기기에 연결합니다.
//
// 기기 주소 결정 우선순위:
//  1. --address 플래그
//  2. HOTWATERMAT_ADDRESS 환경변수
//  3. 설정 파일 (setup 명령으로 저장된 값)
//  4. 자동 스캔 (주변 기기 검색)
//
// DeviceGid 결정 우선순위:
//  1. --device-gid 플래그
//  2. HOTWATERMAT_DEVICE_GID 환경변수
//  3. 설정 파일
//
// 연결 성공 시 설정 파일이 없으면 자동 저장합니다.
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
		// 진행 안내는 stderr로 보내 stdout에는 명령 결과만 남김
		fmt.Fprintln(os.Stderr, "No address specified, scanning for devices...")
		results, err := ble.Scan(scanTimeout, protocol.BLEDeviceName)
		if err != nil {
			return nil, fmt.Errorf("auto-scan failed: %w", err)
		}

		unique := uniqueDevices(results)
		if len(unique) == 0 {
			return nil, withCode(codeDeviceNotFound, fmt.Errorf("no %s device found. Make sure the mat is powered on and in range", protocol.BLEDeviceName))
		}
		if len(unique) == 1 {
			// 기기가 1개만 발견되면 자동 선택
			address = unique[0].Address
			fmt.Fprintf(os.Stderr, "Found device: %s (RSSI: %d)\n", address, unique[0].RSSI)
		} else {
			// 기기가 여러 개 발견되면 사용자에게 선택 요청
			fmt.Fprintln(os.Stderr, "Multiple devices found:")
			for i, r := range unique {
				fmt.Fprintf(os.Stderr, "  [%d] %s (RSSI: %d)\n", i+1, r.Address, r.RSSI)
			}
			fmt.Fprintln(os.Stderr, "Please specify --address to select a device.")
			return nil, withCode(codeNotConfigured, fmt.Errorf("multiple devices found, specify --address"))
		}
	}

	// DeviceGid 결정: flag → env → config → error
	var gid [6]byte
	if deviceGid != "" {
		// --device-gid 플래그에서 파싱
		var err error
		gid, err = protocol.ParseDeviceGidHex(deviceGid)
		if err != nil {
			return nil, withCode(codeInvalidArgument, fmt.Errorf("invalid device-gid: %w", err))
		}
	} else if envGid := os.Getenv("HOTWATERMAT_DEVICE_GID"); envGid != "" {
		// 환경변수에서 파싱
		var err error
		gid, err = protocol.ParseDeviceGidHex(envGid)
		if err != nil {
			return nil, withCode(codeInvalidArgument, fmt.Errorf("invalid HOTWATERMAT_DEVICE_GID: %w", err))
		}
	} else if cachedConfig != nil && cachedConfig.DeviceGid != "" {
		var err error
		gid, err = protocol.ParseDeviceGidHex(cachedConfig.DeviceGid)
		if err != nil {
			return nil, withCode(codeNotConfigured, fmt.Errorf("invalid cached device_gid: %w", err))
		}
		if debug {
			fmt.Fprintf(os.Stderr, "Using cached device GID: %s\n", maskDeviceGid(cachedConfig.DeviceGid))
		}
	} else {
		return nil, withCode(codeNotConfigured, fmt.Errorf("no device GID specified. Run 'hotwatermat-ble setup' to pair, or set HOTWATERMAT_DEVICE_GID / --device-gid"))
	}

	// BLE 클라이언트 생성 및 연결 (실패하면 Connect가 연결을 해제함)
	client := newClient(address, gid)
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

// clients는 시그널(Ctrl+C 등)을 받았을 때 연결을 해제해야 하는 BLE 클라이언트 목록입니다.
// Linux(BlueZ)는 프로세스가 끝나도 BLE 연결을 유지합니다. 매트는 연결을 하나만 받으므로,
// 해제하지 않고 종료하면 광고를 멈춰 공식 앱 등 다른 기기가 매트를 찾지 못할 수 있습니다.
var (
	clientsMu sync.Mutex
	clients   []*ble.Client
)

// newClient는 시그널을 받으면 연결이 해제되도록 등록한 BLE 클라이언트를 생성합니다.
func newClient(addr string, gid [6]byte) *ble.Client {
	client := ble.NewClient(addr, gid, debug)
	clientsMu.Lock()
	clients = append(clients, client)
	clientsMu.Unlock()
	return client
}

// signalDisconnectTimeout은 시그널을 받은 뒤 연결 해제를 기다리는 최대 시간입니다.
// 연결을 맺는 중이면 그 연결이 끝나야 해제할 수 있어서, 기기가 응답하지 않을 때를 대비해 상한을 둡니다.
const signalDisconnectTimeout = 10 * time.Second

// interrupted는 시그널을 받아 연결 해제를 시작하면 닫히는 채널입니다.
var interrupted = make(chan struct{})

// disconnectOnSignal은 SIGINT/SIGTERM/SIGHUP을 받으면 등록된 클라이언트의 연결을 모두 해제한 뒤
// 같은 시그널로 종료합니다. 시그널로 종료하면 defer가 실행되지 않으므로 여기서 따로 해제합니다.
func disconnectOnSignal() {
	var sigs []os.Signal
	for _, s := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} {
		// nohup의 SIGHUP처럼 무시하도록 실행된 시그널은 그대로 둠 (Notify가 무시 설정을 풀어 버림)
		if !signal.Ignored(s) {
			sigs = append(sigs, s)
		}
	}
	if len(sigs) == 0 {
		return // 인자 없는 Notify는 모든 시그널을 받으므로 호출하지 않음
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, sigs...)

	go func() {
		sig := <-sigCh
		close(interrupted)
		// 해제가 멈추더라도 한 번 더 Ctrl+C를 누르면 바로 종료되도록 기본 동작을 되돌림
		signal.Stop(sigCh)

		// Disconnect는 이후의 쓰기를 막아 취소한 뒤에 명령이 나가지 않게 하고,
		// 연결을 맺는 중이면 그 연결이 해제될 때까지 기다림. 기기가 응답하지 않으면 상한까지만 기다림
		done := make(chan struct{})
		go func() {
			clientsMu.Lock()
			for _, client := range clients {
				client.Disconnect()
			}
			clientsMu.Unlock()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(signalDisconnectTimeout):
		}
		auditInterrupted(sig)

		// 기본 동작으로 되돌린 같은 시그널을 자신에게 보내 시그널로 종료함.
		// exit 코드로 끝내면 셸 스크립트가 Ctrl+C 뒤에도 다음 명령을 계속 실행함
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			_ = p.Signal(sig)
			time.Sleep(100 * time.Millisecond)
		}
		// 시그널을 다시 보낼 수 없는 환경(Windows 등)에서는 관례대로 128 + 시그널 번호로 종료
		code := 1
		if s, ok := sig.(syscall.Signal); ok {
			code = 128 + int(s)
		}
		os.Exit(code)
	}()
}

// init은 cobra 명령어의 플래그와 하위 명령을 등록합니다.
func init() {
	rootCmd.PersistentFlags().StringVar(&address, "address", "", "BLE device address (or HOTWATERMAT_ADDRESS env)")
	rootCmd.PersistentFlags().StringVar(&deviceGid, "device-gid", "", "Device authentication key hex (or HOTWATERMAT_DEVICE_GID env)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug output")

	tempCmd.Flags().Float64Var(&leftTemp, "left", 0, "Left side target temperature (28.0-48.0)")
	tempCmd.Flags().Float64Var(&rightTemp, "right", 0, "Right side target temperature (28.0-48.0)")
	standbyCmd.Flags().BoolVar(&standbyL, "left", false, "Put the left side in standby")
	standbyCmd.Flags().BoolVar(&standbyR, "right", false, "Put the right side in standby")

	setupCmd.Flags().BoolVar(&resetSetup, "reset", false, "Clear cached device config")

	// --verify: 명령 실행 후 자동 상태 확인
	onCmd.Flags().BoolVar(&verify, "verify", false, "Verify status after command")
	offCmd.Flags().BoolVar(&verify, "verify", false, "Verify status after command")
	tempCmd.Flags().BoolVar(&verify, "verify", false, "Verify status after command")
	standbyCmd.Flags().BoolVar(&verify, "verify", false, "Verify status after command")

	// --json: 결과를 JSON 한 줄로 stdout에 출력 (다른 프로그램이 CLI를 띄워 읽을 때)
	for _, c := range []*cobra.Command{scanCmd, statusCmd, tempCmd, standbyCmd, onCmd, offCmd} {
		c.Flags().BoolVar(&jsonOutput, "json", false, "Print the result (or error) as JSON on stdout")
	}

	// 잘못된 플래그 값(--left abc 등)은 --json 오류에서 invalid_argument로 분류 (메시지는 그대로)
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return withCode(codeInvalidArgument, err)
	})

	// 매트에 닿는 명령은 실행할 때마다 감사 기록을 남김 (끄는 옵션 없음)
	for _, c := range auditCommands() {
		withAudit(c)
	}

	// 루트 명령에 하위 명령 등록
	// --version 출력을 version 하위 명령과 같은 형식으로 맞춤
	rootCmd.SetVersionTemplate("hotwatermat-ble {{.Version}}\n")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(scanCmd, statusCmd, tempCmd, standbyCmd, onCmd, offCmd, setupCmd)
}
