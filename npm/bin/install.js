#!/usr/bin/env node

"use strict";

const https = require("https");
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");
const os = require("os");

if (process.env.HOTWATERMAT_SKIP_BINARY === "1") {
  console.log("HOTWATERMAT_SKIP_BINARY=1 — skipping binary download.");
  process.exit(0);
}

const VERSION = require("../package.json").version;
const REPO = "minjun0219/hotwatermat-ble";

function getPlatform() {
  const platform = os.platform();
  const arch = os.arch();

  const osMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };

  const mappedOS = osMap[platform];
  const mappedArch = archMap[arch];

  if (!mappedOS || !mappedArch) {
    return null;
  }

  return { os: mappedOS, arch: mappedArch };
}

function download(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return download(res.headers.location).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

async function main() {
  const plat = getPlatform();
  if (!plat) {
    console.warn(
      `Unsupported platform: ${os.platform()}-${os.arch()}.\n` +
        "Build from source: https://github.com/minjun0219/hotwatermat-ble"
    );
    process.exit(0);
  }

  const archive = `hotwatermat-ble_${VERSION}_${plat.os}_${plat.arch}.tar.gz`;
  const url = `https://github.com/${REPO}/releases/download/v${VERSION}/${archive}`;

  console.log(`Downloading ${archive}...`);

  let buf;
  try {
    buf = await download(url);
  } catch (err) {
    console.warn(`Failed to download binary: ${err.message}`);
    console.warn("You can build from source: https://github.com/minjun0219/hotwatermat-ble");
    process.exit(0);
  }

  const tmpFile = path.join(os.tmpdir(), archive);
  fs.writeFileSync(tmpFile, buf);

  try {
    execSync(`tar xzf "${tmpFile}" -C "${__dirname}" hotwatermat-ble hotwatermat-ble-mcp`, {
      stdio: "pipe",
    });
  } catch (err) {
    console.warn(`Failed to extract archive: ${err.message}`);
    process.exit(0);
  } finally {
    try {
      fs.unlinkSync(tmpFile);
    } catch (_) {}
  }

  // Ensure binaries are executable
  for (const name of ["hotwatermat-ble", "hotwatermat-ble-mcp"]) {
    const binPath = path.join(__dirname, name);
    if (fs.existsSync(binPath)) {
      fs.chmodSync(binPath, 0o755);
    }
  }

  console.log("hotwatermat-ble binaries installed successfully.");
}

main();
