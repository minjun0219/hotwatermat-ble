#!/usr/bin/env node
"use strict";

const { spawn } = require("child_process");
const path = require("path");
const fs = require("fs");

const BIN_DIR = __dirname;
const BIN_NAME = "hotwatermat-ble";

// Native binary path
function getNativeBinaryPath() {
  const ext = process.platform === "win32" ? ".exe" : "";
  return path.join(BIN_DIR, BIN_NAME + ext);
}

// BLE commands that require the native binary
const NATIVE_COMMANDS = ["scan", "status", "temp", "on", "off"];

// WASM protocol commands
const WASM_COMMANDS = [
  "encode-temp",
  "decode-temp",
  "build-handshake",
  "build-heat",
  "build-power-on",
  "build-power-off",
  "parse-status",
  "checksum",
];

function printUsage() {
  const hasNative = fs.existsSync(getNativeBinaryPath());

  console.log(`hotwatermat-ble — BLE 온수매트 CLI

Usage: hotwatermat-ble <command> [options]
`);

  if (hasNative) {
    console.log(`BLE Commands (native binary):
  scan                  Scan for BLE devices
  status                Show current mat status
  temp --left N --right N  Set target temperature (28.0-48.0°C)
  on                    Power on the mat
  off                   Power off the mat
`);
  }

  console.log(`Protocol Commands (WASM, no BLE required):
  encode-temp <temp>        Encode temperature to byte (e.g. 35.0 → 163)
  decode-temp <byte>        Decode byte to temperature (e.g. 163 → 35.0)
  build-handshake [gid]     Build handshake packet (hex output)
  build-heat <side> <lt> <rt>  Build heat command packet
  build-power-on            Build power-on packet
  build-power-off           Build power-off packet
  parse-status <hex>        Parse a 20-byte status packet
  checksum <hex>            Calculate checksum for a packet
`);

  if (!hasNative) {
    console.log(`Note: BLE commands (scan, status, temp, on, off) require the native binary.
  Download from: https://github.com/minjun0219/hotwatermat-ble/releases
  Or reinstall:  npm install hotwatermat-ble
`);
  }
}

// Delegate to native binary
function runNative(args) {
  const binPath = getNativeBinaryPath();

  if (!fs.existsSync(binPath)) {
    console.error(
      `Error: Native binary not found at ${binPath}`
    );
    console.error(
      "BLE commands require the native binary."
    );
    console.error(
      "Download from: https://github.com/minjun0219/hotwatermat-ble/releases"
    );
    console.error(
      "Or reinstall with: npm install hotwatermat-ble"
    );
    process.exit(1);
  }

  const child = spawn(binPath, args, {
    stdio: "inherit",
    env: process.env,
  });

  child.on("error", (err) => {
    console.error(`Failed to run native binary: ${err.message}`);
    process.exit(1);
  });

  child.on("exit", (code) => {
    process.exit(code || 0);
  });
}

// WASM-based protocol commands
async function runWasmCommand(command, args) {
  // Load the compiled library from dist/
  let lib;
  try {
    lib = require("../dist/index.js");
  } catch {
    console.error(
      "Error: Cannot load hotwatermat-ble library."
    );
    console.error(
      "WASM protocol commands require the package to be built first."
    );
    console.error("Run: cd npm && npm run build");
    process.exit(1);
  }

  await lib.init();

  switch (command) {
    case "encode-temp": {
      if (args.length < 1) {
        console.error("Usage: hotwatermat-ble encode-temp <temperature>");
        console.error("  temperature: 28.0-48.0 (0.5°C steps)");
        process.exit(1);
      }
      const temp = parseFloat(args[0]);
      if (isNaN(temp)) {
        console.error(`Invalid temperature: ${args[0]}`);
        process.exit(1);
      }
      const encoded = lib.encodeTemp(temp);
      console.log(encoded);
      break;
    }

    case "decode-temp": {
      if (args.length < 1) {
        console.error("Usage: hotwatermat-ble decode-temp <byte>");
        process.exit(1);
      }
      const byteVal = parseInt(args[0], 10);
      if (isNaN(byteVal)) {
        console.error(`Invalid byte value: ${args[0]}`);
        process.exit(1);
      }
      const decoded = lib.decodeTemp(byteVal);
      console.log(decoded);
      break;
    }

    case "build-handshake": {
      const gid = args[0] || undefined;
      const pkt = lib.buildHandshake(gid);
      console.log(Buffer.from(pkt).toString("hex"));
      break;
    }

    case "build-heat": {
      if (args.length < 3) {
        console.error(
          "Usage: hotwatermat-ble build-heat <side> <left-target> <right-target>"
        );
        console.error("  side: left, right, or both");
        console.error("  targets: temperature in °C (28.0-48.0)");
        process.exit(1);
      }
      const sideMap = { left: lib.SIDE_LEFT, right: lib.SIDE_RIGHT, both: lib.SIDE_BOTH };
      const sideArg = args[0].toLowerCase();
      const side = sideMap[sideArg];
      if (side === undefined) {
        console.error(`Invalid side: ${args[0]} (use: left, right, both)`);
        process.exit(1);
      }
      const lt = lib.encodeTemp(parseFloat(args[1]));
      const rt = lib.encodeTemp(parseFloat(args[2]));
      const pkt = lib.buildHeat(side, lt, rt, lt, rt);
      console.log(Buffer.from(pkt).toString("hex"));
      break;
    }

    case "build-power-on": {
      const pkt = lib.buildPowerOn();
      console.log(Buffer.from(pkt).toString("hex"));
      break;
    }

    case "build-power-off": {
      const pkt = lib.buildPowerOff();
      console.log(Buffer.from(pkt).toString("hex"));
      break;
    }

    case "parse-status": {
      if (args.length < 1) {
        console.error("Usage: hotwatermat-ble parse-status <hex>");
        console.error("  hex: 40 hex chars (20 bytes)");
        process.exit(1);
      }
      const hex = args[0].replace(/\s/g, "");
      if (hex.length !== 40) {
        console.error(`Expected 40 hex chars (20 bytes), got ${hex.length}`);
        process.exit(1);
      }
      const buf = new Uint8Array(Buffer.from(hex, "hex"));
      const status = lib.parseStatus(buf);
      console.log(JSON.stringify(status, null, 2));
      break;
    }

    case "checksum": {
      if (args.length < 1) {
        console.error("Usage: hotwatermat-ble checksum <hex>");
        process.exit(1);
      }
      const hex = args[0].replace(/\s/g, "");
      const buf = new Uint8Array(Buffer.from(hex, "hex"));
      const cs = lib.calcChecksum(buf);
      console.log(`0x${cs.toString(16).padStart(2, "0")} (${cs})`);
      break;
    }

    default:
      console.error(`Unknown command: ${command}`);
      printUsage();
      process.exit(1);
  }
}

// Main
const args = process.argv.slice(2);
const command = args[0];

if (!command || command === "--help" || command === "-h") {
  printUsage();
  process.exit(0);
}

if (command === "--version" || command === "-v") {
  const pkg = require("../package.json");
  console.log(pkg.version);
  process.exit(0);
}

if (NATIVE_COMMANDS.includes(command)) {
  // Delegate to native binary
  runNative(args);
} else if (WASM_COMMANDS.includes(command)) {
  // Run WASM protocol command
  runWasmCommand(command, args.slice(1)).catch((err) => {
    console.error(`Error: ${err.message}`);
    process.exit(1);
  });
} else {
  // Try native binary first (it might support additional commands/flags)
  if (fs.existsSync(getNativeBinaryPath())) {
    runNative(args);
  } else {
    console.error(`Unknown command: ${command}`);
    printUsage();
    process.exit(1);
  }
}
