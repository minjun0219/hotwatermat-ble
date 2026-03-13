// Package config는 기기 설정 파일 로드/저장을 처리합니다.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	appName    = "hotwatermat-ble"
	configFile = "device.json"
)

// DeviceConfig는 캐시된 기기 정보를 나타냅니다.
type DeviceConfig struct {
	Address   string    `json:"address"`
	DeviceGid string    `json:"device_gid"`
	CachedAt  time.Time `json:"cached_at"`
}

// configDir는 설정 디렉토리 경로를 반환합니다.
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config directory: %w", err)
	}
	return filepath.Join(base, appName), nil
}

// ConfigPath는 설정 파일의 전체 경로를 반환합니다.
func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFile), nil
}

// Load는 캐시된 기기 설정을 로드합니다.
// 파일이 없으면 nil, nil을 반환합니다.
func Load() (*DeviceConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg DeviceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Save는 기기 설정을 파일에 저장합니다.
func Save(cfg *DeviceConfig) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	cfg.CachedAt = time.Now()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	path := filepath.Join(dir, configFile)
	return os.WriteFile(path, data, 0644)
}

// Delete는 캐시된 설정 파일을 삭제합니다.
func Delete() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete config: %w", err)
	}
	return nil
}
