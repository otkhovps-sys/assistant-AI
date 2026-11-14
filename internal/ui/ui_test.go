package ui

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/otkhovps-sys/assistant-AI/internal/assistant"
)

func TestServer(t *testing.T) {
	s, err := Start(nil)
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + s.ln.Addr().String()

	// Thiếu hoặc sai mã bí mật thì bị từ chối.
	for _, u := range []string{base + "/", base + "/events?t=wrong"} {
		resp, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s = %d, muốn 403", u, resp.StatusCode)
		}
	}
	resp, _ := http.Post(base+"/send", "application/json", strings.NewReader(`{"text":"x"}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST /send không mã = %d, muốn 403", resp.StatusCode)
	}

	// Cửa sổ mới mở nhận lại trạng thái gần nhất.
	s.Emit(assistant.Event{Type: "status", State: "ready"})
	ev, err := http.Get(base + "/events?t=" + s.token)
	if err != nil {
		t.Fatal(err)
	}
	rd := bufio.NewReader(ev.Body)
	line, _ := rd.ReadString('\n')
	if !strings.Contains(line, `"state":"ready"`) {
		t.Errorf("dòng đầu = %q", line)
	}

	// Lệnh gõ đi tới kênh Lines; nút micro đổi Muted.
	resp, _ = http.Post(base+"/send?t="+s.token, "application/json", strings.NewReader(`{"text":" mở notepad "}`))
	resp.Body.Close()
	select {
	case got := <-s.Lines():
		if got != "mở notepad" {
			t.Errorf("Lines = %q", got)
		}
	case <-time.After(time.Second):
		t.Error("không nhận được lệnh")
	}
	resp, _ = http.Post(base+"/mute?t="+s.token, "application/json", strings.NewReader(`{"muted":true}`))
	resp.Body.Close()
	if !s.Muted().Load() {
		t.Error("Muted phải là true")
	}

	// Đóng cửa sổ cuối cùng thì Closed báo sau vài giây.
	ev.Body.Close()
	select {
	case <-s.Closed():
	case <-time.After(6 * time.Second):
		t.Error("Closed không báo sau khi cửa sổ đóng")
	}
}
