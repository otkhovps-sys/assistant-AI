//go:build !windows

package ui

import "os/exec"

// OpenWindow mở giao diện bằng trình duyệt mặc định.
func OpenWindow(url string) error { return exec.Command("xdg-open", url).Start() }
