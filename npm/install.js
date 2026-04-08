#!/usr/bin/env node
"use strict";

const { execSync } = require("child_process");
const fs = require("fs");
const https = require("https");
const os = require("os");
const path = require("path");

const VERSION = require("./package.json").version;
const REPO = "Prgebish/mcp-telegram";

function getPlatform() {
  const platform = os.platform();
  const arch = os.arch();

  const osMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };

  const mappedOS = osMap[platform];
  const mappedArch = archMap[arch];

  if (!mappedOS || !mappedArch) {
    throw new Error(`Unsupported platform: ${platform}/${arch}`);
  }

  return { os: mappedOS, arch: mappedArch };
}

function getBinaryName() {
  return os.platform() === "win32" ? "mcp-telegram.exe" : "mcp-telegram";
}

function getDownloadURL(platform) {
  const ext = platform.os === "windows" ? "zip" : "tar.gz";
  return `https://github.com/${REPO}/releases/download/v${VERSION}/mcp-telegram_${platform.os}_${platform.arch}.${ext}`;
}

function download(url) {
  return new Promise((resolve, reject) => {
    const get = (url) => {
      https
        .get(url, (res) => {
          if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
            get(res.headers.location);
            return;
          }
          if (res.statusCode !== 200) {
            reject(new Error(`Download failed: HTTP ${res.statusCode}`));
            return;
          }
          const chunks = [];
          res.on("data", (chunk) => chunks.push(chunk));
          res.on("end", () => resolve(Buffer.concat(chunks)));
          res.on("error", reject);
        })
        .on("error", reject);
    };
    get(url);
  });
}

async function extractTarGz(buffer, destDir) {
  const tmpFile = path.join(destDir, "tmp.tar.gz");
  fs.writeFileSync(tmpFile, buffer);
  execSync(`tar xzf "${tmpFile}" -C "${destDir}"`);
  fs.unlinkSync(tmpFile);
}

async function extractZip(buffer, destDir) {
  const tmpFile = path.join(destDir, "tmp.zip");
  fs.writeFileSync(tmpFile, buffer);
  execSync(`tar xf "${tmpFile}" -C "${destDir}"`, { stdio: "ignore" }).toString?.();
  // Fallback for Windows
  try {
    execSync(
      `powershell -Command "Expand-Archive -Force '${tmpFile}' '${destDir}'"`,
      { stdio: "ignore" }
    );
  } catch {}
  fs.unlinkSync(tmpFile);
}

async function main() {
  const binDir = __dirname;
  const binaryName = getBinaryName();
  const binaryPath = path.join(binDir, binaryName);

  if (fs.existsSync(binaryPath)) {
    return;
  }

  const platform = getPlatform();
  const url = getDownloadURL(platform);

  console.log(`Downloading mcp-telegram v${VERSION} for ${platform.os}/${platform.arch}...`);

  const buffer = await download(url);

  if (platform.os === "windows") {
    await extractZip(buffer, binDir);
  } else {
    await extractTarGz(buffer, binDir);
  }

  fs.chmodSync(binaryPath, 0o755);
  console.log("mcp-telegram installed successfully.");
}

main().catch((err) => {
  console.error(`Failed to install mcp-telegram: ${err.message}`);
  process.exit(1);
});
