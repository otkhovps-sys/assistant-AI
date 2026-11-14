//go:build windows

package ui

import (
	"os"
	"os/exec"
	"path/filepath"
)

// OpenWindow mở giao diện trong một cửa sổ riêng không có thanh địa chỉ (Edge ở chế độ app),
// hoặc bằng trình duyệt mặc định nếu không tìm thấy Edge.
func OpenWindow(url string) error {
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
		edge := filepath.Join(os.Getenv(env), "Microsoft", "Edge", "Application", "msedge.exe")
		if _, err := os.Stat(edge); err == nil {
			return exec.Command(edge, "--app="+url, "--window-size=380,620").Start()
		}
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
