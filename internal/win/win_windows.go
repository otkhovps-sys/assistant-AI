//go:build windows

// Package win quản lý cửa sổ trên Windows: cửa sổ của trợ lý và cửa sổ các ứng dụng khác.
package win

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	pEnumWindows      = user32.NewProc("EnumWindows")
	pGetWindowTextW   = user32.NewProc("GetWindowTextW")
	pIsWindowVisible  = user32.NewProc("IsWindowVisible")
	pSetWindowPos     = user32.NewProc("SetWindowPos")
	pGetForeground    = user32.NewProc("GetForegroundWindow")
	pSetForeground    = user32.NewProc("SetForegroundWindow")
	pShowWindow       = user32.NewProc("ShowWindow")
	pIsIconic         = user32.NewProc("IsIconic")
	pSystemParameters = user32.NewProc("SystemParametersInfoW")
	pGetWindowRect    = user32.NewProc("GetWindowRect")
	pKeybdEvent       = user32.NewProc("keybd_event")
)

const (
	hwndTopmost    = ^uintptr(0) // -1
	hwndNoTopmost  = ^uintptr(1) // -2
	swpNoSize      = 0x0001
	swpNoMove      = 0x0002
	swpNoZOrder    = 0x0004
	swpNoActivate  = 0x0010
	swRestore      = 9
	spiGetWorkArea = 0x0030
	vkMenu         = 0x12
	keyUp          = 0x0002
)

type rect struct{ Left, Top, Right, Bottom int32 }

// Window là một cửa sổ đang hiện trên màn hình.
type Window struct {
	Handle uintptr
	Title  string
}

var (
	self atomic.Uintptr // cửa sổ của trợ lý
	last atomic.Uintptr // cửa sổ ứng dụng khác mà người dùng dùng gần nhất

	enumMu   sync.Mutex
	enumOut  []Window
	enumOnce sync.Once
	enumCB   uintptr
)

func title(h uintptr) string {
	buf := make([]uint16, 512)
	n, _, _ := pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func all() []Window {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumOnce.Do(func() {
		enumCB = windows.NewCallback(func(h, _ uintptr) uintptr {
			if v, _, _ := pIsWindowVisible.Call(h); v != 0 {
				if t := title(h); t != "" {
					enumOut = append(enumOut, Window{h, t})
				}
			}
			return 1
		})
	})
	enumOut = nil
	pEnumWindows.Call(enumCB, 0)
	return enumOut
}

// List trả về các cửa sổ đang hiện, không kể cửa sổ của trợ lý.
func List() []Window {
	var out []Window
	for _, w := range all() {
		if w.Handle != self.Load() && w.Title != "Program Manager" {
			out = append(out, w)
		}
	}
	return out
}

// Adopt tìm cửa sổ của trợ lý theo tiêu đề, đặt nó ở góc dưới phải màn hình và cho nổi trên các ứng dụng khác.
// Hàm chờ tối đa nửa phút vì trình duyệt cần thời gian mở cửa sổ.
func Adopt(titlePart string, width, height int) {
	go trackForeground()
	for i := 0; i < 150; i++ {
		for _, w := range all() {
			if strings.Contains(w.Title, titlePart) {
				self.Store(w.Handle)
				last.CompareAndSwap(w.Handle, 0)
				var wa rect
				pSystemParameters.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
				x, y := int(wa.Right)-width-16, int(wa.Bottom)-height-16
				pSetWindowPos.Call(w.Handle, hwndTopmost, uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpNoActivate)
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// trackForeground ghi nhớ cửa sổ ứng dụng mà người dùng đang làm việc, để thao tác phím và chuột
// đi tới đó chứ không rơi vào cửa sổ của trợ lý.
func trackForeground() {
	for range time.Tick(250 * time.Millisecond) {
		if fg, _, _ := pGetForeground.Call(); fg != 0 && fg != self.Load() {
			last.Store(fg)
		}
	}
}

// SetTopmost bật hoặc tắt việc cửa sổ trợ lý nổi trên các ứng dụng khác.
func SetTopmost(on bool) {
	h := self.Load()
	if h == 0 {
		return
	}
	z := hwndNoTopmost
	if on {
		z = hwndTopmost
	}
	pSetWindowPos.Call(h, z, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

func activate(h uintptr) {
	if min, _, _ := pIsIconic.Call(h); min != 0 {
		pShowWindow.Call(h, swRestore)
	}
	// Windows chỉ cho đổi cửa sổ đang chọn ngay sau một thao tác phím; nhấn nhả Alt để được phép.
	pKeybdEvent.Call(vkMenu, 0, 0, 0)
	pKeybdEvent.Call(vkMenu, 0, keyUp, 0)
	pSetForeground.Call(h)
	last.Store(h)
	time.Sleep(250 * time.Millisecond)
}

// LeaveSelf chuyển về ứng dụng người dùng dùng gần nhất nếu cửa sổ trợ lý đang được chọn.
func LeaveSelf() {
	fg, _, _ := pGetForeground.Call()
	if h := last.Load(); fg == self.Load() && h != 0 {
		activate(h)
	}
}

// Focus đưa cửa sổ có tiêu đề chứa titlePart lên trước và trả về tiêu đề đầy đủ của nó.
func Focus(titlePart string) (string, error) {
	want := strings.ToLower(strings.TrimSpace(titlePart))
	for _, w := range List() {
		if want != "" && strings.Contains(strings.ToLower(w.Title), want) {
			activate(w.Handle)
			return w.Title, nil
		}
	}
	return "", fmt.Errorf("không thấy cửa sổ nào có tiêu đề chứa %q", titlePart)
}

func selfRect() (rect, bool) {
	var r rect
	h := self.Load()
	if h == 0 {
		return r, false
	}
	ok, _, _ := pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return r, ok != 0
}

func moveSelf(x, y int32) {
	pSetWindowPos.Call(self.Load(), 0, uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
}

// HideForCapture tạm đưa cửa sổ trợ lý ra ngoài màn hình để nó không che ảnh chụp; gọi hàm trả về để đưa lại.
func HideForCapture() (restore func()) {
	r, ok := selfRect()
	if !ok {
		return func() {}
	}
	moveSelf(-32000, -32000)
	time.Sleep(150 * time.Millisecond)
	return func() { moveSelf(r.Left, r.Top) }
}

// Avoid dời cửa sổ trợ lý sang phía bên kia màn hình nếu nó đang che điểm (x, y).
func Avoid(x, y int) {
	r, ok := selfRect()
	if !ok || int32(x) < r.Left || int32(x) > r.Right || int32(y) < r.Top || int32(y) > r.Bottom {
		return
	}
	var wa rect
	pSystemParameters.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
	nx := wa.Left + 16
	if r.Left < (wa.Left+wa.Right)/2 {
		nx = wa.Right - (r.Right - r.Left) - 16
	}
	moveSelf(nx, r.Top)
	time.Sleep(150 * time.Millisecond)
}
