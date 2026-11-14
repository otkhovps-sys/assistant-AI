// Package audio thu micro và phát loa bằng miniaudio (malgo).
package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
)

const (
	// Định dạng Gemini Live yêu cầu: PCM 16-bit little-endian, mono.
	InputRate  = 16000
	OutputRate = 24000
)

// IO giữ thiết bị thu và phát.
type IO struct {
	ctx      *malgo.AllocatedContext
	capture  *malgo.Device
	playback *malgo.Device

	// Mic là các khối PCM 16 kHz thu được. Khối bị bỏ nếu bên nhận quá chậm.
	Mic chan []byte

	mu        sync.Mutex
	out       []byte
	lastSound time.Time

	// Độ lớn âm thanh gần nhất (RMS 0..1), lưu dạng bit của float32 cho giao diện đọc.
	micLevel, outLevel atomic.Uint32
}

// rms tính độ lớn trung bình của một khối PCM 16-bit little-endian.
func rms(pcm []byte) float32 {
	n := len(pcm) / 2
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[2*i:]))) / 32768
		sum += v * v
	}
	return float32(math.Sqrt(sum / float64(n)))
}

// Levels trả về độ lớn micro, độ lớn loa và việc loa có đang phát hay không.
func (a *IO) Levels() (mic, out float32, speaking bool) {
	return math.Float32frombits(a.micLevel.Load()), math.Float32frombits(a.outLevel.Load()), a.Speaking()
}

// Open mở micro và loa mặc định của hệ thống.
func Open() (*IO, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("khởi tạo âm thanh: %w", err)
	}
	a := &IO{ctx: ctx, Mic: make(chan []byte, 64)}

	capCfg := malgo.DefaultDeviceConfig(malgo.Capture)
	capCfg.Capture.Format = malgo.FormatS16
	capCfg.Capture.Channels = 1
	capCfg.SampleRate = InputRate
	a.capture, err = malgo.InitDevice(ctx.Context, capCfg, malgo.DeviceCallbacks{
		Data: func(_, in []byte, _ uint32) {
			chunk := make([]byte, len(in))
			copy(chunk, in)
			a.micLevel.Store(math.Float32bits(rms(in)))
			select {
			case a.Mic <- chunk:
			default:
			}
		},
	})
	if err != nil {
		a.Close()
		return nil, fmt.Errorf("mở micro: %w", err)
	}

	playCfg := malgo.DefaultDeviceConfig(malgo.Playback)
	playCfg.Playback.Format = malgo.FormatS16
	playCfg.Playback.Channels = 1
	playCfg.SampleRate = OutputRate
	a.playback, err = malgo.InitDevice(ctx.Context, playCfg, malgo.DeviceCallbacks{
		Data: func(out, _ []byte, _ uint32) { a.fill(out) },
	})
	if err != nil {
		a.Close()
		return nil, fmt.Errorf("mở loa: %w", err)
	}

	if err := a.capture.Start(); err != nil {
		a.Close()
		return nil, fmt.Errorf("bật micro: %w", err)
	}
	if err := a.playback.Start(); err != nil {
		a.Close()
		return nil, fmt.Errorf("bật loa: %w", err)
	}
	return a, nil
}

func (a *IO) fill(out []byte) {
	a.mu.Lock()
	n := copy(out, a.out)
	a.out = a.out[n:]
	if n > 0 {
		a.lastSound = time.Now()
	}
	a.mu.Unlock()
	a.outLevel.Store(math.Float32bits(rms(out[:n])))
	clear(out[n:])
}

// Play xếp PCM 24 kHz vào hàng đợi phát.
func (a *IO) Play(pcm []byte) {
	a.mu.Lock()
	a.out = append(a.out, pcm...)
	a.mu.Unlock()
}

// Flush bỏ phần âm thanh chưa phát (khi user ngắt lời).
func (a *IO) Flush() {
	a.mu.Lock()
	a.out = nil
	a.mu.Unlock()
}

// Speaking cho biết loa đang phát, tính cả một khoảng đuôi ngắn để tiếng vang tắt hẳn.
func (a *IO) Speaking() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.out) > 0 || time.Since(a.lastSound) < 300*time.Millisecond
}

// Close giải phóng thiết bị.
func (a *IO) Close() {
	if a.capture != nil {
		a.capture.Uninit()
	}
	if a.playback != nil {
		a.playback.Uninit()
	}
	if a.ctx != nil {
		_ = a.ctx.Uninit()
		a.ctx.Free()
	}
}
