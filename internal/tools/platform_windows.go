//go:build windows

package tools

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"google.golang.org/genai"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procSendInput       = user32.NewProc("SendInput")
	procLockWorkStation = user32.NewProc("LockWorkStation")
)

const (
	inputKeyboard    = 1
	keyeventfKeyUp   = 0x0002
	keyeventfUnicode = 0x0004
	swShowNormal     = 1
)

// keyInput khớp với cấu trúc INPUT của Win32 khi type = INPUT_KEYBOARD.
// Struct lồng nhau để phần union được căn lề theo con trỏ như trong C
// (tổng 40 byte trên 64-bit, 28 byte trên 32-bit).
type keyInput struct {
	typ uint32
	ki  struct {
		vk    uint16
		scan  uint16
		flags uint32
		time  uint32
		extra uintptr
	}
	_ [8]byte
}

func key(vk, scan uint16, flags uint32) keyInput {
	in := keyInput{typ: inputKeyboard}
	in.ki.vk, in.ki.scan, in.ki.flags = vk, scan, flags
	return in
}

func send(inputs []keyInput) error {
	if len(inputs) == 0 {
		return nil
	}
	n, _, err := procSendInput.Call(uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	if int(n) != len(inputs) {
		return fmt.Errorf("SendInput: %w", err)
	}
	return nil
}

// pressCombo nhấn giữ các phím theo thứ tự rồi nhả theo thứ tự ngược lại.
func pressCombo(vks []uint16) error {
	var in []keyInput
	for _, vk := range vks {
		in = append(in, key(vk, 0, 0))
	}
	for i := len(vks) - 1; i >= 0; i-- {
		in = append(in, key(vks[i], 0, keyeventfKeyUp))
	}
	return send(in)
}

func tap(vk uint16, times int) error {
	for i := 0; i < times; i++ {
		if err := pressCombo([]uint16{vk}); err != nil {
			return err
		}
	}
	return nil
}

func shellOpen(target string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, swShowNormal)
}

var processNameRe = regexp.MustCompile(`^[\w.\- ]+$`)

func platformTools(opts Options) []Tool {
	return []Tool{
		{
			Decl: &genai.FunctionDeclaration{
				Name:        "open_app",
				Description: "Mở một ứng dụng trên Windows theo tên, ví dụ notepad, chrome, excel, calc, explorer, hoặc một trang cài đặt như ms-settings:display.",
				Parameters:  obj(map[string]*genai.Schema{"name": strProp("Tên ứng dụng hoặc URI ms-settings:")}, "name"),
			},
			Run: func(_ context.Context, a map[string]any) (string, error) {
				name := strings.TrimSpace(str(a, "name"))
				if err := checkAppName(name); err != nil {
					return "", err
				}
				if err := shellOpen(name); err != nil {
					return "", fmt.Errorf("không mở được %q: %w", name, err)
				}
				return "đã mở " + name, nil
			},
		},
		{
			Decl: &genai.FunctionDeclaration{
				Name:        "open_url",
				Description: "Mở một địa chỉ web http/https bằng trình duyệt mặc định.",
				Parameters:  obj(map[string]*genai.Schema{"url": strProp("Địa chỉ đầy đủ, bắt đầu bằng https://")}, "url"),
			},
			Run: func(_ context.Context, a map[string]any) (string, error) {
				u, err := url.Parse(str(a, "url"))
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					return "", fmt.Errorf("chỉ mở được địa chỉ http/https")
				}
				if err := shellOpen(u.String()); err != nil {
					return "", err
				}
				return "đã mở " + u.String(), nil
			},
		},
		{
			Decl: &genai.FunctionDeclaration{
				Name:        "set_volume",
				Description: "Đặt âm lượng hệ thống theo phần trăm, hoặc bật/tắt tiếng.",
				Parameters: obj(map[string]*genai.Schema{
					"level": {Type: genai.TypeInteger, Description: "Âm lượng 0-100. Bỏ trống nếu chỉ bật/tắt tiếng."},
					"mute":  {Type: genai.TypeBoolean, Description: "true để đảo trạng thái tắt tiếng."},
				}),
			},
			Run: func(_ context.Context, a map[string]any) (string, error) {
				if m, _ := a["mute"].(bool); m {
					return "đã đảo trạng thái tắt tiếng", tap(vkVolumeMute, 1)
				}
				f, ok := a["level"].(float64)
				if !ok || f < 0 || f > 100 {
					return "", fmt.Errorf("level phải từ 0 đến 100")
				}
				// Mỗi lần nhấn phím âm lượng đổi 2%: hạ về 0 rồi tăng lên mức cần.
				if err := tap(vkVolumeDown, 50); err != nil {
					return "", err
				}
				if err := tap(vkVolumeUp, int(f+1)/2); err != nil {
					return "", err
				}
				return fmt.Sprintf("âm lượng khoảng %d%%", int(f+1)/2*2), nil
			},
		},
		{
			Decl: &genai.FunctionDeclaration{
				Name:        "media_control",
				Description: "Điều khiển nhạc/video đang phát.",
				Parameters:  obj(map[string]*genai.Schema{"action": strProp("Hành động", "play_pause", "next", "previous", "stop")}, "action"),
			},
			Run: func(_ context.Context, a map[string]any) (string, error) {
				vk, ok := map[string]uint16{"play_pause": vkMediaPlay, "next": vkMediaNext,
					"previous": vkMediaPrev, "stop": vkMediaStop}[str(a, "action")]
				if !ok {
					return "", fmt.Errorf("action không hợp lệ")
				}
				return "xong", tap(vk, 1)
			},
		},
		{
			Decl: &genai.FunctionDeclaration{
				Name:        "press_keys",
				Description: "Nhấn một tổ hợp phím trên cửa sổ đang mở, ví dụ ctrl+s, alt+tab, win+d, alt+f4, f5.",
				Parameters:  obj(map[string]*genai.Schema{"combo": strProp("Các phím nối bằng dấu +")}, "combo"),
			},
			Run: func(_ context.Context, a map[string]any) (string, error) {
				vks, err := parseCombo(str(a, "combo"))
				if err != nil {
					return "", err
				}
				return "đã nhấn " + str(a, "combo"), pressCombo(vks)
			},
		},
		{
			Decl: &genai.FunctionDeclaration{
				Name:        "type_text",
				Description: "Gõ một đoạn văn bản vào cửa sổ đang được chọn.",
				Parameters:  obj(map[string]*genai.Schema{"text": strProp("Văn bản cần gõ")}, "text"),
			},
			Run: func(_ context.Context, a map[string]any) (string, error) {
				var in []keyInput
				for _, u := range utf16.Encode([]rune(str(a, "text"))) {
					in = append(in, key(0, u, keyeventfUnicode), key(0, u, keyeventfUnicode|keyeventfKeyUp))
				}
				return "đã gõ", send(in)
			},
		},
		{
			Decl: &genai.FunctionDeclaration{
				Name:        "close_app",
				Description: "Đóng một ứng dụng theo tên tiến trình, ví dụ notepad hoặc chrome. Ứng dụng vẫn có thể hỏi lưu dữ liệu.",
				Parameters:  obj(map[string]*genai.Schema{"process": strProp("Tên tiến trình")}, "process"),
			},
			Run: func(ctx context.Context, a map[string]any) (string, error) {
				name := strings.TrimSpace(str(a, "process"))
				if !processNameRe.MatchString(name) {
					return "", fmt.Errorf("tên tiến trình không hợp lệ")
				}
				if !strings.HasSuffix(strings.ToLower(name), ".exe") {
					name += ".exe"
				}
				// Không dùng /F để ứng dụng tự đóng và kịp hỏi lưu.
				out, err := exec.CommandContext(ctx, "taskkill", "/IM", name).CombinedOutput()
				if err != nil {
					return "", fmt.Errorf("không đóng được %s: %s", name, strings.TrimSpace(string(out)))
				}
				return "đã gửi yêu cầu đóng " + name, nil
			},
		},
		{
			Decl: &genai.FunctionDeclaration{
				Name:        "power",
				Description: "Khoá màn hình, cho máy ngủ, tắt máy hoặc khởi động lại. Tắt máy và khởi động lại diễn ra sau 30 giây và huỷ được bằng cancel_shutdown.",
				Parameters:  obj(map[string]*genai.Schema{"action": strProp("Hành động", "lock", "sleep", "shutdown", "restart", "cancel_shutdown")}, "action"),
			},
			Run: func(ctx context.Context, a map[string]any) (string, error) {
				action := str(a, "action")
				switch action {
				case "lock":
					if r, _, err := procLockWorkStation.Call(); r == 0 {
						return "", err
					}
					return "đã khoá màn hình", nil
				case "sleep":
					// Chờ một chút để câu trả lời kịp phát trước khi máy ngủ.
					go func() {
						time.Sleep(3 * time.Second)
						_ = exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0").Run()
					}()
					return "máy sẽ ngủ sau 3 giây", nil
				case "cancel_shutdown":
					return "đã huỷ", exec.CommandContext(ctx, "shutdown", "/a").Run()
				case "shutdown", "restart":
					if !opts.AllowPower {
						return "", fmt.Errorf("tắt máy/khởi động lại đang bị khoá; chạy app với -allow-power để bật")
					}
					flag := "/s"
					if action == "restart" {
						flag = "/r"
					}
					return "sẽ thực hiện sau 30 giây", exec.CommandContext(ctx, "shutdown", flag, "/t", "30").Run()
				}
				return "", fmt.Errorf("action không hợp lệ")
			},
		},
	}
}
