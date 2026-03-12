import * as fs from "fs";
import * as path from "path";

// WASM module interface (populated after init)
let wasmReady = false;
let wasmModule: any = null;

interface StatusResult {
  mode: number;
  modeName: string;
  side: number;
  volume: number;
  waterLevel: number;
  leftCurrent: number;
  rightCurrent: number;
  leftTarget: number;
  rightTarget: number;
  leftHeating: boolean;
  rightHeating: boolean;
  poweredOff: boolean;
}

interface ErrorResult {
  error: string;
}

function isError(v: any): v is ErrorResult {
  return typeof v === "object" && v !== null && "error" in v;
}

function ensureReady(): void {
  if (!wasmReady) {
    throw new Error(
      "WASM not initialized. Call init() first and await its result."
    );
  }
}

/**
 * Initialize the WASM module. Must be called before using any other function.
 */
export async function init(): Promise<void> {
  if (wasmReady) return;

  // Load wasm_exec.js (TinyGo runtime)
  const execPath = path.join(__dirname, "wasm_exec.js");
  if (fs.existsSync(execPath)) {
    require(execPath);
  }

  const wasmPath = path.join(__dirname, "hotwatermat.wasm");
  const wasmBuffer = fs.readFileSync(wasmPath);

  const go = new (globalThis as any).Go();
  const result = await WebAssembly.instantiate(wasmBuffer, go.importObject);
  go.run(result.instance);

  wasmModule = (globalThis as any).hotwatermat;
  if (!wasmModule) {
    throw new Error("WASM module failed to register global 'hotwatermat'");
  }
  wasmReady = true;
}

/**
 * Encode a temperature (0.5°C precision) to its BLE byte value.
 * @param temp Temperature in range 28.0-48.0, 0.5 steps
 */
export function encodeTemp(temp: number): number {
  ensureReady();
  const result = wasmModule.encodeTemp(temp);
  if (isError(result)) throw new Error(result.error);
  return result as number;
}

/**
 * Decode a BLE byte value to temperature.
 */
export function decodeTemp(byteVal: number): number {
  ensureReady();
  return wasmModule.decodeTemp(byteVal) as number;
}

/**
 * Calculate checksum for a 20-byte packet.
 */
export function calcChecksum(packet: Uint8Array): number {
  ensureReady();
  const result = wasmModule.calcChecksum(packet);
  if (isError(result)) throw new Error(result.error);
  return result as number;
}

/**
 * Build a handshake packet.
 * @param deviceGidHex Optional 12-char hex string for DeviceGid
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
 * Build a HEAT command packet.
 * @param side Side bitmask (0x02=left, 0x04=right, 0x06=both)
 * @param leftCur Left current temp byte from STATUS
 * @param rightCur Right current temp byte from STATUS
 * @param leftTarget Left target temp (encoded)
 * @param rightTarget Right target temp (encoded)
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
 * Build a power-on command packet.
 */
export function buildPowerOn(
  leftCur: number = 0,
  rightCur: number = 0
): Uint8Array {
  ensureReady();
  return wasmModule.buildPowerOn(leftCur, rightCur) as Uint8Array;
}

/**
 * Build a power-off command packet.
 */
export function buildPowerOff(
  leftCur: number = 0,
  rightCur: number = 0
): Uint8Array {
  ensureReady();
  return wasmModule.buildPowerOff(leftCur, rightCur) as Uint8Array;
}

/**
 * Parse a STATUS notification packet from the mat.
 */
export function parseStatus(packet: Uint8Array): StatusResult {
  ensureReady();
  const result = wasmModule.parseStatus(packet);
  if (isError(result)) throw new Error(result.error);
  return result as StatusResult;
}

// Constants
export const SERVICE_UUID = "00001c0d-d102-11e1-9b23-2ce2a80000dd";
export const CHAR1_UUID = "00001c0d-d102-11e1-9b23-2ce2a80100dd";
export const CHAR2_UUID = "00001c0d-d102-11e1-9b23-2ce2a80200dd";
export const SIDE_LEFT = 0x02;
export const SIDE_RIGHT = 0x04;
export const SIDE_BOTH = 0x06;
export const TEMP_MIN = 28.0;
export const TEMP_MAX = 48.0;
