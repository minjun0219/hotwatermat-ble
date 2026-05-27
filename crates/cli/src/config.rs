//! 기기 설정 파일 로드/저장.
//!
//! setup 명령 또는 자동 저장을 통해 생성되며, 플랫폼별 설정 디렉터리에 JSON으로 저장된다.
//! (Linux: `~/.config/hotwatermat-ble/`, macOS: `~/Library/Application Support/hotwatermat-ble/`)

use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

const APP_NAME: &str = "hotwatermat-ble";
const CONFIG_FILE: &str = "device.json";

/// 캐시된 기기 설정 정보.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DeviceConfig {
    /// BLE 기기 주소 (macOS에서는 UUID).
    pub address: String,
    /// 6바이트 인증 키 (12자리 16진수 문자열).
    pub device_gid: String,
    /// 설정이 저장된 시간 (Unix epoch 초).
    #[serde(default)]
    pub cached_at: i64,
}

/// 설정 디렉터리 경로를 반환한다.
fn config_dir() -> Result<PathBuf> {
    let base = dirs::config_dir().context("could not determine config directory")?;
    Ok(base.join(APP_NAME))
}

/// 설정 파일의 전체 경로를 반환한다.
pub fn config_path() -> Result<PathBuf> {
    Ok(config_dir()?.join(CONFIG_FILE))
}

/// 캐시된 기기 설정을 로드한다. 파일이 없으면 `Ok(None)`을 반환한다.
pub fn load() -> Result<Option<DeviceConfig>> {
    read_config(&config_path()?)
}

/// 기기 설정을 파일에 저장한다 (`cached_at`은 현재 시간으로 갱신).
pub fn save(cfg: &DeviceConfig) -> Result<()> {
    let dir = config_dir()?;
    std::fs::create_dir_all(&dir).context("create config dir")?;
    write_config(&dir.join(CONFIG_FILE), cfg)
}

/// 캐시된 설정 파일을 삭제한다. 파일이 없으면 성공으로 간주한다.
pub fn delete() -> Result<()> {
    let path = config_path()?;
    match std::fs::remove_file(&path) {
        Ok(()) => Ok(()),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(e).context("delete config"),
    }
}

/// 지정한 경로에서 설정을 읽는다. 파일이 없으면 `Ok(None)`.
fn read_config(path: &Path) -> Result<Option<DeviceConfig>> {
    match std::fs::read(path) {
        Ok(data) => {
            let cfg = serde_json::from_slice(&data).context("parse config")?;
            Ok(Some(cfg))
        }
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(e) => Err(e).context("read config"),
    }
}

/// 지정한 경로에 설정을 쓴다. 인증 키가 포함되므로 파일 권한을 0600으로 제한한다(Unix).
fn write_config(path: &Path, cfg: &DeviceConfig) -> Result<()> {
    let mut cfg = cfg.clone();
    cfg.cached_at = now_unix();
    let data = serde_json::to_vec_pretty(&cfg).context("marshal config")?;
    std::fs::write(path, &data).context("write config")?;

    // 인증 키 노출 방지를 위해 소유자만 읽기/쓰기 가능하도록 제한
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600))
            .context("set config permissions")?;
    }
    Ok(())
}

/// 현재 시간을 Unix epoch 초로 반환한다.
fn now_unix() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn write_then_read_roundtrip() {
        let dir = std::env::temp_dir().join(format!("hwm-cfg-test-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join("device.json");

        let cfg = DeviceConfig {
            address: "AA:BB:CC:DD:EE:FF".to_string(),
            device_gid: "13CE3CC53E5A".to_string(),
            cached_at: 0,
        };
        write_config(&path, &cfg).unwrap();

        let loaded = read_config(&path).unwrap().expect("config should exist");
        assert_eq!(loaded.address, cfg.address);
        assert_eq!(loaded.device_gid, cfg.device_gid);
        assert!(loaded.cached_at > 0, "cached_at은 저장 시 갱신되어야 함");

        std::fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn read_missing_file_returns_none() {
        let path = std::env::temp_dir().join("hwm-cfg-does-not-exist-xyz.json");
        std::fs::remove_file(&path).ok();
        assert!(read_config(&path).unwrap().is_none());
    }
}
