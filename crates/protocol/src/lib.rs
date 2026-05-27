//! 온수매트(KDO_HotWaterMat) BLE 패킷 생성 및 파싱.
//!
//! 매트와 앱 사이의 BLE 통신은 항상 20바이트 고정 길이 패킷으로 이루어진다.
//! 이 크레이트는 BLE 라이브러리에 의존하지 않으며, 순수하게 패킷의 인코딩/디코딩만 담당한다.
//!
//! 패킷 구조 (20바이트):
//!
//! ```text
//! [0]     STX (0xB2) - 시작 바이트
//! [1]     방향/타입 - 0x80: 앱→매트, 0x00: 매트→앱, 0x01: 핸드셰이크, 0xF1: 인증 응답
//! [2]     모드 - 0x01: 난방, 0x02: 타이머꺼짐, 0x03: 취침, 0x06: 전원, 0x07: 급속난방, 0x08: 이온케어
//! [3]     좌/우 선택 - 0x02: 왼쪽, 0x04: 오른쪽, 0x06: 양쪽
//! [4]     볼륨(상위4비트) + 수위(하위4비트)
//! [5-6]   모드별 고정값
//! [7]     왼쪽 현재 온도 (인코딩된 바이트)
//! [8]     오른쪽 현재 온도 (인코딩된 바이트)
//! [9]     모드별 고정값
//! [10]    왼쪽 목표 온도 (인코딩된 바이트)
//! [11]    오른쪽 목표 온도 (인코딩된 바이트)
//! [12-18] 패딩 또는 추가 데이터
//! [19]    체크섬 - bytes[1..19]의 합을 0xFF로 AND 연산한 값
//! ```

use std::fmt;

/// BLE 패킷의 고정 크기. 모든 패킷은 반드시 20바이트여야 한다.
pub const PACKET_SIZE: usize = 20;

/// 모든 패킷의 시작 바이트 (Start of TeXt). 첫 바이트가 0xB2가 아니면 유효하지 않은 패킷이다.
pub const STX: u8 = 0xB2;

/// 앱 → 매트 명령 패킷의 방향 바이트 (byte[1] = 0x80). 온도 설정, 전원 ON/OFF 등에 사용.
pub const DIR_APP_TO_MAT: u8 = 0x80;

/// 매트 → 앱 상태 알림 패킷의 방향 바이트 (byte[1] = 0x00). CHAR1(0100dd) STATUS 알림.
pub const DIR_MAT_TO_APP: u8 = 0x00;

/// 핸드셰이크(연결 초기화) 패킷의 방향 바이트 (byte[1] = 0x01).
pub const DIR_HANDSHAKE: u8 = 0x01;

/// 매트가 보내는 인증 응답 패킷의 방향 바이트 (byte[1] = 0xF1). CHAR2(0200dd) B2F1 응답.
pub const DIR_AUTH: u8 = 0xF1;

/// 온수매트 BLE 서비스 UUID.
pub const SERVICE_UUID: &str = "00001c0d-d102-11e1-9b23-2ce2a80000dd";

/// 상태 알림 수신용 BLE 특성 UUID (0100dd). 매트 → 앱. 쓰기 금지.
pub const CHAR1_UUID: &str = "00001c0d-d102-11e1-9b23-2ce2a80100dd";

/// 명령 쓰기 + 인증 응답용 BLE 특성 UUID (0200dd). 앱 → 매트 + B2F1 응답. write-with-response 사용.
pub const CHAR2_UUID: &str = "00001c0d-d102-11e1-9b23-2ce2a80200dd";

/// 미사용 BLE 특성 UUID (0400dd).
pub const CHAR3_UUID: &str = "00001c0d-d102-11e1-9b23-2ce2a80400dd";

/// 온수매트가 BLE 광고 시 사용하는 기기 이름.
pub const BLE_DEVICE_NAME: &str = "KDO_HotWaterMat";

// --- 모드 값 (byte[2]) ---

/// 난방 모드 (0x01). 목표 온도를 설정하여 매트를 가열한다.
pub const MODE_HEAT: u8 = 0x01;
/// 타이머 꺼짐 모드 (0x02).
pub const MODE_TIMER_OFF: u8 = 0x02;
/// 취침 모드 (0x03).
pub const MODE_SLEEP: u8 = 0x03;
/// 전원 제어 모드 (0x06). byte[6]이 0x2B이면 ON, 0xAB이면 OFF.
pub const MODE_POWER: u8 = 0x06;
/// 급속 난방 모드 (0x07).
pub const MODE_FASTHEAT: u8 = 0x07;
/// 이온케어 모드 (0x08).
pub const MODE_IONCARE: u8 = 0x08;

// --- 좌/우 선택 값 (byte[3]) ---

/// 왼쪽 매트만 선택 (0x02).
pub const SIDE_LEFT: u8 = 0x02;
/// 오른쪽 매트만 선택 (0x04).
pub const SIDE_RIGHT: u8 = 0x04;
/// 양쪽 매트 모두 선택 (0x06 = 0x02 | 0x04).
pub const SIDE_BOTH: u8 = 0x06;

// --- 온도 범위 ---

/// 설정 가능한 최저 온도 (28.0°C).
pub const TEMP_MIN: f64 = 28.0;
/// 설정 가능한 최고 온도 (48.0°C).
pub const TEMP_MAX: f64 = 48.0;

/// 패딩 바이트 (0xFE). 사용하지 않는 바이트를 채운다.
pub const PAD: u8 = 0xFE;

/// 프로토콜 인코딩/파싱 과정에서 발생하는 오류.
#[derive(Debug, Clone, PartialEq)]
pub enum ProtocolError {
    /// 온도가 허용 범위 [28.0, 48.0]를 벗어남.
    TempOutOfRange(f64),
    /// 온도가 0.5°C 단위가 아님.
    TempNotHalfDegree(f64),
    /// 패킷 길이가 20바이트 미만.
    PacketTooShort,
    /// 첫 바이트가 STX(0xB2)가 아님.
    InvalidStx(u8),
    /// STATUS 패킷이 아님 (방향 바이트 불일치).
    NotStatusPacket(u8),
    /// 인증 응답 패킷이 아님.
    NotAuthResponse,
    /// 체크섬 불일치.
    ChecksumMismatch { got: u8, expected: u8 },
    /// 페어링 응답이 아님 (인증 타입 불일치).
    NotPairingResponse(u8),
    /// 유효하지 않은 16진수 문자열.
    InvalidHex(String),
    /// DeviceGid 길이가 6바이트가 아님.
    InvalidGidLength(usize),
}

impl fmt::Display for ProtocolError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ProtocolError::TempOutOfRange(t) => {
                write!(
                    f,
                    "temperature {t:.1} out of range [{TEMP_MIN:.1}, {TEMP_MAX:.1}]"
                )
            }
            ProtocolError::TempNotHalfDegree(t) => {
                write!(f, "temperature {t:.1} must be in 0.5°C increments")
            }
            ProtocolError::PacketTooShort => write!(f, "packet too short"),
            ProtocolError::InvalidStx(b) => write!(f, "invalid STX: 0x{b:02X}"),
            ProtocolError::NotStatusPacket(d) => {
                write!(f, "not a STATUS packet (direction=0x{d:02X})")
            }
            ProtocolError::NotAuthResponse => write!(f, "not an auth response"),
            ProtocolError::ChecksumMismatch { got, expected } => {
                write!(
                    f,
                    "checksum mismatch: got 0x{got:02X}, expected 0x{expected:02X}"
                )
            }
            ProtocolError::NotPairingResponse(t) => {
                write!(f, "not a pairing response (type=0x{t:02X})")
            }
            ProtocolError::InvalidHex(s) => write!(f, "invalid hex: {s}"),
            ProtocolError::InvalidGidLength(n) => {
                write!(f, "DeviceGid must be 6 bytes, got {n}")
            }
        }
    }
}

impl std::error::Error for ProtocolError {}

/// 6바이트 기기 인증 키.
///
/// 최초 페어링 시 매트에서 발급받은 키로, 이후 핸드셰이크 인증에 사용된다.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct DeviceGid([u8; 6]);

impl DeviceGid {
    /// 6바이트 배열로부터 DeviceGid를 생성한다.
    pub fn new(bytes: [u8; 6]) -> Self {
        DeviceGid(bytes)
    }

    /// 내부 6바이트 배열에 대한 참조를 반환한다.
    pub fn as_bytes(&self) -> &[u8; 6] {
        &self.0
    }

    /// 16진수 문자열(예: "13CE3CC53E5A")을 6바이트 DeviceGid로 파싱한다.
    ///
    /// 정확히 12자리 16진수 문자열(= 6바이트)이어야 한다.
    pub fn from_hex(s: &str) -> Result<Self, ProtocolError> {
        let bytes = decode_hex(s)?;
        if bytes.len() != 6 {
            return Err(ProtocolError::InvalidGidLength(bytes.len()));
        }
        let mut arr = [0u8; 6];
        arr.copy_from_slice(&bytes);
        Ok(DeviceGid(arr))
    }
}

/// 대문자 hex 문자열로 포맷한다 (예: "13CE3CC53E5A").
impl fmt::Display for DeviceGid {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        for b in &self.0 {
            write!(f, "{b:02X}")?;
        }
        Ok(())
    }
}

/// 모드 바이트 값을 읽기 쉬운 이름으로 변환한다 (예: 0x01 → "HEAT").
/// 알 수 없는 모드는 "UNKNOWN(0xXX)" 형식이다.
pub fn mode_name(mode: u8) -> String {
    match mode {
        MODE_HEAT => "HEAT".to_string(),
        MODE_TIMER_OFF => "TIMER_OFF".to_string(),
        MODE_SLEEP => "SLEEP".to_string(),
        MODE_POWER => "POWER".to_string(),
        MODE_FASTHEAT => "FASTHEAT".to_string(),
        MODE_IONCARE => "IONCARE".to_string(),
        _ => format!("UNKNOWN(0x{mode:02X})"),
    }
}

/// 좌/우 선택 바이트를 읽기 쉬운 이름으로 변환한다 (예: 0x02 → "left").
pub fn side_name(side: u8) -> String {
    match side {
        SIDE_LEFT => "left".to_string(),
        SIDE_RIGHT => "right".to_string(),
        SIDE_BOTH => "both".to_string(),
        _ => format!("unknown(0x{side:02X})"),
    }
}

/// 수위 값을 읽기 쉬운 이름으로 변환한다 (1=LOW, 2=OK, 3=FULL).
pub fn water_level_name(wlv: i32) -> String {
    match wlv {
        1 => "LOW".to_string(),
        2 => "OK".to_string(),
        3 => "FULL".to_string(),
        _ => format!("UNKNOWN({wlv})"),
    }
}

/// 실제 온도값을 BLE 패킷에 넣을 바이트로 인코딩한다.
///
/// - 정수 온도(예: 33.0°C): 그대로 바이트로 변환 → 33 (0x21)
/// - 소수점 온도(예: 33.5°C): `temp + 127.5` = 161 (0xA1)
/// - 0.5°C 단위만 허용, 범위 28.0°C ~ 48.0°C
///
/// 주의: bit7(최상위 비트)은 난방 플래그가 아니라 0.5도 소수점을 구분하기 위한 인코딩이다.
pub fn encode_temp(temp: f64) -> Result<u8, ProtocolError> {
    if !(TEMP_MIN..=TEMP_MAX).contains(&temp) {
        return Err(ProtocolError::TempOutOfRange(temp));
    }
    // 0.5도 단위 검증: 온도를 2배 했을 때 정수가 되어야 함
    let doubled = temp * 2.0;
    if doubled != doubled.trunc() {
        return Err(ProtocolError::TempNotHalfDegree(temp));
    }

    let int_part = temp.trunc();
    if temp != int_part {
        // 소수점(.5°C) — int(temp + 127.5)로 인코딩 (예: 33.5 + 127.5 = 161 = 0xA1)
        Ok((temp + 127.5) as u8)
    } else {
        // 정수 온도는 그대로 바이트로 변환
        Ok(int_part as u8)
    }
}

/// BLE 패킷의 온도 바이트를 실제 온도값으로 디코딩한다.
///
/// - 바이트 값 > 127: 소수점 온도 → `byte - 127.5` (예: 161 → 33.5°C)
/// - 바이트 값 ≤ 127: 정수 온도 → 그대로 (예: 33 → 33.0°C)
///
/// `encode_temp`의 역함수다.
pub fn decode_temp(b: u8) -> f64 {
    if b > 127 {
        f64::from(b) - 127.5
    } else {
        f64::from(b)
    }
}

/// 20바이트 패킷의 체크섬을 계산한다.
///
/// bytes[1]부터 bytes[18]까지(총 18바이트)를 모두 더한 후 하위 8비트만 취한다(& 0xFF).
/// 결과는 bytes[19]에 저장된다.
pub fn calc_checksum(pkt: &[u8]) -> u8 {
    if pkt.len() < PACKET_SIZE {
        return 0;
    }
    let mut total: u16 = 0;
    for &b in &pkt[1..19] {
        total += u16::from(b);
    }
    // 16비트 합의 하위 바이트만 반환
    (total & 0xFF) as u8
}

/// 최초 페어링용 핸드셰이크 패킷을 생성한다.
///
/// DeviceGid가 없는 상태에서 처음 기기와 연결할 때 사용한다.
/// 매트는 이 패킷을 받으면 B2F1 페어링 응답으로 DeviceGid를 돌려준다.
pub fn build_handshake() -> [u8; PACKET_SIZE] {
    let mut pkt = [0u8; PACKET_SIZE];
    pkt[0] = STX;
    pkt[1] = DIR_HANDSHAKE;
    for b in &mut pkt[2..19] {
        *b = PAD;
    }
    pkt[19] = calc_checksum(&pkt);
    pkt
}

/// DeviceGid를 사용한 인증 핸드셰이크 패킷을 생성한다.
///
/// 이미 페어링된 기기에 다시 연결할 때 사용한다.
/// DeviceGid는 byte[2]~byte[7]에 들어간다.
pub fn build_handshake_with_key(gid: &DeviceGid) -> [u8; PACKET_SIZE] {
    let mut pkt = [0u8; PACKET_SIZE];
    pkt[0] = STX;
    pkt[1] = DIR_HANDSHAKE;
    pkt[2..8].copy_from_slice(gid.as_bytes());
    for b in &mut pkt[8..19] {
        *b = PAD;
    }
    pkt[19] = calc_checksum(&pkt);
    pkt
}

/// 난방(HEAT) 명령 패킷을 생성한다.
///
/// 매트의 목표 온도를 설정하는 핵심 명령이다. 현재 STATUS에서 받은 현재 온도 바이트(raw)를
/// 함께 전달해야 한다.
///
/// - `side`: 좌/우 선택 (SIDE_LEFT/SIDE_RIGHT/SIDE_BOTH)
/// - `left_cur`/`right_cur`: 좌/우 현재 온도 원시 바이트 (STATUS의 byte[7]/byte[8])
/// - `left_target`/`right_target`: 좌/우 목표 온도 (`encode_temp`로 인코딩한 값)
pub fn build_heat(
    side: u8,
    left_cur: u8,
    right_cur: u8,
    left_target: u8,
    right_target: u8,
) -> [u8; PACKET_SIZE] {
    let mut pkt = [0u8; PACKET_SIZE];
    pkt[0] = STX;
    pkt[1] = DIR_APP_TO_MAT; // 앱 → 매트
    pkt[2] = MODE_HEAT; // 난방 모드 (0x01)
    pkt[3] = side; // 좌/우 선택
    pkt[4] = 0x00; // 볼륨/수위 (난방 명령에서는 0)
    pkt[5] = 0x02; // HEAT 모드 고정값
    pkt[6] = 0x25; // HEAT 모드 고정값
    pkt[7] = left_cur; // 왼쪽 현재 온도
    pkt[8] = right_cur; // 오른쪽 현재 온도
    pkt[9] = 0x23; // HEAT 모드 고정값
    pkt[10] = left_target; // 왼쪽 목표 온도
    pkt[11] = right_target; // 오른쪽 목표 온도
    for b in &mut pkt[12..19] {
        *b = PAD;
    }
    pkt[19] = calc_checksum(&pkt);
    pkt
}

/// 전원 켜기(Power ON) 명령 패킷을 생성한다.
///
/// 가능하면 최신 STATUS에서 받은 현재 온도를 전달하되, 없으면 0을 넣어도 동작한다.
/// byte[6]의 0x2B가 ON 마커다.
pub fn build_power_on(left_cur: u8, right_cur: u8) -> [u8; PACKET_SIZE] {
    let mut pkt = [0u8; PACKET_SIZE];
    pkt[0] = STX;
    pkt[1] = DIR_APP_TO_MAT; // 앱 → 매트
    pkt[2] = MODE_POWER; // 전원 제어 모드 (0x06)
    pkt[3] = SIDE_BOTH; // 전원은 항상 양쪽
    pkt[4] = 0x00;
    pkt[5] = PAD;
    pkt[6] = 0x2B; // ON 마커
    pkt[7] = left_cur; // 왼쪽 현재 온도
    pkt[8] = right_cur; // 오른쪽 현재 온도
    pkt[9] = PAD;
    // byte[10]~byte[15]는 0 (배열 초기값)
    for b in &mut pkt[16..19] {
        *b = PAD;
    }
    pkt[19] = calc_checksum(&pkt);
    pkt
}

/// 전원 끄기(Power OFF) 명령 패킷을 생성한다.
///
/// byte[6]의 0xAB가 OFF 마커다. PROTOCOL.md 사양에 따라 오른쪽 현재 온도가
/// byte[11]과 byte[13]에도 들어간다.
pub fn build_power_off(left_cur: u8, right_cur: u8) -> [u8; PACKET_SIZE] {
    let mut pkt = [0u8; PACKET_SIZE];
    pkt[0] = STX;
    pkt[1] = DIR_APP_TO_MAT; // 앱 → 매트
    pkt[2] = MODE_POWER; // 전원 제어 모드 (0x06)
    pkt[3] = SIDE_RIGHT; // 0x04 — PROTOCOL.md 사양
    pkt[4] = 0x00;
    pkt[5] = PAD;
    pkt[6] = 0xAB; // OFF 마커
    pkt[7] = left_cur; // 왼쪽 현재 온도
    pkt[8] = right_cur; // 오른쪽 현재 온도
    pkt[9] = PAD;
    pkt[10] = 0x00;
    pkt[11] = right_cur; // 오른쪽 현재 온도 (반복)
    pkt[12] = PAD;
    pkt[13] = right_cur; // 오른쪽 현재 온도 (반복)
    for b in &mut pkt[14..19] {
        *b = PAD;
    }
    pkt[19] = calc_checksum(&pkt);
    pkt
}

/// 매트에서 수신한 STATUS 알림 패킷을 파싱한 결과.
///
/// CHAR1(0100dd) 특성의 알림을 통해 주기적으로 수신된다.
#[derive(Debug, Clone)]
pub struct Status {
    /// 현재 동작 모드 바이트 값 (예: 0x01=HEAT, 0x06=POWER).
    pub mode: u8,
    /// 모드의 사람이 읽을 수 있는 이름 (예: "HEAT").
    pub mode_name: String,
    /// 현재 활성화된 좌/우 선택값 (0x02/0x04/0x06).
    pub side: u8,
    /// 볼륨 값 (byte[4]의 상위 4비트).
    pub volume: i32,
    /// 수위 값 (byte[4]의 하위 4비트). 1=LOW, 2=OK, 3=FULL.
    pub water_level: i32,
    /// byte[5]의 원시값 (모드별로 의미가 다름).
    pub field5: u8,
    /// byte[6]의 원시값 (모드별로 의미가 다름).
    pub field6: u8,
    /// 왼쪽 현재 온도의 인코딩된 원시 바이트 (명령 생성 시 그대로 전달).
    pub left_current_raw: u8,
    /// 오른쪽 현재 온도의 인코딩된 원시 바이트 (명령 생성 시 그대로 전달).
    pub right_current_raw: u8,
    /// 왼쪽 현재 온도 (디코딩된 실제 °C).
    pub left_current: f64,
    /// 오른쪽 현재 온도 (디코딩된 실제 °C).
    pub right_current: f64,
    /// 왼쪽 목표 온도 (디코딩된 실제 °C).
    pub left_target: f64,
    /// 오른쪽 목표 온도 (디코딩된 실제 °C).
    pub right_target: f64,
    /// byte[12]의 하위 명령 코드.
    pub sub_cmd: u8,
    /// 원본 20바이트 패킷 데이터 (디버깅용).
    pub raw: [u8; PACKET_SIZE],
    /// 전원이 꺼져 있는지 여부. POWER 모드이고 목표 온도가 양쪽 모두 0이면 true.
    pub powered_off: bool,
}

/// 매트에서 수신한 20바이트 상태 알림 패킷을 파싱한다.
///
/// 길이/STX/방향/체크섬을 검증한 후 [`Status`]로 변환한다.
pub fn parse_status(data: &[u8]) -> Result<Status, ProtocolError> {
    if data.len() < PACKET_SIZE {
        return Err(ProtocolError::PacketTooShort);
    }
    if data[0] != STX {
        return Err(ProtocolError::InvalidStx(data[0]));
    }
    if data[1] != DIR_MAT_TO_APP {
        return Err(ProtocolError::NotStatusPacket(data[1]));
    }
    // 체크섬 검증 — 손상된 패킷 수락 방지
    let expected = calc_checksum(data);
    if data[19] != expected {
        return Err(ProtocolError::ChecksumMismatch {
            got: data[19],
            expected,
        });
    }

    let mode = data[2];

    // byte[4]에서 볼륨(상위 4비트)과 수위(하위 4비트) 추출
    let volume = i32::from((data[4] >> 4) & 0x0F);
    let water_level = i32::from(data[4] & 0x0F);

    let left_current_raw = data[7];
    let right_current_raw = data[8];

    // 전원 꺼짐 판단: POWER 모드이고 목표 온도가 양쪽 모두 0이면 OFF
    let powered_off = mode == MODE_POWER && data[10] == 0 && data[11] == 0;

    let mut raw = [0u8; PACKET_SIZE];
    raw.copy_from_slice(&data[..PACKET_SIZE]);

    Ok(Status {
        mode,
        mode_name: mode_name(mode),
        side: data[3],
        volume,
        water_level,
        field5: data[5],
        field6: data[6],
        left_current_raw,
        right_current_raw,
        left_current: decode_temp(left_current_raw),
        right_current: decode_temp(right_current_raw),
        left_target: decode_temp(data[10]),
        right_target: decode_temp(data[11]),
        sub_cmd: data[12],
        raw,
        powered_off,
    })
}

/// 수신된 패킷이 B2F1 인증 응답인지 확인하고 인증 타입 바이트를 반환한다.
///
/// 반환값:
/// - 0x01: 페어링 응답 — DeviceGid 포함 ([`parse_device_gid`]로 추출)
/// - 0x02: 인증 완료 — 이후 명령 전송 가능
pub fn parse_auth_response(data: &[u8]) -> Result<u8, ProtocolError> {
    if data.len() < PACKET_SIZE {
        return Err(ProtocolError::PacketTooShort);
    }
    if data[0] != STX {
        return Err(ProtocolError::InvalidStx(data[0]));
    }
    if data[1] != DIR_AUTH {
        return Err(ProtocolError::NotAuthResponse);
    }
    // 체크섬 검증 — 손상된 패킷 수락 방지
    let expected = calc_checksum(data);
    if data[19] != expected {
        return Err(ProtocolError::ChecksumMismatch {
            got: data[19],
            expected,
        });
    }
    // byte[2]가 인증 타입
    Ok(data[2])
}

/// 페어링 응답 패킷에서 6바이트 DeviceGid를 추출한다.
///
/// 최초 페어링 후 매트가 보내는 B2F1 응답(authType=0x01)의 byte[3]~byte[8]에서 추출한다.
pub fn parse_device_gid(data: &[u8]) -> Result<DeviceGid, ProtocolError> {
    let auth_type = parse_auth_response(data)?;
    if auth_type != 0x01 {
        return Err(ProtocolError::NotPairingResponse(auth_type));
    }
    let mut bytes = [0u8; 6];
    bytes.copy_from_slice(&data[3..9]);
    Ok(DeviceGid(bytes))
}

/// 패킷을 16진수 문자열로 포맷한다 (디버깅용). 예: `[0xB2, 0x01, 0xFE]` → "B2 01 FE".
pub fn format_packet(pkt: &[u8]) -> String {
    pkt.iter()
        .map(|b| format!("{b:02X}"))
        .collect::<Vec<_>>()
        .join(" ")
}

/// 16진수 문자열을 바이트 벡터로 디코딩한다.
///
/// 길이가 홀수이거나 16진수가 아닌 문자가 있으면 [`ProtocolError::InvalidHex`]를 반환한다.
fn decode_hex(s: &str) -> Result<Vec<u8>, ProtocolError> {
    if !s.len().is_multiple_of(2) {
        return Err(ProtocolError::InvalidHex(s.to_string()));
    }
    let bytes = s.as_bytes();
    let mut out = Vec::with_capacity(s.len() / 2);
    for chunk in bytes.chunks(2) {
        let hi = hex_val(chunk[0]).ok_or_else(|| ProtocolError::InvalidHex(s.to_string()))?;
        let lo = hex_val(chunk[1]).ok_or_else(|| ProtocolError::InvalidHex(s.to_string()))?;
        out.push((hi << 4) | lo);
    }
    Ok(out)
}

/// 16진수 한 글자를 0~15 값으로 변환한다. 유효하지 않으면 None.
fn hex_val(c: u8) -> Option<u8> {
    match c {
        b'0'..=b'9' => Some(c - b'0'),
        b'a'..=b'f' => Some(c - b'a' + 10),
        b'A'..=b'F' => Some(c - b'A' + 10),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // 테스트 전용 DeviceGid (실제 기기의 GID 아님).
    const TEST_DEVICE_GID: [u8; 6] = [0x13, 0xCE, 0x3C, 0xC5, 0x3E, 0x5A];

    #[test]
    fn encode_temp_cases() {
        let cases: &[(f64, u8)] = &[
            (28.0, 28),
            (28.5, 156), // 0x9C = 28.5 + 127.5
            (33.0, 33),
            (33.5, 161), // 0xA1 = 33.5 + 127.5
            (34.0, 34),
            (34.5, 162), // 0xA2 = 34.5 + 127.5
            (48.0, 48),
        ];
        for &(temp, want) in cases {
            assert_eq!(encode_temp(temp).unwrap(), want, "encode_temp({temp})");
        }

        // 오류 케이스
        assert!(encode_temp(27.0).is_err(), "최저 미만");
        assert!(encode_temp(49.0).is_err(), "최고 초과");
        assert!(encode_temp(33.3).is_err(), "0.5 단위 아님");
    }

    #[test]
    fn decode_temp_cases() {
        let cases: &[(u8, f64)] = &[
            (28, 28.0),
            (156, 28.5),
            (33, 33.0),
            (161, 33.5),
            (34, 34.0),
            (162, 34.5),
            (48, 48.0),
        ];
        for &(b, want) in cases {
            assert_eq!(decode_temp(b), want, "decode_temp({b})");
        }
    }

    #[test]
    fn encode_decode_roundtrip() {
        // 28.0°C ~ 48.0°C 0.5°C 단위 전 구간 왕복 검증
        for i in 0..=40 {
            let temp = 28.0 + f64::from(i) * 0.5;
            let encoded = encode_temp(temp).unwrap();
            let decoded = decode_temp(encoded);
            assert_eq!(
                decoded, temp,
                "왕복 변환 실패: {temp} → {encoded} → {decoded}"
            );
        }
    }

    #[test]
    fn calc_checksum_known_packets() {
        // 예시 1: 왼쪽 33°C→34°C 난방 명령
        let pkt1 = [
            0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21, 0x1C, 0x23, 0x22, 0x1C, 0xFE, 0xFE,
            0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0x3A,
        ];
        assert_eq!(calc_checksum(&pkt1), 0x3A);

        // 예시 2: 왼쪽 33.5°C 설정 (소수점 온도)
        let pkt2 = [
            0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21, 0x1C, 0x23, 0xA1, 0x1C, 0xFE, 0xFE,
            0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xB9,
        ];
        assert_eq!(calc_checksum(&pkt2), 0xB9);

        // 예시 3: 최초 페어링 핸드셰이크 (DeviceGid 없음)
        let pkt3 = [
            0xB2, 0x01, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE,
            0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xDF,
        ];
        assert_eq!(calc_checksum(&pkt3), 0xDF);
    }

    #[test]
    fn build_handshake_packet() {
        let pkt = build_handshake();
        assert_eq!(pkt[0], STX);
        assert_eq!(pkt[1], DIR_HANDSHAKE);
        for (i, &b) in pkt.iter().enumerate().take(19).skip(2) {
            assert_eq!(b, PAD, "byte[{i}]");
        }
        assert_eq!(pkt[19], calc_checksum(&pkt));
        assert_eq!(pkt[19], 0xDF);
    }

    #[test]
    fn build_handshake_with_key_packet() {
        let gid = DeviceGid::new(TEST_DEVICE_GID);
        let pkt = build_handshake_with_key(&gid);
        assert_eq!(pkt[0], STX);
        assert_eq!(pkt[1], DIR_HANDSHAKE);
        assert_eq!(&pkt[2..8], &TEST_DEVICE_GID);
        assert_eq!(pkt[19], calc_checksum(&pkt));
    }

    #[test]
    fn build_heat_packet() {
        // 왼쪽 33°C(0x21) → 34°C(0x22), 오른쪽 28°C(0x1C) 유지
        let pkt = build_heat(SIDE_LEFT, 0x21, 0x1C, 0x22, 0x1C);
        let expected = [
            0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21, 0x1C, 0x23, 0x22, 0x1C, 0xFE, 0xFE,
            0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0x3A,
        ];
        assert_eq!(pkt, expected);
    }

    #[test]
    fn build_heat_packet_half_degree() {
        // 왼쪽 33.5°C(0xA1 = 33.5 + 127.5)
        let pkt = build_heat(SIDE_LEFT, 0x21, 0x1C, 0xA1, 0x1C);
        let expected = [
            0xB2, 0x80, 0x01, 0x02, 0x00, 0x02, 0x25, 0x21, 0x1C, 0x23, 0xA1, 0x1C, 0xFE, 0xFE,
            0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xB9,
        ];
        assert_eq!(pkt, expected);
    }

    #[test]
    fn build_power_on_packet() {
        let pkt = build_power_on(0x21, 0x1C);
        assert_eq!(pkt[0], STX);
        assert_eq!(pkt[2], MODE_POWER);
        assert_eq!(pkt[3], SIDE_BOTH);
        assert_eq!(pkt[6], 0x2B, "ON 마커");
        assert_eq!(pkt[19], calc_checksum(&pkt));
    }

    #[test]
    fn build_power_off_packet() {
        let pkt = build_power_off(0x21, 0x1C);
        assert_eq!(pkt[0], STX);
        assert_eq!(pkt[2], MODE_POWER);
        assert_eq!(pkt[6], 0xAB, "OFF 마커");
        // PROTOCOL.md: 오른쪽 현재 온도가 byte[11], byte[13]에 들어감
        assert_eq!(pkt[11], 0x1C);
        assert_eq!(pkt[13], 0x1C);
        assert_eq!(pkt[19], calc_checksum(&pkt));
    }

    #[test]
    fn parse_status_packet() {
        // HEAT 모드, 양쪽 활성, 왼쪽 33°C, 오른쪽 28°C
        let mut pkt = [
            0xB2, 0x00, 0x01, 0x06, 0x22, 0x02, 0x25, 0x21, 0x1C, 0x23, 0x21, 0x1C, 0xFE, 0xFE,
            0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0x00,
        ];
        pkt[19] = calc_checksum(&pkt);

        let st = parse_status(&pkt).unwrap();
        assert_eq!(st.mode, MODE_HEAT);
        assert_eq!(st.mode_name, "HEAT");
        assert_eq!(st.side, SIDE_BOTH);
        assert_eq!(st.left_current, 33.0);
        assert_eq!(st.right_current, 28.0);
        assert!(!st.powered_off);
    }

    #[test]
    fn parse_status_power_off() {
        // POWER 모드, 목표온도 0/0 → PoweredOff
        let mut pkt = [
            0xB2, 0x00, 0x06, 0x06, 0x00, 0xFE, 0x00, 0x00, 0x00, 0xFE, 0x00, 0x00, 0xFE, 0xFE,
            0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0x00,
        ];
        pkt[19] = calc_checksum(&pkt);

        let st = parse_status(&pkt).unwrap();
        assert!(st.powered_off);
    }

    #[test]
    fn parse_status_checksum_mismatch() {
        let mut pkt = [
            0xB2, 0x00, 0x01, 0x06, 0x22, 0x02, 0x25, 0x21, 0x1C, 0x23, 0x21, 0x1C, 0xFE, 0xFE,
            0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0x00,
        ];
        pkt[19] = calc_checksum(&pkt).wrapping_add(1); // 의도적으로 틀린 체크섬
        assert!(parse_status(&pkt).is_err());
    }

    #[test]
    fn parse_status_invalid() {
        // 케이스 1: 너무 짧음
        assert!(parse_status(&[0xB2, 0x00]).is_err());

        // 케이스 2: 잘못된 STX
        let mut pkt = [0u8; 20];
        pkt[0] = 0xAA;
        assert!(parse_status(&pkt).is_err());

        // 케이스 3: STATUS가 아닌 방향 (앱→매트)
        pkt[0] = STX;
        pkt[1] = DIR_APP_TO_MAT;
        assert!(parse_status(&pkt).is_err());
    }

    #[test]
    fn parse_auth_response_packet() {
        // 인증 완료 응답 (authType = 0x02)
        let mut pkt = [
            0xB2, 0xF1, 0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
            0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
        ];
        pkt[19] = calc_checksum(&pkt);

        assert_eq!(parse_auth_response(&pkt).unwrap(), 0x02);
    }

    #[test]
    fn parse_device_gid_packet() {
        // 페어링 응답 (authType=0x01, DeviceGid=13CE3CC53E5A)
        let mut pkt = [
            0xB2, 0xF1, 0x01, 0x13, 0xCE, 0x3C, 0xC5, 0x3E, 0x5A, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
            0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
        ];
        pkt[19] = calc_checksum(&pkt);

        let gid = parse_device_gid(&pkt).unwrap();
        assert_eq!(gid, DeviceGid::new(TEST_DEVICE_GID));
    }

    #[test]
    fn device_gid_from_hex() {
        let gid = DeviceGid::from_hex("13CE3CC53E5A").unwrap();
        assert_eq!(gid, DeviceGid::new(TEST_DEVICE_GID));

        // 유효하지 않은 16진수 문자
        assert!(DeviceGid::from_hex("ZZZZ").is_err());
        // 길이가 6바이트가 아님 (2바이트)
        assert!(DeviceGid::from_hex("AABB").is_err());
    }

    #[test]
    fn device_gid_display() {
        let gid = DeviceGid::new(TEST_DEVICE_GID);
        assert_eq!(gid.to_string(), "13CE3CC53E5A");
    }

    #[test]
    fn format_packet_hex() {
        assert_eq!(format_packet(&[0xB2, 0x01, 0xFE]), "B2 01 FE");
    }

    #[test]
    fn parse_auth_response_checksum_mismatch() {
        let mut pkt = [
            0xB2, 0xF1, 0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
            0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
        ];
        pkt[19] = calc_checksum(&pkt).wrapping_add(1);
        assert!(parse_auth_response(&pkt).is_err());
    }

    #[test]
    fn parse_device_gid_checksum_mismatch() {
        // 체크섬 불일치가 parse_auth_response → parse_device_gid로 전파되는지 검증
        let mut pkt = [
            0xB2, 0xF1, 0x01, 0x13, 0xCE, 0x3C, 0xC5, 0x3E, 0x5A, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
            0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
        ];
        pkt[19] = calc_checksum(&pkt).wrapping_add(1);
        assert!(parse_device_gid(&pkt).is_err());
    }
}
