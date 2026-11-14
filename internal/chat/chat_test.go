package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
