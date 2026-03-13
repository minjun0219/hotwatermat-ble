// CLI for controlling a BLE hot water mat.
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

const scanTimeout = 5 * time.Second

var (
	address    string
	deviceGid  string
	debug      bool
	leftTemp   float64
	rightTemp  float64
	resetSetup bool
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "hotwatermat-ble",
	Short: "Control a BLE hot water mat",
	Long:  "CLI tool to control a " + protocol.BLEDeviceName + " device via Bluetooth Low Energy.",
}

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
		for _, r := range results {
			fmt.Printf("  %s  %s  (RSSI: %d)\n", r.Address, r.Name, r.RSSI)
		}
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current mat status",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect()

		st, err := client.GetStatus(5 * time.Second)
		if err != nil {
			return err
		}

		if st.PoweredOff {
			fmt.Println("Power: OFF")
			return nil
		}

		fmt.Printf("Mode:    %s\n", st.ModeName)
		fmt.Printf("Side:    %s\n", protocol.SideName(st.Side))
		fmt.Printf("Water:   %s\n", protocol.WaterLevelName(st.WaterLevel))
		fmt.Printf("Left:    %.1f°C → %.1f°C\n", st.LeftCurrent, st.LeftTarget)
		fmt.Printf("Right:   %.1f°C → %.1f°C\n", st.RightCurrent, st.RightTarget)
		return nil
	},
}

var tempCmd = &cobra.Command{
	Use:   "temp",
	Short: "Set target temperature",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect()

		side := protocol.SideBoth
		if leftTemp > 0 && rightTemp == 0 {
			side = protocol.SideLeft
			rightTemp = leftTemp
		} else if rightTemp > 0 && leftTemp == 0 {
			side = protocol.SideRight
			leftTemp = rightTemp
		}

		if leftTemp == 0 && rightTemp == 0 {
			return fmt.Errorf("specify --left and/or --right temperature")
		}

		err = client.SetTemp(side, leftTemp, rightTemp)
		if err != nil {
			return err
		}
		fmt.Printf("Temperature set: left=%.1f°C right=%.1f°C\n", leftTemp, rightTemp)
		return nil
	},
}

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

var offCmd = &cobra.Command{
	Use:   "off",
	Short: "Power off the mat (requires physical button to restart)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := connect()
		if err != nil {
			return err
		}
		defer client.Disconnect()

		if err := client.PowerOff(); err != nil {
			return err
		}
		fmt.Println("Power OFF sent. Physical button required to restart.")
		return nil
	},
}

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
			pairClient.Disconnect() // 검증 전에 즉시 연결 해제 (기기가 단일 연결만 지원할 수 있음)
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

func connect() (*ble.Client, error) {
	// Address 해석: flag → env → config → auto-scan
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
		// Auto-scan for device
		fmt.Println("No address specified, scanning for devices...")
		results, err := ble.Scan(scanTimeout, protocol.BLEDeviceName)
		if err != nil {
			return nil, fmt.Errorf("auto-scan failed: %w", err)
		}
		// De-duplicate by address, keeping strongest RSSI
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
				return unique[i].RSSI > unique[j].RSSI // strongest first
			}
			return unique[i].Address < unique[j].Address // stable tiebreaker
		})
		if len(unique) == 0 {
			return nil, fmt.Errorf("no %s device found. Make sure the mat is powered on and in range", protocol.BLEDeviceName)
		}
		if len(unique) == 1 {
			address = unique[0].Address
			fmt.Printf("Found device: %s (RSSI: %d)\n", address, unique[0].RSSI)
		} else {
			fmt.Println("Multiple devices found:")
			for i, r := range unique {
				fmt.Printf("  [%d] %s (RSSI: %d)\n", i+1, r.Address, r.RSSI)
			}
			fmt.Println("Please specify --address to select a device.")
			return nil, fmt.Errorf("multiple devices found, specify --address")
		}
	}

	// GID 해석: flag → env → config → error
	var gid [6]byte
	if deviceGid != "" {
		var err error
		gid, err = protocol.ParseDeviceGidHex(deviceGid)
		if err != nil {
			return nil, fmt.Errorf("invalid device-gid: %w", err)
		}
	} else if envGid := os.Getenv("HOTWATERMAT_DEVICE_GID"); envGid != "" {
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

func init() {
	rootCmd.PersistentFlags().StringVar(&address, "address", "", "BLE device address (or HOTWATERMAT_ADDRESS env)")
	rootCmd.PersistentFlags().StringVar(&deviceGid, "device-gid", "", "Device authentication key hex (or HOTWATERMAT_DEVICE_GID env)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug output")

	tempCmd.Flags().Float64Var(&leftTemp, "left", 0, "Left side target temperature (28.0-48.0)")
	tempCmd.Flags().Float64Var(&rightTemp, "right", 0, "Right side target temperature (28.0-48.0)")

	setupCmd.Flags().BoolVar(&resetSetup, "reset", false, "Clear cached device config")

	rootCmd.AddCommand(scanCmd, statusCmd, tempCmd, onCmd, offCmd, setupCmd)
}
