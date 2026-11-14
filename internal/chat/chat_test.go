package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/otkhovps-sys/assistant-AI/internal/assistant"
	"github.com/otkhovps-sys/assistant-AI/internal/tools"
)

// Máy chủ giả: lượt đầu yêu cầu gọi hàm (stream SSE, tham số chia nhiều mẩu), lượt hai trả lời bằng JSON thường.
func TestRunToolRoundTrip(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, `{"error":{"message":"sai đường dẫn hoặc key"}}`, 401)
			return
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		switch len(bodies) {
		case 1:
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, `data: {"choices":[{"delta":{"role":"assistant"}}]}`+"\n\n")
			fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"text\":"}}]}}]}`+"\n\n")
			fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"xin chào\"}"}}]}}]}`+"\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
		case 2:
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Xong rồi."}}]}`)
		default:
			w.WriteHeader(500)
			fmt.Fprint(w, `{"error":{"message":"hết lượt"}}`)
		}
	}))
	defer srv.Close()

	reg := tools.New(tools.Options{})
	var gotArg string
	reg.Add(tools.Tool{
		Decl: &genai.FunctionDeclaration{Name: "echo", Parameters: &genai.Schema{Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{"text": {Type: genai.TypeString}}, Required: []string{"text"}}},
		Run: func(_ context.Context, a map[string]any) (string, error) {
			gotArg, _ = a["text"].(string)
			return "ok", nil
		},
	})

	lines := make(chan string, 3)
	lines <- "nói xin chào"
	lines <- "câu thứ hai"
	close(lines)
	var events []assistant.Event
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := Run(ctx, Config{BaseURL: srv.URL + "/v1/", APIKey: "k", Model: "m", Tools: reg, Lines: lines,
		Events: func(e assistant.Event) { events = append(events, e) }})
	if err != nil {
		t.Fatal(err)
	}

	if gotArg != "xin chào" {
		t.Errorf("tham số hàm = %q", gotArg)
	}
	var summary []string
	for _, e := range events {
		summary = append(summary, strings.TrimSpace(e.Type+" "+e.State+e.Name+e.Text+e.Message))
	}
	want := "status ready|tool echo|text Xong rồi.|turn|error máy chủ AI báo lỗi: hết lượt|turn"
	if got := strings.Join(summary, "|"); got != want {
		t.Errorf("sự kiện =\n %s\nmuốn\n %s", got, want)
	}

	// Lượt hai phải gửi kèm lượt gọi hàm và kết quả của nó.
	msgs := bodies[1]["messages"].([]any)
	roles := ""
	for _, m := range msgs {
		roles += m.(map[string]any)["role"].(string) + " "
	}
	if roles != "system user assistant tool " {
		t.Errorf("vai trò lượt hai = %q", roles)
	}
	if id := msgs[3].(map[string]any)["tool_call_id"]; id != "c1" {
		t.Errorf("tool_call_id = %v", id)
	}
	// Câu bị lỗi không được nằm lại trong lịch sử; khai báo hàm phải đúng dạng OpenAI.
	tool0 := bodies[0]["tools"].([]any)[0].(map[string]any)
	if tool0["type"] != "function" || tool0["function"].(map[string]any)["parameters"].(map[string]any)["type"] != "object" {
		t.Errorf("khai báo hàm = %v", tool0)
	}
}

// Ảnh do hàm để lại phải đi kèm trong một tin nhắn riêng; nút Dừng phải huỷ được yêu cầu đang chạy.
func TestImagesAndInterrupt(t *testing.T) {
	var second map[string]any
	n := 0
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Để mình xem màn hình.","tool_calls":[{"id":"c1","type":"function","function":{"name":"snap","arguments":"{}"}}]}}]}`)
		case 2:
			json.NewDecoder(r.Body).Decode(&second)
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Thấy rồi."}}]}`)
		default:
			select { // yêu cầu thứ hai treo cho tới khi bị dừng
			case <-r.Context().Done():
			case <-release:
			}
		}
	}))
	defer srv.Close()
	defer close(release)

	reg := tools.New(tools.Options{})
	reg.Add(tools.Tool{Decl: &genai.FunctionDeclaration{Name: "snap"},
		Run: func(context.Context, map[string]any) (string, error) {
			reg.SetImage([]byte("JPEGDATA"))
			return "ok", nil
		}})

	lines := make(chan string, 2)
	interrupt := make(chan struct{}, 1)
	lines <- "màn hình đang có gì"
	lines <- "việc lâu"
	var got []string
	var once sync.Once
	done := make(chan error)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		done <- Run(ctx, Config{BaseURL: srv.URL, APIKey: "k", Model: "m", Tools: reg, Lines: lines, Interrupt: interrupt,
			Events: func(e assistant.Event) {
				got = append(got, strings.TrimSpace(e.Type+" "+e.Name+e.Text+e.Message))
				if e.Type == "turn" { // yêu cầu đầu đã xong: dừng yêu cầu thứ hai rồi thoát
					once.Do(func() {
						go func() { time.Sleep(200 * time.Millisecond); interrupt <- struct{}{}; close(lines) }()
					})
				}
			}})
	}()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	want := "status|text Để mình xem màn hình.|say|tool snap|text Thấy rồi.|turn|error đã dừng theo yêu cầu|turn"
	if s := strings.Join(got, "|"); s != want {
		t.Errorf("sự kiện =\n %s\nmuốn\n %s", s, want)
	}
	msgs := second["messages"].([]any)
	img := msgs[len(msgs)-1].(map[string]any)
	parts, _ := img["content"].([]any)
	if img["role"] != "user" || len(parts) != 2 ||
		parts[1].(map[string]any)["image_url"].(map[string]any)["url"] != "data:image/jpeg;base64,SlBFR0RBVEE=" {
		t.Errorf("tin nhắn ảnh = %v", img)
	}
}

func TestDropOldImages(t *testing.T) {
	var h []message
	for i := 0; i < 4; i++ {
		h = append(h, message{"role": "user", "content": []any{"img"}}, message{"role": "assistant", "content": "x"})
	}
	dropOldImages(h)
	kept := 0
	for _, m := range h {
		if _, ok := m["content"].([]any); ok {
			kept++
		}
	}
	if _, newest := h[6]["content"].([]any); kept != keepImages || !newest {
		t.Errorf("còn %d ảnh, ảnh mới nhất còn: %v", kept, newest)
	}
}

func TestTrim(t *testing.T) {
	var h []message
	for i := 0; i < 30; i++ {
		h = append(h, message{"role": "user"}, message{"role": "assistant"}, message{"role": "tool"})
	}
	got := trim(h)
	if len(got) > maxHistory || got[0]["role"] != "user" {
		t.Errorf("trim: dài %d, bắt đầu bằng %v", len(got), got[0]["role"])
	}
}
