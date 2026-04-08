#!/usr/bin/env node
"use strict";

const { spawn } = require("child_process");
const os = require("os");
const path = require("path");

const binaryName = os.platform() === "win32" ? "mcp-telegram.exe" : "mcp-telegram";
const binaryPath = path.join(__dirname, binaryName);

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
});

child.on("exit", (code) => {
  process.exit(code ?? 1);
});
