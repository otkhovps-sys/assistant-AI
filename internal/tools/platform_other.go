//go:build !windows

package tools

// Các hàm điều khiển máy chỉ có trên Windows.
func platformTools(Options) []Tool { return nil }
