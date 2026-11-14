// Package tools định nghĩa các hàm mà AI được phép gọi.
package tools

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

// Options điều chỉnh những hành động được phép.
type Options struct {
	// AllowPower cho phép tắt máy và khởi động lại.
	AllowPower bool
	// Confirm hỏi người dùng trước một hành động nguy hiểm và trả về true nếu họ đồng ý.
	// Nếu nil, các hành động cần xác nhận bị từ chối.
	Confirm func(ctx context.Context, title, detail string) bool
	// Vision bật các hàm chụp màn hình và bấm chuột, dùng khi model nhìn được ảnh.
	Vision bool
}

// Tool là một hàm AI có thể gọi.
type Tool struct {
	Decl *genai.FunctionDeclaration
	Run  func(ctx context.Context, args map[string]any) (string, error)
	// Timeout là thời gian tối đa cho một lần chạy; 0 nghĩa là 15 giây.
	Timeout time.Duration
}

// Registry tra hàm theo tên.
type Registry struct {
	tools map[string]Tool
	order []string

	mu    sync.Mutex
	image []byte // ảnh JPEG mà hàm vừa chạy muốn đưa cho model xem
}

// SetImage để lại một ảnh JPEG cho model xem sau lượt gọi hàm hiện tại.
func (r *Registry) SetImage(jpeg []byte) {
	r.mu.Lock()
	r.image = jpeg
	r.mu.Unlock()
}

// TakeImage lấy ảnh mà hàm vừa chạy để lại, nếu có.
func (r *Registry) TakeImage() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	img := r.image
	r.image = nil
	return img
}

// New tạo registry gồm các hàm chung và các hàm của hệ điều hành hiện tại.
func New(opts Options) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	r.Add(Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "get_time",
			Description: "Lấy ngày giờ hiện tại trên máy của người dùng.",
		},
		Run: func(context.Context, map[string]any) (string, error) {
			return time.Now().Format("Monday 2006-01-02 15:04:05 MST"), nil
		},
	})
	r.Add(Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "wait",
			Description: "Chờ vài giây, ví dụ để ứng dụng vừa mở kịp hiện lên trước bước tiếp theo.",
			Parameters:  obj(map[string]*genai.Schema{"seconds": {Type: genai.TypeNumber, Description: "Số giây, tối đa 10"}}, "seconds"),
		},
		Run: func(ctx context.Context, a map[string]any) (string, error) {
			sec, _ := a["seconds"].(float64)
			sec = min(max(sec, 0.1), 10)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(sec * float64(time.Second))):
			}
			return fmt.Sprintf("đã chờ %.1f giây", sec), nil
		},
	})
	r.Add(commandTool(opts))
	for _, t := range platformTools(opts, r) {
		r.Add(t)
	}
	return r
}

// Add đăng ký một hàm.
func (r *Registry) Add(t Tool) {
	if _, ok := r.tools[t.Decl.Name]; !ok {
		r.order = append(r.order, t.Decl.Name)
	}
	r.tools[t.Decl.Name] = t
}

// Declarations trả về khai báo hàm để gửi cho Gemini.
func (r *Registry) Declarations() []*genai.FunctionDeclaration {
	out := make([]*genai.FunctionDeclaration, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name].Decl)
	}
	return out
}

// Call chạy một hàm và trả về kết quả ở dạng Gemini nhận được.
func (r *Registry) Call(ctx context.Context, name string, args map[string]any) map[string]any {
	t, ok := r.tools[name]
	if !ok {
		return map[string]any{"error": fmt.Sprintf("không có hàm %q", name)}
	}
	timeout := t.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := t.Run(ctx, args)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"result": res}
}

func str(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

func obj(props map[string]*genai.Schema, required ...string) *genai.Schema {
	return &genai.Schema{Type: genai.TypeObject, Properties: props, Required: required}
}

func strProp(desc string, enum ...string) *genai.Schema {
	return &genai.Schema{Type: genai.TypeString, Description: desc, Enum: enum}
}

// commandTool cho AI chạy một lệnh shell, nhưng chỉ sau khi người dùng xem lệnh và đồng ý.
func commandTool(opts Options) Tool {
	shell := "PowerShell"
	if runtime.GOOS != "windows" {
		shell = "sh"
	}
	return Tool{
		Timeout: 3 * time.Minute,
		Decl: &genai.FunctionDeclaration{
			Name: "run_command",
			Description: "Chạy một lệnh " + shell + " trên máy và trả về kết quả in ra. Dùng cho việc không có hàm riêng: " +
				"thao tác file và thư mục, xem thông tin hệ thống, cài đặt. Người dùng sẽ thấy lệnh và phải bấm đồng ý trước khi nó chạy.",
			Parameters: obj(map[string]*genai.Schema{
				"command": strProp("Lệnh cần chạy"),
				"reason":  strProp("Một câu ngắn giải thích lệnh này làm gì, bằng ngôn ngữ của người dùng"),
			}, "command", "reason"),
		},
		Run: func(ctx context.Context, a map[string]any) (string, error) {
			command := strings.TrimSpace(str(a, "command"))
			if command == "" {
				return "", fmt.Errorf("thiếu lệnh")
			}
			if opts.Confirm == nil {
				return "", fmt.Errorf("chế độ này không hỏi được người dùng nên không chạy lệnh")
			}
			if !opts.Confirm(ctx, str(a, "reason"), command) {
				return "", fmt.Errorf("người dùng không đồng ý chạy lệnh này")
			}
			ctx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "-c", command)
			if runtime.GOOS == "windows" {
				cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
					"[Console]::OutputEncoding=[Text.Encoding]::UTF8; "+command)
			}
			hideConsole(cmd)
			out, err := cmd.CombinedOutput()
			text := strings.TrimSpace(string(out))
			if len(text) > 4000 {
				text = text[:4000] + "\n… (đã cắt bớt)"
			}
			if ctx.Err() != nil {
				return "", fmt.Errorf("lệnh chạy quá một phút nên đã bị dừng. Kết quả tới lúc đó: %s", text)
			}
			if err != nil {
				return "", fmt.Errorf("lệnh lỗi (%v): %s", err, text)
			}
			if text == "" {
				text = "(lệnh chạy xong, không in ra gì)"
			}
			return text, nil
		},
	}
}
