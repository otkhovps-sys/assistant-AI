// Lệnh assistant là trợ lý AI điều khiển Windows bằng giọng nói hoặc gõ chữ.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"

	"github.com/otkhovps-sys/assistant-AI/internal/assistant"
	"github.com/otkhovps-sys/assistant-AI/internal/audio"
	"github.com/otkhovps-sys/assistant-AI/internal/tools"
	"github.com/otkhovps-sys/assistant-AI/internal/ui"
)

func main() {
	loadDotEnv(".env")

	model := flag.String("model", envOr("GEMINI_MODEL", "gemini-3.8-live"), "model Gemini Live")
	voice := flag.String("voice", os.Getenv("GEMINI_VOICE"), "tên giọng đọc (bỏ trống = mặc định)")
	textOnly := flag.Bool("text", false, "chỉ gõ chữ, không dùng micro và loa")
	cli := flag.Bool("cli", false, "chạy trong terminal, không mở cửa sổ giao diện")
	fullDuplex := flag.Bool("full-duplex", false, "giữ micro mở khi trợ lý đang nói để ngắt lời được (nên dùng tai nghe)")
	allowPower := flag.Bool("allow-power", false, "cho phép trợ lý tắt máy và khởi động lại")
	flag.Parse()
	log.SetFlags(log.Ltime)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg := assistant.Config{
		Model:      *model,
		Voice:      *voice,
		FullDuplex: *fullDuplex,
		Tools:      tools.New(tools.Options{AllowPower: *allowPower}),
	}

	// fail báo lỗi khiến app không chạy tiếp được: ra terminal, hoặc lên cửa sổ rồi chờ người dùng đóng.
	fail := func(msg string) { log.Fatal(msg) }
	if *cli {
		cfg.Lines = stdinLines()
		cfg.Events = printEvents()
	} else {
		// Giao diện mở trước micro/loa để lỗi âm thanh cũng hiện được trên cửa sổ.
		var dev atomic.Pointer[audio.IO]
		srv, err := ui.Start(func() (float32, float32, bool) {
			if a := dev.Load(); a != nil {
				return a.Levels()
			}
			return 0, 0, false
		})
		if err != nil {
			log.Fatalf("mở giao diện: %v", err)
		}
		cfg.Lines, cfg.Events, cfg.Muted = srv.Lines(), srv.Emit, srv.Muted()
		fail = func(msg string) {
			log.Print(msg)
			srv.Emit(assistant.Event{Type: "status", State: "error", Message: msg})
			select {
			case <-srv.Closed():
			case <-ctx.Done():
			}
			os.Exit(1)
		}
		go func() { <-srv.Closed(); stop() }()
		log.Printf("giao diện: %s", srv.URL())
		if err := ui.OpenWindow(srv.URL()); err != nil {
			log.Printf("không tự mở được cửa sổ (%v); hãy mở địa chỉ trên bằng trình duyệt", err)
		}
		if !*textOnly {
			a, err := audio.Open()
			if err != nil {
				fail(err.Error())
			}
			defer a.Close()
			cfg.Audio = a
			dev.Store(a)
		}
	}

	if os.Getenv("GEMINI_API_KEY") == "" && os.Getenv("GOOGLE_API_KEY") == "" {
		fail("Thiếu GEMINI_API_KEY: đặt biến môi trường hoặc ghi vào file .env")
	}
	if *cli && !*textOnly {
		a, err := audio.Open()
		if err != nil {
			log.Fatalf("%v (chạy với -text nếu máy không có micro/loa)", err)
		}
		defer a.Close()
		cfg.Audio = a
	}
	if *cli {
		log.Print("nói hoặc gõ lệnh rồi Enter (gõ /quit để thoát)")
	}

	if err := assistant.Run(ctx, cfg); err != nil {
		fail(err.Error())
	}
}

// stdinLines đọc lệnh gõ từ terminal; kênh đóng khi hết đầu vào.
func stdinLines() <-chan string {
	out := make(chan string)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			out <- sc.Text()
		}
		close(out)
	}()
	return out
}

// printEvents in hội thoại ra terminal, gom các mẩu cùng người nói trên một dòng.
func printEvents() func(assistant.Event) {
	who := ""
	flush := func() {
		if who != "" {
			fmt.Println()
			who = ""
		}
	}
	return func(e assistant.Event) {
		switch e.Type {
		case "text":
			if who != e.Who {
				flush()
				who = e.Who
				fmt.Print(map[string]string{"user": "Bạn: ", "ai": "AI: "}[who])
			}
			fmt.Print(e.Text)
		case "turn", "tool":
			flush()
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadDotEnv nạp các dòng KEY=VALUE từ file, không ghi đè biến đã có.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set && v != "" {
			os.Setenv(k, v)
		}
	}
}
