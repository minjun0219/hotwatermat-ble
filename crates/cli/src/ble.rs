//! 온수매트(KDO_HotWaterMat) 기기에 BLE로 연결하는 클라이언트.
//!
//! `btleplug`를 사용하여 실제 BLE 하드웨어와 통신한다(Linux: BlueZ, macOS: CoreBluetooth,
//! Windows: WinRT). 비동기(tokio) 기반이며, 상태/인증 알림은 별도 태스크에서 스트림으로 수신한다.
//!
//! 연결 및 인증 흐름:
//! 1. BLE 어댑터 확보 → 기기 스캔(주소 또는 이름 매칭)
//! 2. 연결 → GATT 서비스/특성 탐색
//! 3. CHAR1(상태)/CHAR2(인증) 구독 + 알림 수신 태스크 시작
//! 4. CHAR2로 핸드셰이크 전송
//! 5. CHAR2에서 B2F1 인증 완료(authType=0x02) 알림 수신 → 명령 전송 가능

use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use anyhow::{anyhow, bail, Context, Result};
use btleplug::api::{Central, Manager as _, Peripheral as _, ScanFilter, WriteType};
use btleplug::platform::{Adapter, Manager, Peripheral};
use futures::StreamExt;
use tokio::sync::Notify;
use uuid::Uuid;

/// BLE 스캔으로 발견된 기기 정보.
#[derive(Debug, Clone)]
pub struct ScanResult {
    /// BLE 기기 주소 (예: "AA:BB:CC:DD:EE:FF").
    pub address: String,
    /// 기기가 광고하는 이름.
    pub name: String,
    /// 수신 신호 강도. 0에 가까울수록 강함.
    pub rssi: i16,
}

/// BLE 어댑터 활성화/접근 실패 시 플랫폼별 안내 메시지.
#[cfg(target_os = "macos")]
const PERMISSION_HELP: &str = "Bluetooth permission denied or adapter unavailable.\n\
Please allow Bluetooth access for your terminal app:\n  \
System Settings → Privacy & Security → Bluetooth → Allow your terminal";
#[cfg(not(target_os = "macos"))]
const PERMISSION_HELP: &str = "Bluetooth adapter unavailable. Please check:\n  \
- Bluetooth is enabled on your system\n  \
- Required permissions are granted (e.g., running as root for BlueZ on Linux)";

/// 온수매트 프로토콜의 쓰기 타입을 반환한다.
///
/// Linux(BlueZ)는 v0.14 Go 구현과 동일하게 write-without-response를 사용하며(BlueZ가 특성
/// 속성에 따라 실제 쓰기 타입을 선택), 그 외 플랫폼은 write-with-response를 사용한다.
#[cfg(target_os = "linux")]
fn write_type() -> WriteType {
    WriteType::WithoutResponse
}
#[cfg(not(target_os = "linux"))]
fn write_type() -> WriteType {
    WriteType::WithResponse
}

/// 시스템의 첫 번째 BLE 어댑터를 반환한다.
async fn first_adapter() -> Result<Adapter> {
    let manager = Manager::new()
        .await
        .map_err(|e| anyhow!("enable BLE adapter: {e}\n\n{PERMISSION_HELP}"))?;
    let adapters = manager
        .adapters()
        .await
        .map_err(|e| anyhow!("enable BLE adapter: {e}\n\n{PERMISSION_HELP}"))?;
    adapters
        .into_iter()
        .next()
        .ok_or_else(|| anyhow!("no BLE adapter found\n\n{PERMISSION_HELP}"))
}

/// 주어진 시간 동안 BLE 기기를 검색한다.
///
/// `name_filter`가 비어 있지 않으면 해당 이름과 일치하는 기기만 반환한다.
/// 같은 기기가 중복으로 포함될 수 있으므로 중복 제거는 호출자가 처리한다.
pub async fn scan(timeout: Duration, name_filter: &str) -> Result<Vec<ScanResult>> {
    let adapter = first_adapter().await?;
    adapter
        .start_scan(ScanFilter::default())
        .await
        .context("scan")?;
    tokio::time::sleep(timeout).await;
    let _ = adapter.stop_scan().await;

    let peripherals = adapter.peripherals().await.context("list peripherals")?;
    let mut results = Vec::new();
    for p in peripherals {
        let Ok(Some(props)) = p.properties().await else {
            continue;
        };
        let name = props.local_name.unwrap_or_default();
        if !name_filter.is_empty() && name != name_filter {
            continue;
        }
        results.push(ScanResult {
            address: p.address().to_string(),
            name,
            rssi: props.rssi.unwrap_or(0),
        });
    }
    Ok(results)
}

/// 온수매트 BLE 기기와의 연결을 관리하는 클라이언트.
///
/// 사용 순서: [`Client::new`] → [`Client::connect`] → 제어 명령 → [`Client::disconnect`].
pub struct Client {
    address: String,
    device_gid: protocol::DeviceGid,
    debug: bool,
    peripheral: Option<Peripheral>,
    cmd_char: Option<btleplug::api::Characteristic>,
    /// 가장 최근에 수신한 STATUS. 알림 태스크가 갱신한다.
    last_status: Arc<Mutex<Option<protocol::Status>>>,
    /// 인증 완료(authType=0x02) 신호.
    auth_notify: Arc<Notify>,
    /// 페어링 완료(authType=0x01) 신호.
    pair_notify: Arc<Notify>,
    /// 페어링으로 취득한 DeviceGid.
    paired_gid: Arc<Mutex<Option<protocol::DeviceGid>>>,
    /// 알림 수신 태스크 핸들.
    notif_task: Option<tokio::task::JoinHandle<()>>,
}

impl Client {
    /// 새로운 BLE 클라이언트를 생성한다(연결은 수행하지 않음).
    pub fn new(address: impl Into<String>, device_gid: protocol::DeviceGid, debug: bool) -> Self {
        Client {
            address: address.into(),
            device_gid,
            debug,
            peripheral: None,
            cmd_char: None,
            last_status: Arc::new(Mutex::new(None)),
            auth_notify: Arc::new(Notify::new()),
            pair_notify: Arc::new(Notify::new()),
            paired_gid: Arc::new(Mutex::new(None)),
            notif_task: None,
        }
    }

    fn debug_log(&self, msg: &str) {
        if self.debug {
            println!("[DEBUG] {msg}");
        }
    }

    /// BLE 연결 + 서비스/특성 탐색 + 알림 구독까지 수행한다(핸드셰이크는 보내지 않음).
    async fn connect_ble(&mut self) -> Result<()> {
        let adapter = first_adapter().await?;
        let peripheral = self.find_peripheral(&adapter).await?;

        self.debug_log(&format!("Connecting to {}...", peripheral.address()));
        peripheral.connect().await.context("connect")?;

        self.debug_log("Discovering services...");
        peripheral
            .discover_services()
            .await
            .context("discover services")?;

        // CHAR1(상태)/CHAR2(명령+인증) 특성을 UUID로 구분
        let char1_uuid = parse_uuid(protocol::CHAR1_UUID);
        let char2_uuid = parse_uuid(protocol::CHAR2_UUID);
        let mut status_char = None;
        let mut cmd_char = None;
        for c in peripheral.characteristics() {
            if c.uuid == char1_uuid {
                status_char = Some(c);
            } else if c.uuid == char2_uuid {
                cmd_char = Some(c);
            }
        }
        let status_char = status_char.context("STATUS characteristic (CHAR1) not found")?;
        let cmd_char = cmd_char.context("command characteristic (CHAR2) not found")?;

        // CHAR1/CHAR2 구독 후 알림 수신 태스크 시작
        self.debug_log("Subscribing to notifications...");
        peripheral
            .subscribe(&status_char)
            .await
            .context("subscribe CHAR1")?;
        peripheral
            .subscribe(&cmd_char)
            .await
            .context("subscribe CHAR2")?;

        self.spawn_notification_task(&peripheral, char1_uuid, char2_uuid)
            .await?;

        self.cmd_char = Some(cmd_char);
        self.peripheral = Some(peripheral);

        // 연결 직후 바로 쓰기를 하면 실패할 수 있어 안정화를 위해 대기
        tokio::time::sleep(Duration::from_millis(300)).await;
        Ok(())
    }

    /// 주소(지정 시) 또는 기기 이름으로 대상 기기를 찾는다.
    ///
    /// 연결 해제 직후 매트가 광고를 재시작하기까지 시간이 걸릴 수 있어 최대 3회 재시도한다.
    async fn find_peripheral(&self, adapter: &Adapter) -> Result<Peripheral> {
        const MAX_RETRIES: u32 = 3;
        let has_address = !self.address.is_empty();

        for attempt in 1..=MAX_RETRIES {
            self.debug_log(&format!(
                "Scanning for device {}... (attempt {attempt}/{MAX_RETRIES})",
                self.address
            ));
            adapter
                .start_scan(ScanFilter::default())
                .await
                .context("scan")?;

            let deadline = Instant::now() + Duration::from_secs(10);
            loop {
                for p in adapter.peripherals().await.context("list peripherals")? {
                    if self.matches(&p, has_address).await {
                        let _ = adapter.stop_scan().await;
                        return Ok(p);
                    }
                }
                if Instant::now() >= deadline {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(500)).await;
            }
            let _ = adapter.stop_scan().await;

            if attempt < MAX_RETRIES {
                self.debug_log("Device not found, retrying in 2s...");
                tokio::time::sleep(Duration::from_secs(2)).await;
            }
        }

        bail!(
            "device not found after {MAX_RETRIES} attempts\n\n\
Possible causes:\n  \
- Official app is still connected (force close it first)\n  \
- Mat is powered off and not advertising (press any button on the remote)\n  \
- Device is out of BLE range"
        )
    }

    /// 주소가 지정되면 주소로, 아니면 기기 이름으로 일치 여부를 판단한다.
    async fn matches(&self, p: &Peripheral, has_address: bool) -> bool {
        if has_address {
            return p.address().to_string() == self.address;
        }
        match p.properties().await {
            Ok(Some(props)) => props.local_name.as_deref() == Some(protocol::BLE_DEVICE_NAME),
            _ => false,
        }
    }

    /// CHAR1(상태)/CHAR2(인증) 알림을 수신하여 상태 캐시 갱신 및 인증 신호를 보내는 태스크를 시작한다.
    async fn spawn_notification_task(
        &mut self,
        peripheral: &Peripheral,
        char1_uuid: Uuid,
        char2_uuid: Uuid,
    ) -> Result<()> {
        let mut stream = peripheral
            .notifications()
            .await
            .context("notification stream")?;
        let last_status = self.last_status.clone();
        let auth_notify = self.auth_notify.clone();
        let pair_notify = self.pair_notify.clone();
        let paired_gid = self.paired_gid.clone();
        let debug = self.debug;

        let task = tokio::spawn(async move {
            while let Some(notif) = stream.next().await {
                if notif.uuid == char1_uuid {
                    // STATUS 알림 — 파싱 성공 시 캐시 갱신
                    if let Ok(st) = protocol::parse_status(&notif.value) {
                        *last_status.lock().unwrap() = Some(st);
                    }
                } else if notif.uuid == char2_uuid {
                    // B2F1 인증 응답
                    match protocol::parse_auth_response(&notif.value) {
                        Ok(0x01) => {
                            // 페어링 응답 — DeviceGid 추출 (민감 정보이므로 로그 생략)
                            if notif.value.len() >= 9 {
                                let mut g = [0u8; 6];
                                g.copy_from_slice(&notif.value[3..9]);
                                *paired_gid.lock().unwrap() = Some(protocol::DeviceGid::new(g));
                                pair_notify.notify_one();
                            }
                        }
                        Ok(0x02) => {
                            if debug {
                                println!("[DEBUG] Authenticated!");
                            }
                            auth_notify.notify_one();
                        }
                        _ => {}
                    }
                }
            }
        });
        self.notif_task = Some(task);
        Ok(())
    }

    /// BLE 기기에 연결하고 기존 DeviceGid로 인증한다.
    pub async fn connect(&mut self) -> Result<()> {
        self.connect_ble().await?;

        let handshake = protocol::build_handshake_with_key(&self.device_gid);
        self.debug_log("Sending handshake (GID masked)");
        self.write_cmd(&handshake)
            .await
            .context("write handshake")?;

        // 인증 완료(authType=0x02) 대기 (최대 5초)
        tokio::time::timeout(Duration::from_secs(5), self.auth_notify.notified())
            .await
            .map_err(|_| anyhow!("authentication timeout (wrong DeviceGid?)"))?;
        self.debug_log("Authentication complete");
        Ok(())
    }

    /// 초기 페어링을 수행하여 기기의 DeviceGid를 획득한다.
    ///
    /// DeviceGid가 빈 값(모두 0)인 클라이언트에서 호출해야 한다.
    pub async fn pair(&mut self) -> Result<protocol::DeviceGid> {
        if self.device_gid != protocol::DeviceGid::new([0u8; 6]) {
            bail!("Pair must be called on a Client with an empty DeviceGid");
        }
        self.connect_ble().await?;

        let handshake = protocol::build_handshake();
        self.debug_log(&format!(
            "Sending pairing handshake: {}",
            protocol::format_packet(&handshake)
        ));
        self.write_cmd(&handshake)
            .await
            .context("write pairing handshake")?;

        // 페어링 응답(authType=0x01) 대기 (최대 10초)
        tokio::time::timeout(Duration::from_secs(10), self.pair_notify.notified())
            .await
            .map_err(|_| anyhow!("pairing timeout - is the mat in pairing mode?"))?;

        let gid = self
            .paired_gid
            .lock()
            .unwrap()
            .context("pairing signaled but no GID captured")?;
        self.device_gid = gid;
        self.debug_log("Pairing complete");
        Ok(gid)
    }

    /// BLE 연결을 해제한다. 연결 전이라도 안전하게 호출 가능하다.
    pub async fn disconnect(&mut self) -> Result<()> {
        if let Some(task) = self.notif_task.take() {
            task.abort();
        }
        if let Some(p) = self.peripheral.take() {
            let _ = p.disconnect().await;
        }
        Ok(())
    }

    /// 캐시된 STATUS 데이터를 초기화한다(verify 등에서 최신 상태를 강제로 다시 받을 때 사용).
    pub fn clear_status(&self) {
        *self.last_status.lock().unwrap() = None;
    }

    /// 매트의 최신 STATUS를 반환한다.
    ///
    /// 알림으로 수신된 캐시를 반환하며, 아직 없으면 `timeout`까지 100ms 간격으로 폴링한다.
    pub async fn get_status(&self, timeout: Duration) -> Result<protocol::Status> {
        let deadline = Instant::now() + timeout;
        loop {
            // 락 가드를 await 너머로 들고 가지 않도록 즉시 복제 후 해제
            let cached = self.last_status.lock().unwrap().clone();
            if let Some(st) = cached {
                return Ok(st);
            }
            if Instant::now() >= deadline {
                break;
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
        bail!("no status received within timeout")
    }

    /// 매트의 목표 온도를 설정한다.
    ///
    /// 현재 STATUS의 현재 온도(원시 바이트)를 읽어 HEAT 명령을 전송한다.
    pub async fn set_temp(&self, side: u8, left_temp: f64, right_temp: f64) -> Result<()> {
        let st = self
            .get_status(Duration::from_secs(3))
            .await
            .context("get current status")?;

        let left_tgt = protocol::encode_temp(left_temp).context("encode left temp")?;
        let right_tgt = protocol::encode_temp(right_temp).context("encode right temp")?;

        let pkt = protocol::build_heat(
            side,
            st.left_current_raw,
            st.right_current_raw,
            left_tgt,
            right_tgt,
        );
        self.debug_log(&format!("HEAT: {}", protocol::format_packet(&pkt)));
        self.write_cmd(&pkt)
            .await
            .context("write set_temp command")?;
        Ok(())
    }

    /// 매트의 전원을 켠다. STATUS 미수신 시 현재 온도를 0으로 폴백한다.
    pub async fn power_on(&self) -> Result<()> {
        let (left, right) = self.current_raw_or_zero().await;
        let pkt = protocol::build_power_on(left, right);
        self.debug_log(&format!("POWER ON: {}", protocol::format_packet(&pkt)));
        self.write_cmd(&pkt).await.context("write power_on command")
    }

    /// 매트의 전원을 끈다. STATUS 미수신 시 현재 온도를 0으로 폴백한다.
    pub async fn power_off(&self) -> Result<()> {
        let (left, right) = self.current_raw_or_zero().await;
        let pkt = protocol::build_power_off(left, right);
        self.debug_log(&format!("POWER OFF: {}", protocol::format_packet(&pkt)));
        self.write_cmd(&pkt)
            .await
            .context("write power_off command")
    }

    /// 현재 온도 원시 바이트(좌, 우)를 반환하되, STATUS가 없으면 (0, 0)을 반환한다.
    async fn current_raw_or_zero(&self) -> (u8, u8) {
        match self.get_status(Duration::from_secs(3)).await {
            Ok(st) => (st.left_current_raw, st.right_current_raw),
            Err(_) => (0, 0),
        }
    }

    /// CHAR2(명령 특성)에 데이터를 쓴다.
    async fn write_cmd(&self, data: &[u8]) -> Result<()> {
        let peripheral = self.peripheral.as_ref().context("not connected")?;
        let cmd_char = self
            .cmd_char
            .as_ref()
            .context("command characteristic unavailable")?;
        peripheral
            .write(cmd_char, data, write_type())
            .await
            .context("write characteristic")
    }
}

/// 프로토콜 상수의 UUID 문자열을 파싱한다. 상수는 항상 유효하므로 실패하면 패닉.
fn parse_uuid(s: &str) -> Uuid {
    Uuid::parse_str(s).expect("protocol UUID constant must be valid")
}
