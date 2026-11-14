//go:build windows

package ui

import (
	"os"
	"os/exec"
	"path/filepath"
)

// OpenWindow mở giao diện trong một cửa sổ riêng không có thanh địa chỉ, bằng Edge hoặc Chrome ở chế độ app.
// Trình duyệt chạy với một hồ sơ riêng của trợ lý, nên cửa sổ không bị gộp vào các tab hay cửa sổ
// trình duyệt người dùng đang mở, và có biểu tượng riêng trên thanh tác vụ.
// Nếu không tìm thấy trình duyệt nào thì mở bằng trình duyệt mặc định.
func OpenWindow(url string) error {
	candidates := []struct{ env, path string }{
		{"ProgramFiles(x86)", `Microsoft\Edge\Application\msedge.exe`},
		{"ProgramFiles", `Microsoft\Edge\Application\msedge.exe`},
		{"LocalAppData", `Microsoft\Edge\Application\msedge.exe`},
		{"ProgramFiles", `Google\Chrome\Application\chrome.exe`},
		{"ProgramFiles(x86)", `Google\Chrome\Application\chrome.exe`},
		{"LocalAppData", `Google\Chrome\Application\chrome.exe`},
	}
	for _, c := range candidates {
		root := os.Getenv(c.env)
		if root == "" {
			continue
		}
		browser := filepath.Join(root, c.path)
		if _, err := os.Stat(browser); err != nil {
			continue
		}
		args := []string{"--app=" + url, "--window-size=380,620", "--no-first-run", "--no-default-browser-check"}
		if dir, err := profileDir(); err == nil {
			args = append(args, "--user-data-dir="+dir)
		}
		return exec.Command(browser, args...).Start()
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// profileDir trả về thư mục hồ sơ trình duyệt riêng của trợ lý, tạo nếu chưa có.
// Quyền dùng micro đã cấp được nhớ trong hồ sơ này.
func profileDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "assistant-AI", "browser")
	return dir, os.MkdirAll(dir, 0o700)
}
