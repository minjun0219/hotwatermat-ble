#!/usr/bin/env node
"use strict";

// postinstall script: downloads the native hotwatermat-ble binary
// from GitHub releases for the current OS/arch.
// Gracefully exits on failure (WASM fallback remains available).

const https = require("https");
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");
const os = require("os");

const REPO = "minjun0219/hotwatermat-ble";
const BIN_NAME = "hotwatermat-ble";
const BIN_DIR = __dirname;

// Map Node.js platform/arch to release naming
const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function getBinaryPath() {
  const ext = process.platform === "win32" ? ".exe" : "";
  return path.join(BIN_DIR, BIN_NAME + ext);
}

function getPlatformArch() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];

  if (!platform || !arch) {
    return null;
  }
  return { platform, arch };
}

// Follow redirects for HTTPS requests
function httpsGet(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { "User-Agent": "hotwatermat-ble-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          httpsGet(res.headers.location).then(resolve, reject);
          return;
        }
        if (res.statusCode !== 200) {
          reject(new Error(`HTTP ${res.statusCode} for ${url}`));
          return;
        }
        resolve(res);
      })
      .on("error", reject);
  });
}

function downloadToFile(url, dest) {
  return new Promise((resolve, reject) => {
    httpsGet(url)
      .then((res) => {
        const file = fs.createWriteStream(dest);
        res.pipe(file);
        file.on("finish", () => {
          file.close(resolve);
        });
        file.on("error", (err) => {
          fs.unlink(dest, () => {});
          reject(err);
        });
      })
      .catch(reject);
  });
}

async function tryDownloadTarGz(version, platform, arch) {
  // New release format: hotwatermat-ble_v0.1.0_darwin_arm64.tar.gz
  const tarName = `${BIN_NAME}_${version}_${platform}_${arch}.tar.gz`;
  const url = `https://github.com/${REPO}/releases/download/${version}/${tarName}`;
  const tmpFile = path.join(os.tmpdir(), tarName);

  try {
    await downloadToFile(url, tmpFile);
    // Extract only the hotwatermat-ble binary from the tar.gz
    execSync(`tar xzf "${tmpFile}" -C "${BIN_DIR}" ${BIN_NAME}`, {
      stdio: "pipe",
    });
    fs.unlinkSync(tmpFile);
    return true;
  } catch {
    // Clean up
    try { fs.unlinkSync(tmpFile); } catch {}
    return false;
  }
}

async function tryDownloadRawBinary(version, platform, arch) {
  // Old release format: hotwatermat-ble-darwin-arm64
  const binaryName = `${BIN_NAME}-${platform}-${arch}`;
  const url = `https://github.com/${REPO}/releases/download/${version}/${binaryName}`;
  const dest = getBinaryPath();

  try {
    await downloadToFile(url, dest);
    return true;
  } catch {
    try { fs.unlinkSync(dest); } catch {}
    return false;
  }
}

async function getLatestTag() {
  return new Promise((resolve, reject) => {
    const url = `https://api.github.com/repos/${REPO}/releases/latest`;
    https
      .get(url, { headers: { "User-Agent": "hotwatermat-ble-npm" } }, (res) => {
        // If no latest release (404), try tags
        if (res.statusCode === 404) {
          resolve(null);
          res.resume();
          return;
        }
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          resolve(null);
          res.resume();
          return;
        }
        let data = "";
        res.on("data", (chunk) => { data += chunk; });
        res.on("end", () => {
          try {
            const json = JSON.parse(data);
            resolve(json.tag_name || null);
          } catch {
            resolve(null);
          }
        });
      })
      .on("error", () => resolve(null));
  });
}

async function main() {
  const info = getPlatformArch();
  if (!info) {
    console.log(
      `[hotwatermat-ble] Skipping binary download: unsupported platform (${process.platform}/${process.arch}).`
    );
    console.log(
      "[hotwatermat-ble] WASM protocol commands are still available."
    );
    return;
  }

  // Skip in CI unless explicitly requested
  if (process.env.HOTWATERMAT_SKIP_BINARY === "1") {
    console.log("[hotwatermat-ble] Skipping binary download (HOTWATERMAT_SKIP_BINARY=1).");
    return;
  }

  const { platform, arch } = info;

  // Determine version: use package.json version → GitHub tag
  const pkgVersion = require("../package.json").version;
  const tag = `v${pkgVersion}`;

  console.log(
    `[hotwatermat-ble] Downloading native binary for ${platform}/${arch}...`
  );

  // Try tar.gz format first (new CI), then raw binary (old releases)
  let ok = await tryDownloadTarGz(tag, platform, arch);
  if (!ok) {
    ok = await tryDownloadRawBinary(tag, platform, arch);
  }

  // If version-specific failed, try latest release
  if (!ok) {
    const latestTag = await getLatestTag();
    if (latestTag && latestTag !== tag) {
      console.log(
        `[hotwatermat-ble] Version ${tag} not found, trying latest release (${latestTag})...`
      );
      ok = await tryDownloadTarGz(latestTag, platform, arch);
      if (!ok) {
        ok = await tryDownloadRawBinary(latestTag, platform, arch);
      }
    }
  }

  if (ok) {
    const binPath = getBinaryPath();
    fs.chmodSync(binPath, 0o755);
    console.log(`[hotwatermat-ble] Binary installed: ${binPath}`);
  } else {
    console.log(
      "[hotwatermat-ble] Could not download native binary. This is OK —"
    );
    console.log(
      "[hotwatermat-ble] WASM-based protocol commands (encode-temp, decode-temp, etc.) are still available."
    );
    console.log(
      "[hotwatermat-ble] BLE commands (scan, status, temp, on, off) require the native binary."
    );
    console.log(
      "[hotwatermat-ble] You can download it manually from:"
    );
    console.log(
      `[hotwatermat-ble]   https://github.com/${REPO}/releases`
    );
  }
}

main().catch((err) => {
  // Never fail the npm install
  console.log(`[hotwatermat-ble] Binary download skipped: ${err.message}`);
});
