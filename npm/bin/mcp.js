#!/usr/bin/env node

"use strict";

const { execFileSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const binPath = path.join(__dirname, "hotwatermat-ble-mcp");

if (!fs.existsSync(binPath)) {
  console.error(
    "hotwatermat-ble-mcp binary not found.\n" +
      "Run `npm install` to download it, or build from source:\n" +
      "  https://github.com/minjun0219/hotwatermat-ble"
  );
  process.exit(1);
}

try {
  execFileSync(binPath, process.argv.slice(2), { stdio: "inherit" });
} catch (e) {
  process.exit(e.status ?? 1);
}
