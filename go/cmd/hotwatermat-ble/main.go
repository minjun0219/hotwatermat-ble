// CLI for controlling a BLE hot water mat.
package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/minjun0219/hotwatermat-ble/pkg/ble"
	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
	"github.com/spf13/cobra"
)

const scanTimeout = 5 * time.Second

var (
	address   string
	deviceGid string
	debug     bool
	leftTemp  float64
	rightTemp float64
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

func connect() (*ble.Client, error) {
	if address == "" {
		address = os.Getenv("HOTWATERMAT_ADDRESS")
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
	} else {
		return nil, fmt.Errorf("no device GID specified. Set HOTWATERMAT_DEVICE_GID or use --device-gid flag")
	}

	client := ble.NewClient(address, gid, debug)
	if err := client.Connect(); err != nil {
		return nil, err
	}
	return client, nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&address, "address", "", "BLE device address (or HOTWATERMAT_ADDRESS env)")
	rootCmd.PersistentFlags().StringVar(&deviceGid, "device-gid", "", "Device authentication key hex (or HOTWATERMAT_DEVICE_GID env)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug output")

	tempCmd.Flags().Float64Var(&leftTemp, "left", 0, "Left side target temperature (28.0-48.0)")
	tempCmd.Flags().Float64Var(&rightTemp, "right", 0, "Right side target temperature (28.0-48.0)")

	rootCmd.AddCommand(scanCmd, statusCmd, tempCmd, onCmd, offCmd)
}
