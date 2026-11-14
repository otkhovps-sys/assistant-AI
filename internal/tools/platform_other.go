//go:build !windows

package tools

import "os/exec"

// Các hàm điều khiển máy chỉ có trên Windows.
func platformTools(Options, *Registry) []Tool { return nil }

func hideConsole(*exec.Cmd) {}
