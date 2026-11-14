//go:build !windows

// Package win quản lý cửa sổ; ngoài Windows các hàm này không làm gì.
package win

import "errors"

type Window struct {
	Handle uintptr
	Title  string
}

func List() []Window                   { return nil }
func Adopt(string, int, int)           {}
func SetTopmost(bool)                  {}
func LeaveSelf()                       {}
func Focus(string) (string, error)     { return "", errors.New("chỉ hỗ trợ trên Windows") }
func HideForCapture() (restore func()) { return func() {} }
func Avoid(int, int)                   {}
