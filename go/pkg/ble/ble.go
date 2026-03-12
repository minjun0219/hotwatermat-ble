// Package ble provides a BLE client for connecting to the KDO_HotWaterMat device.
package ble

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

// ScanResult holds a discovered BLE device.
type ScanResult struct {
	Address string
	Name    string
	RSSI    int16
}

// Scan discovers BLE devices for the given duration.
// If nameFilter is non-empty, only devices matching that name are returned.
func Scan(timeout time.Duration, nameFilter string) ([]ScanResult, error) {
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("enable BLE adapter: %w", err)
	}

	var (
		mu      sync.Mutex
		results []ScanResult
	)

	err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		name := result.LocalName()
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
	if err != nil {
		// Scan will be stopped by the timeout goroutine
		_ = err
	}

	// Stop scanning after timeout
	go func() {
		time.Sleep(timeout)
		adapter.StopScan()
	}()

	time.Sleep(timeout + 100*time.Millisecond)
	return results, nil
}

// Client manages a BLE connection to the hot water mat.
type Client struct {
	address    string
	deviceGid  [6]byte
	device     bluetooth.Device
	statusChar bluetooth.DeviceCharacteristic
	cmdChar    bluetooth.DeviceCharacteristic
	lastStatus *protocol.Status
	mu         sync.Mutex
	connected  bool
	authDone   chan struct{}
	debug      bool
}

// NewClient creates a new BLE client.
func NewClient(address string, deviceGid [6]byte, debug bool) *Client {
	return &Client{
		address:   address,
		deviceGid: deviceGid,
		authDone:  make(chan struct{}),
		debug:     debug,
	}
}

func (c *Client) debugf(format string, args ...any) {
	if c.debug {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

// Connect establishes a BLE connection and authenticates.
func (c *Client) Connect() error {
	if err := adapter.Enable(); err != nil {
		return fmt.Errorf("enable BLE adapter: %w", err)
	}

	uuid, err := bluetooth.ParseUUID(protocol.ServiceUUID)
	if err != nil {
		return fmt.Errorf("parse service UUID: %w", err)
	}

	c.debugf("Scanning for device %s...", c.address)

	var targetAddr bluetooth.Address
	found := make(chan struct{})

	err = adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
		if result.Address.String() == c.address || result.LocalName() == "KDO_HotWaterMat" {
			targetAddr = result.Address
			a.StopScan()
			close(found)
		}
	})
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	select {
	case <-found:
	case <-time.After(10 * time.Second):
		adapter.StopScan()
		return errors.New("device not found within timeout")
	}

	c.debugf("Connecting to %s...", targetAddr.String())
	device, err := adapter.Connect(targetAddr, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	c.device = device

	c.debugf("Discovering services...")
	services, err := device.DiscoverServices([]bluetooth.UUID{uuid})
	if err != nil {
		return fmt.Errorf("discover services: %w", err)
	}
	if len(services) == 0 {
		return errors.New("service not found")
	}

	char1UUID, _ := bluetooth.ParseUUID(protocol.Char1UUID)
	char2UUID, _ := bluetooth.ParseUUID(protocol.Char2UUID)

	chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{char1UUID, char2UUID})
	if err != nil {
		return fmt.Errorf("discover characteristics: %w", err)
	}
	if len(chars) < 2 {
		return errors.New("characteristics not found")
	}

	// Assign characteristics (order may vary)
	for i := range chars {
		charUUID := chars[i].UUID().String()
		switch charUUID {
		case protocol.Char1UUID:
			c.statusChar = chars[i]
		case protocol.Char2UUID:
			c.cmdChar = chars[i]
		}
	}

	// Subscribe to STATUS notifications on CHAR1
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

	// Subscribe to auth notifications on CHAR2
	c.debugf("Subscribing to AUTH notifications...")
	err = c.cmdChar.EnableNotifications(func(buf []byte) {
		c.debugf("AUTH: %s", protocol.FormatPacket(buf))
		authType, err := protocol.ParseAuthResponse(buf)
		if err == nil && authType == 0x02 {
			c.debugf("Authenticated!")
			select {
			case <-c.authDone:
			default:
				close(c.authDone)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe CHAR2: %w", err)
	}

	// Wait before handshake (per connection sequence)
	time.Sleep(300 * time.Millisecond)

	// Send handshake
	handshake := protocol.BuildHandshakeWithKey(c.deviceGid)
	c.debugf("Sending handshake: %s", protocol.FormatPacket(handshake[:]))
	_, err = c.cmdChar.Write(handshake[:])
	if err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}

	// Wait for authentication
	select {
	case <-c.authDone:
		c.debugf("Authentication complete")
	case <-time.After(5 * time.Second):
		return errors.New("authentication timeout (wrong DeviceGid?)")
	}

	c.connected = true
	return nil
}

// Disconnect closes the BLE connection.
func (c *Client) Disconnect() error {
	if !c.connected {
		return nil
	}
	c.connected = false
	return c.device.Disconnect()
}

// GetStatus returns the latest STATUS from the mat.
// Waits up to timeout for a status update.
func (c *Client) GetStatus(timeout time.Duration) (*protocol.Status, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		st := c.lastStatus
		c.mu.Unlock()
		if st != nil {
			return st, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("no status received within timeout")
}

// SetTemp sends a temperature change command.
func (c *Client) SetTemp(side byte, leftTemp, rightTemp float64) error {
	st, err := c.GetStatus(3 * time.Second)
	if err != nil {
		return fmt.Errorf("get current status: %w", err)
	}

	leftTgt, err := protocol.EncodeTemp(leftTemp)
	if err != nil {
		return fmt.Errorf("encode left temp: %w", err)
	}
	rightTgt, err := protocol.EncodeTemp(rightTemp)
	if err != nil {
		return fmt.Errorf("encode right temp: %w", err)
	}

	pkt := protocol.BuildHeat(side, st.LeftCurrentRaw, st.RightCurrentRaw, leftTgt, rightTgt)
	c.debugf("HEAT: %s", protocol.FormatPacket(pkt[:]))
	_, err = c.cmdChar.Write(pkt[:])
	return err
}

// PowerOn sends a power-on command.
func (c *Client) PowerOn() error {
	st, err := c.GetStatus(3 * time.Second)
	if err != nil {
		// If no status yet, use zeros
		pkt := protocol.BuildPowerOn(0, 0)
		c.debugf("POWER ON: %s", protocol.FormatPacket(pkt[:]))
		_, err = c.cmdChar.Write(pkt[:])
		return err
	}

	pkt := protocol.BuildPowerOn(st.LeftCurrentRaw, st.RightCurrentRaw)
	c.debugf("POWER ON: %s", protocol.FormatPacket(pkt[:]))
	_, err = c.cmdChar.Write(pkt[:])
	return err
}

// PowerOff sends a power-off command.
func (c *Client) PowerOff() error {
	st, err := c.GetStatus(3 * time.Second)
	if err != nil {
		pkt := protocol.BuildPowerOff(0, 0)
		c.debugf("POWER OFF: %s", protocol.FormatPacket(pkt[:]))
		_, err = c.cmdChar.Write(pkt[:])
		return err
	}

	pkt := protocol.BuildPowerOff(st.LeftCurrentRaw, st.RightCurrentRaw)
	c.debugf("POWER OFF: %s", protocol.FormatPacket(pkt[:]))
	_, err = c.cmdChar.Write(pkt[:])
	return err
}
