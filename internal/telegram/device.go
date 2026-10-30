package telegram

import (
	"runtime"

	"github.com/gotd/td/telegram"
)

// Device describes this app in initConnection. Without it gotd reports the Go
// version as the device model and "darwin" as the system, which reads like a
// script in Settings → Devices and may look suspicious to Telegram.
func Device() telegram.DeviceConfig {
	model, system := "PC", runtime.GOOS
	switch runtime.GOOS {
	case "darwin":
		model, system = "Mac", "macOS"
	case "linux":
		system = "Linux"
	case "windows":
		system = "Windows"
	}
	// SystemLangCode "en-US" instead of gotd's "en": with "en" login codes
	// were reported not to arrive (LonamiWebs/Telethon#4730).
	return telegram.DeviceConfig{
		DeviceModel:    model,
		SystemVersion:  system,
		AppVersion:     "mcp-telegram 0.1.0",
		SystemLangCode: "en-US",
	}
}
