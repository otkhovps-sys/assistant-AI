package tools

import (
	"fmt"
	"strings"
)

// Mã phím ảo của Windows.
var vkByName = map[string]uint16{
	"ctrl": 0x11, "control": 0x11, "alt": 0x12, "shift": 0x10, "win": 0x5B, "windows": 0x5B,
	"enter": 0x0D, "return": 0x0D, "esc": 0x1B, "escape": 0x1B, "tab": 0x09, "space": 0x20,
	"backspace": 0x08, "delete": 0x2E, "del": 0x2E, "insert": 0x2D, "home": 0x24, "end": 0x23,
	"pageup": 0x21, "pagedown": 0x22, "left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28,
	"printscreen": 0x2C, "capslock": 0x14,
}

const (
	vkVolumeMute = 0xAD
	vkVolumeDown = 0xAE
	vkVolumeUp   = 0xAF
	vkMediaNext  = 0xB0
	vkMediaPrev  = 0xB1
	vkMediaStop  = 0xB2
	vkMediaPlay  = 0xB3
)

// parseCombo đổi chuỗi như "ctrl+shift+esc" thành danh sách mã phím ảo.
func parseCombo(combo string) ([]uint16, error) {
	var vks []uint16
	for _, part := range strings.Split(combo, "+") {
		name := strings.ToLower(strings.TrimSpace(part))
		switch {
		case name == "":
			return nil, fmt.Errorf("tổ hợp phím không hợp lệ: %q", combo)
		case vkByName[name] != 0:
			vks = append(vks, vkByName[name])
		case len(name) == 1 && (name[0] >= 'a' && name[0] <= 'z'):
			vks = append(vks, uint16(name[0]-'a'+'A'))
		case len(name) == 1 && (name[0] >= '0' && name[0] <= '9'):
			vks = append(vks, uint16(name[0]))
		case name[0] == 'f' && len(name) <= 3:
			var n int
			if _, err := fmt.Sscanf(name, "f%d", &n); err != nil || n < 1 || n > 24 {
				return nil, fmt.Errorf("không biết phím %q", part)
			}
			vks = append(vks, uint16(0x70+n-1))
		default:
			return nil, fmt.Errorf("không biết phím %q", part)
		}
	}
	return vks, nil
}

// Các trình thông dịch có thể chạy lệnh tuỳ ý; AI không được mở chúng.
var blockedApps = map[string]bool{
	"cmd": true, "powershell": true, "pwsh": true, "wscript": true, "cscript": true,
	"mshta": true, "rundll32": true, "regsvr32": true, "wt": true, "bash": true, "wsl": true,
	"reg": true, "regedit": true, "msiexec": true, "schtasks": true, "sc": true, "net": true,
}

// checkAppName kiểm tra tên ứng dụng AI muốn mở.
func checkAppName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("thiếu tên ứng dụng")
	}
	if strings.ContainsAny(name, `/\"<>|&;`) || strings.Contains(name, "..") {
		return fmt.Errorf("tên ứng dụng không hợp lệ: %q", name)
	}
	base := strings.TrimSuffix(strings.ToLower(name), ".exe")
	if blockedApps[base] {
		return fmt.Errorf("không được phép mở %q", name)
	}
	if strings.Contains(name, ":") && !strings.HasPrefix(strings.ToLower(name), "ms-settings:") {
		return fmt.Errorf("tên ứng dụng không hợp lệ: %q", name)
	}
	return nil
}
