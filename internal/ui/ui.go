// Package ui phục vụ giao diện hình cầu qua một máy chủ web chỉ nghe trên máy này.
package ui

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/otkhovps-sys/assistant-AI/internal/assistant"
)

//go:embed index.html
var indexHTML []byte

// Levels đọc độ lớn micro, độ lớn loa và việc loa có đang phát.
type Levels func() (mic, out float32, speaking bool)

// Server là máy chủ giao diện.
type Server struct {
	token  string
	ln     net.Listener
	levels Levels
	lines  chan string
	muted  atomic.Bool
	closed chan struct{}

	mu        sync.Mutex
	clients   map[chan []byte]struct{}
	sticky    map[string][]byte // sự kiện gần nhất theo loại, gửi lại cho cửa sổ mới mở
	closeOnce sync.Once
}

// Start mở máy chủ trên 127.0.0.1. Cổng cố định được ưu tiên để trình duyệt nhớ quyền dùng micro
// giữa các lần chạy; nếu cổng bận thì dùng cổng ngẫu nhiên.
func Start(levels Levels) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:47821")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return nil, err
	}
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	s := &Server{
		token:   hex.EncodeToString(tok),
		ln:      ln,
		levels:  levels,
		lines:   make(chan string, 8),
		closed:  make(chan struct{}),
		clients: map[chan []byte]struct{}{},
		sticky:  map[string][]byte{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.auth(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(indexHTML)
	}))
	mux.HandleFunc("GET /events", s.auth(s.handleEvents))
	mux.HandleFunc("POST /send", s.auth(s.handleSend))
	mux.HandleFunc("POST /mute", s.auth(s.handleMute))
	go http.Serve(ln, mux)
	if levels != nil {
		go s.pumpLevels()
	}
	return s, nil
}

// URL là địa chỉ mở giao diện, kèm mã bí mật.
func (s *Server) URL() string { return fmt.Sprintf("http://%s/?t=%s", s.ln.Addr(), s.token) }

// Lines là các lệnh người dùng gõ trong giao diện.
func (s *Server) Lines() <-chan string { return s.lines }

// Muted là trạng thái tắt micro do người dùng chọn.
func (s *Server) Muted() *atomic.Bool { return &s.muted }

// Closed đóng lại khi người dùng đã đóng cửa sổ giao diện.
func (s *Server) Closed() <-chan struct{} { return s.closed }

// Emit gửi một diễn biến của trợ lý tới giao diện.
func (s *Server) Emit(e assistant.Event) {
	b, _ := json.Marshal(e)
	if e.Type == "status" {
		s.setSticky("status", b)
	}
	s.broadcast(b)
}

// UseBrowserSpeech bảo giao diện tự nhận giọng nói và đọc câu trả lời bằng tính năng có sẵn của trình duyệt.
// Dùng khi máy chủ AI chỉ trao đổi bằng chữ.
func (s *Server) UseBrowserSpeech() {
	b := []byte(`{"type":"config","speech":"browser"}`)
	s.setSticky("config", b)
	s.broadcast(b)
}

func (s *Server) setSticky(kind string, b []byte) {
	s.mu.Lock()
	s.sticky[kind] = b
	s.mu.Unlock()
}

// auth chỉ cho qua yêu cầu mang đúng mã bí mật, để trang web khác trên máy không ra lệnh được cho trợ lý.
func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func (s *Server) broadcast(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		select {
		case c <- b:
		default: // cửa sổ nhận chậm: bỏ bớt
		}
	}
}

func (s *Server) pumpLevels() {
	for range time.Tick(33 * time.Millisecond) {
		s.mu.Lock()
		n := len(s.clients)
		s.mu.Unlock()
		if n == 0 {
			continue
		}
		mic, out, speaking := s.levels()
		b, _ := json.Marshal(map[string]any{"type": "level", "mic": mic, "out": out, "speaking": speaking})
		s.broadcast(b)
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")

	c := make(chan []byte, 128)
	s.mu.Lock()
	s.clients[c] = struct{}{}
	replay := [][]byte{s.sticky["config"], s.sticky["status"]}
	s.mu.Unlock()
	defer s.dropClient(c)

	write := func(b []byte) { fmt.Fprintf(w, "data: %s\n\n", b); fl.Flush() }
	for _, b := range replay {
		if b != nil {
			write(b)
		}
	}
	write(fmt.Appendf(nil, `{"type":"muted","muted":%t}`, s.muted.Load()))
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-c:
			write(b)
		}
	}
}

// dropClient gỡ một cửa sổ; nếu sau vài giây không còn cửa sổ nào thì coi như người dùng đã đóng app.
func (s *Server) dropClient(c chan []byte) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
	time.AfterFunc(3*time.Second, func() {
		s.mu.Lock()
		gone := len(s.clients) == 0
		s.mu.Unlock()
		if gone {
			s.closeOnce.Do(func() { close(s.closed) })
		}
	})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var body struct{ Text string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if text := strings.TrimSpace(body.Text); text != "" {
		select {
		case s.lines <- text:
			s.Emit(assistant.Event{Type: "text", Who: "user", Text: text})
		default:
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMute(w http.ResponseWriter, r *http.Request) {
	var body struct{ Muted bool }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.muted.Store(body.Muted)
	s.broadcast(fmt.Appendf(nil, `{"type":"muted","muted":%t}`, body.Muted))
	w.WriteHeader(http.StatusNoContent)
}
