// Package tools định nghĩa các hàm mà AI được phép gọi.
package tools

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genai"
)

// Options điều chỉnh những hành động được phép.
type Options struct {
	// AllowPower cho phép tắt máy và khởi động lại.
	AllowPower bool
}

// Tool là một hàm AI có thể gọi.
type Tool struct {
	Decl *genai.FunctionDeclaration
	Run  func(ctx context.Context, args map[string]any) (string, error)
}

// Registry tra hàm theo tên.
type Registry struct {
	tools map[string]Tool
	order []string
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
	for _, t := range platformTools(opts) {
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
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
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
