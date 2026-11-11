// Lệnh assistant là trợ lý AI điều khiển Windows bằng giọng nói hoặc gõ chữ.
package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/otkhovps-sys/assistant-AI/internal/assistant"
	"github.com/otkhovps-sys/assistant-AI/internal/audio"
	"github.com/otkhovps-sys/assistant-AI/internal/tools"
)

func main() {
	loadDotEnv(".env")

	model := flag.String("model", envOr("GEMINI_MODEL", "gemini-3.8-live"), "model Gemini Live")
	voice := flag.String("voice", os.Getenv("GEMINI_VOICE"), "tên giọng đọc (bỏ trống = mặc định)")
	textOnly := flag.Bool("text", false, "chỉ gõ chữ, không dùng micro và loa")
	fullDuplex := flag.Bool("full-duplex", false, "giữ micro mở khi trợ lý đang nói để ngắt lời được (nên dùng tai nghe)")
	allowPower := flag.Bool("allow-power", false, "cho phép trợ lý tắt máy và khởi động lại")
	flag.Parse()
	log.SetFlags(log.Ltime)

	if os.Getenv("GEMINI_API_KEY") == "" && os.Getenv("GOOGLE_API_KEY") == "" {
		log.Fatal("thiếu GEMINI_API_KEY: đặt biến môi trường hoặc ghi vào file .env")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg := assistant.Config{
		Model:      *model,
		Voice:      *voice,
		FullDuplex: *fullDuplex,
		Tools:      tools.New(tools.Options{AllowPower: *allowPower}),
		Input:      os.Stdin,
	}
	if !*textOnly {
		a, err := audio.Open()
		if err != nil {
			log.Fatalf("%v (chạy với -text nếu máy không có micro/loa)", err)
		}
		defer a.Close()
		cfg.Audio = a
	}

	if err := assistant.Run(ctx, cfg); err != nil {
		log.Fatal(err)
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
