/**
 * 온수매트 BLE 프로토콜 npm 패키지입니다.
 *
 * Go로 작성된 프로토콜 라이브러리를 WASM으로 컴파일한 것을 TypeScript로 래핑합니다.
 * Node.js 환경에서 온수매트 BLE 패킷을 생성하고 파싱할 수 있습니다.
 *
 * 사용 방법:
 * ```typescript
 * import { init, encodeTemp, buildHandshake, parseStatus } from 'hotwatermat-ble';
 *
 * // 반드시 init()을 먼저 호출해야 합니다
 * await init();
 *
 * // 온도 인코딩/디코딩
 * const encoded = encodeTemp(33.5);  // → 161 (0xA1)
 * const decoded = decodeTemp(161);    // → 33.5
 *
 * // 핸드셰이크 패킷 생성
 * const handshake = buildHandshake("13CE3CC53E5A");
 *
 * // STATUS 패킷 파싱
 * const status = parseStatus(packetData);
 * console.log(status.modeName);      // "HEAT"
 * console.log(status.leftCurrent);   // 33.0
 * ```
 */

import * as fs from "fs";
import * as path from "path";

// --- WASM 모듈 상태 관리 ---

/** WASM 모듈 초기화 완료 여부 */
let wasmReady = false;

/** 초기화된 WASM 모듈 참조 (전역 hotwatermat 객체) */
let wasmModule: any = null;

/**
 * STATUS 패킷 파싱 결과를 나타내는 인터페이스입니다.
 *
 * parseStatus() 함수가 반환하는 객체의 타입입니다.
 * 매트의 현재 상태(모드, 온도, 수위 등)를 포함합니다.
 */
interface StatusResult {
  /** 현재 동작 모드의 바이트 값 (예: 1=HEAT, 6=POWER) */
  mode: number;

  /** 현재 동작 모드의 이름 (예: "HEAT", "POWER", "SLEEP") */
  modeName: string;

  /** 좌/우 선택 값 (0x02=왼쪽, 0x04=오른쪽, 0x06=양쪽) */
  side: number;

  /** 볼륨 값 (STATUS 패킷 byte[4]의 상위 4비트) */
  volume: number;

  /** 수위 값 (1=LOW/부족, 2=OK/정상, 3=FULL/가득참) */
  waterLevel: number;

  /** 왼쪽 현재 온도 (°C, 디코딩된 값) */
  leftCurrent: number;

  /** 오른쪽 현재 온도 (°C, 디코딩된 값) */
  rightCurrent: number;

  /** 왼쪽 목표 온도 (°C, 디코딩된 값) */
  leftTarget: number;

  /** 오른쪽 목표 온도 (°C, 디코딩된 값) */
  rightTarget: number;

  /** 왼쪽 난방 중 여부 (현재 사용되지 않음) */
  leftHeating: boolean;

  /** 오른쪽 난방 중 여부 (현재 사용되지 않음) */
  rightHeating: boolean;

  /** 전원 꺼짐 여부 (Mode=POWER이고 목표온도=0일 때 true) */
  poweredOff: boolean;
}

/**
 * WASM 함수 호출 시 오류가 발생했을 때 반환되는 객체의 인터페이스입니다.
 * {error: "에러 메시지"} 형태입니다.
 */
interface ErrorResult {
  error: string;
}

/**
 * 값이 ErrorResult인지 확인하는 타입 가드 함수입니다.
 * WASM 함수들은 오류 시 {error: string} 객체를 반환하므로,
 * 반환값이 오류인지 정상 결과인지 구분할 때 사용합니다.
 */
function isError(v: any): v is ErrorResult {
  return typeof v === "object" && v !== null && "error" in v;
}

/**
 * WASM 모듈이 초기화되었는지 확인합니다.
 * 초기화되지 않았으면 오류를 던집니다.
 *
 * 모든 프로토콜 함수 호출 전에 실행되어, init()을 먼저 호출하도록 강제합니다.
 */
function ensureReady(): void {
  if (!wasmReady) {
    throw new Error(
      "WASM not initialized. Call init() first and await its result."
    );
  }
}

/**
 * WASM 모듈을 초기화합니다.
 *
 * 다른 모든 함수를 사용하기 전에 반드시 이 함수를 호출하고 await해야 합니다.
 * 이미 초기화된 경우 아무 작업도 수행하지 않습니다 (중복 호출 안전).
 *
 * 초기화 과정:
 * 1. TinyGo 런타임(wasm_exec.js) 로드
 * 2. WASM 바이너리(hotwatermat.wasm) 로드
 * 3. WebAssembly 인스턴스 생성 및 Go 런타임 실행
 * 4. 전역 hotwatermat 객체 확인
 *
 * @throws WASM 파일을 찾을 수 없거나 초기화에 실패한 경우
 */
export async function init(): Promise<void> {
  if (wasmReady) return;

  // TinyGo의 WASM 실행 환경(wasm_exec.js)을 로드
  // 이 파일은 Go의 런타임을 JavaScript에서 실행하기 위한 글루 코드입니다
  const execPath = path.join(__dirname, "wasm_exec.js");
  if (fs.existsSync(execPath)) {
    require(execPath);
  }

  // WASM 바이너리 로드 및 인스턴스화
  const wasmPath = path.join(__dirname, "hotwatermat.wasm");
  const wasmBuffer = fs.readFileSync(wasmPath);

  // Go 런타임 인스턴스 생성 및 WASM 실행
  const go = new (globalThis as any).Go();
  const result = await WebAssembly.instantiate(wasmBuffer, go.importObject);
  go.run(result.instance);

  // WASM에서 등록한 전역 hotwatermat 객체 확인
  wasmModule = (globalThis as any).hotwatermat;
  if (!wasmModule) {
    throw new Error("WASM module failed to register global 'hotwatermat'");
  }
  wasmReady = true;
}

/**
 * 온도를 BLE 바이트로 인코딩합니다.
 *
 * 인코딩 규칙:
 * - 정수 온도 (33.0°C): 그대로 바이트 → 33 (0x21)
 * - 소수점 온도 (33.5°C): temp + 127.5 → 161 (0xA1)
 *
 * @param temp 설정할 온도 (28.0~48.0°C, 0.5°C 단위)
 * @returns 인코딩된 바이트 값
 * @throws 범위 초과 또는 0.5°C 단위가 아닌 경우
 */
export function encodeTemp(temp: number): number {
  ensureReady();
  const result = wasmModule.encodeTemp(temp);
  if (isError(result)) throw new Error(result.error);
  return result as number;
}

/**
 * BLE 바이트를 온도로 디코딩합니다.
 *
 * 디코딩 규칙:
 * - 바이트 > 127: byte - 127.5 (소수점 온도)
 * - 바이트 ≤ 127: 그대로 (정수 온도)
 *
 * @param byteVal 디코딩할 바이트 값
 * @returns 디코딩된 온도 (°C)
 */
export function decodeTemp(byteVal: number): number {
  ensureReady();
  return wasmModule.decodeTemp(byteVal) as number;
}

/**
 * 20바이트 패킷의 체크섬을 계산합니다.
 *
 * 계산 방법: bytes[1]~bytes[18]의 합을 0xFF로 AND 연산
 *
 * @param packet 20바이트 패킷 (Uint8Array)
 * @returns 계산된 체크섬 바이트 값
 */
export function calcChecksum(packet: Uint8Array): number {
  ensureReady();
  const result = wasmModule.calcChecksum(packet);
  if (isError(result)) throw new Error(result.error);
  return result as number;
}

/**
 * 핸드셰이크 패킷을 생성합니다.
 *
 * - DeviceGid 없이 호출: 최초 페어링용 핸드셰이크
 * - DeviceGid 포함 호출: 재연결용 인증 핸드셰이크
 *
 * @param deviceGidHex 12자리 16진수 문자열 (선택, 예: "13CE3CC53E5A")
 * @returns 20바이트 핸드셰이크 패킷 (Uint8Array)
 * @throws 잘못된 DeviceGid 형식
 */
export function buildHandshake(deviceGidHex?: string): Uint8Array {
  ensureReady();
  const result = deviceGidHex
    ? wasmModule.buildHandshake(deviceGidHex)
    : wasmModule.buildHandshake();
  if (isError(result)) throw new Error(result.error);
  return result as Uint8Array;
}

/**
 * 난방(HEAT) 명령 패킷을 생성합니다.
 *
 * 매트의 목표 온도를 설정하는 핵심 명령입니다.
 * STATUS에서 받은 현재 온도(원시 바이트)와 encodeTemp로 인코딩한 목표 온도를 전달합니다.
 *
 * @param side 좌/우 선택 비트마스크 (0x02=왼쪽, 0x04=오른쪽, 0x06=양쪽)
 * @param leftCur 왼쪽 현재 온도 바이트 (STATUS에서 받은 원시값)
 * @param rightCur 오른쪽 현재 온도 바이트 (STATUS에서 받은 원시값)
 * @param leftTarget 왼쪽 목표 온도 바이트 (encodeTemp로 인코딩)
 * @param rightTarget 오른쪽 목표 온도 바이트 (encodeTemp로 인코딩)
 * @returns 20바이트 HEAT 명령 패킷 (Uint8Array)
 */
export function buildHeat(
  side: number,
  leftCur: number,
  rightCur: number,
  leftTarget: number,
  rightTarget: number
): Uint8Array {
  ensureReady();
  const result = wasmModule.buildHeat(
    side,
    leftCur,
    rightCur,
    leftTarget,
    rightTarget
  );
  if (isError(result)) throw new Error(result.error);
  return result as Uint8Array;
}

/**
 * 전원 켜기(Power ON) 패킷을 생성합니다.
 *
 * @param leftCur 왼쪽 현재 온도 바이트 (기본값 0, STATUS에서 받은 원시값)
 * @param rightCur 오른쪽 현재 온도 바이트 (기본값 0, STATUS에서 받은 원시값)
 * @returns 20바이트 Power ON 패킷 (Uint8Array)
 */
export function buildPowerOn(
  leftCur: number = 0,
  rightCur: number = 0
): Uint8Array {
  ensureReady();
  return wasmModule.buildPowerOn(leftCur, rightCur) as Uint8Array;
}

/**
 * 전원 끄기(Power OFF) 패킷을 생성합니다.
 *
 * 전원을 끈 후에도 BLE를 통해 다시 켤 수 있습니다.
 *
 * @param leftCur 왼쪽 현재 온도 바이트 (기본값 0, STATUS에서 받은 원시값)
 * @param rightCur 오른쪽 현재 온도 바이트 (기본값 0, STATUS에서 받은 원시값)
 * @returns 20바이트 Power OFF 패킷 (Uint8Array)
 */
export function buildPowerOff(
  leftCur: number = 0,
  rightCur: number = 0
): Uint8Array {
  ensureReady();
  return wasmModule.buildPowerOff(leftCur, rightCur) as Uint8Array;
}

/**
 * STATUS 알림 패킷을 파싱합니다.
 *
 * CHAR1(0100dd) 특성의 알림으로 수신된 20바이트 패킷을 파싱하여
 * 매트의 현재 상태(모드, 온도, 수위 등)를 반환합니다.
 *
 * @param packet 20바이트 STATUS 패킷 (Uint8Array)
 * @returns 파싱된 상태 정보 (StatusResult)
 * @throws 패킷이 유효하지 않은 경우 (길이 부족, 잘못된 STX, 잘못된 방향)
 */
export function parseStatus(packet: Uint8Array): StatusResult {
  ensureReady();
  const result = wasmModule.parseStatus(packet);
  if (isError(result)) throw new Error(result.error);
  return result as StatusResult;
}

// --- BLE UUID 상수 ---

/** 온수매트 BLE 서비스 UUID */
export const SERVICE_UUID = "00001c0d-d102-11e1-9b23-2ce2a80000dd";

/** CHAR1 UUID — STATUS 알림 수신용 (매트 → 앱) */
export const CHAR1_UUID = "00001c0d-d102-11e1-9b23-2ce2a80100dd";

/** CHAR2 UUID — 명령 쓰기 + 인증 응답용 (앱 → 매트) */
export const CHAR2_UUID = "00001c0d-d102-11e1-9b23-2ce2a80200dd";

// --- 좌/우 선택 상수 ---

/** 왼쪽 매트 선택 (0x02) */
export const SIDE_LEFT = 0x02;

/** 오른쪽 매트 선택 (0x04) */
export const SIDE_RIGHT = 0x04;

/** 양쪽 매트 모두 선택 (0x06 = SIDE_LEFT | SIDE_RIGHT) */
export const SIDE_BOTH = 0x06;

// --- 온도 범위 상수 ---

/** 설정 가능한 최저 온도 (28.0°C) */
export const TEMP_MIN = 28.0;

/** 설정 가능한 최고 온도 (48.0°C) */
export const TEMP_MAX = 48.0;
