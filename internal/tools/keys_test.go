package tools

import (
	"context"
	"reflect"
	"testing"
)

func TestParseCombo(t *testing.T) {
	cases := map[string][]uint16{
		"ctrl+s":         {0x11, 'S'},
		"Alt + Tab":      {0x12, 0x09},
		"win+d":          {0x5B, 'D'},
		"ctrl+shift+esc": {0x11, 0x10, 0x1B},
		"f5":             {0x74},
		"alt+f4":         {0x12, 0x73},
		"1":              {'1'},
	}
	for in, want := range cases {
		got, err := parseCombo(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("parseCombo(%q) = %v, %v; muốn %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ctrl+", "foo", "f25", "ctrl+ab"} {
		if _, err := parseCombo(bad); err == nil {
			t.Errorf("parseCombo(%q) phải lỗi", bad)
		}
	}
}

func TestCheckAppName(t *testing.T) {
	for _, ok := range []string{"notepad", "chrome", "calc.exe", "ms-settings:display", "excel"} {
		if err := checkAppName(ok); err != nil {
			t.Errorf("checkAppName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "cmd", "PowerShell.exe", `C:\x\evil.exe`, "a & b", "file:x", "..", "http://x"} {
		if err := checkAppName(bad); err == nil {
			t.Errorf("checkAppName(%q) phải lỗi", bad)
		}
	}
}

func TestRegistryCall(t *testing.T) {
	r := New(Options{})
	if res := r.Call(context.Background(), "get_time", nil); res["result"] == nil {
		t.Errorf("get_time = %v", res)
	}
	if res := r.Call(context.Background(), "nope", nil); res["error"] == nil {
		t.Errorf("hàm lạ phải trả lỗi, được %v", res)
	}
}
