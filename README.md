# assistant-AI

Trợ lý AI viết bằng Go, điều khiển Windows bằng giọng nói hoặc gõ chữ, dùng Gemini Live API.

App chỉ thu micro và phát loa. Nhận dạng giọng, trả lời, đọc ra tiếng, gọi hàm và tra Google đều do Gemini xử lý qua một kết nối WebSocket.

## Cài đặt

Cần Go 1.24 trở lên và một trình biên dịch C (thư viện âm thanh `malgo` dùng cgo). Trên Windows, cài MinGW-w64, ví dụ qua [MSYS2](https://www.msys2.org/) hoặc `winget install BrechtSanders.WinLibs.POSIX.UCRT`, rồi đảm bảo `gcc` có trong `PATH`.

```bash
go build -o assistant.exe ./cmd/assistant
```

Lấy API key tại <https://aistudio.google.com/apikey>, chép `.env.example` thành `.env` và điền `GEMINI_API_KEY`.

## Chạy

```bash
./assistant.exe
```

Nói vào micro, hoặc gõ lệnh rồi Enter. Gõ `/quit` hoặc Ctrl+C để thoát.

| Cờ | Ý nghĩa |
|---|---|
| `-text` | Chỉ gõ chữ, không dùng micro và loa |
| `-full-duplex` | Giữ micro mở khi trợ lý đang nói để ngắt lời được. Nên dùng tai nghe, nếu không trợ lý sẽ nghe lại chính nó |
| `-allow-power` | Cho phép tắt máy và khởi động lại |
| `-model` | Model Live, mặc định `gemini-3.8-live` (hoặc biến `GEMINI_MODEL`) |
| `-voice` | Tên giọng đọc (hoặc biến `GEMINI_VOICE`) |

## Trợ lý làm được gì

| Hàm | Việc |
|---|---|
| `open_app` | Mở ứng dụng theo tên, hoặc trang cài đặt `ms-settings:` |
| `open_url` | Mở trang web bằng trình duyệt mặc định |
| `set_volume` | Đặt âm lượng, bật/tắt tiếng |
| `media_control` | Phát/dừng, bài kế, bài trước |
| `press_keys` | Nhấn tổ hợp phím như `ctrl+s`, `alt+tab`, `win+d` |
| `type_text` | Gõ văn bản vào cửa sổ đang chọn |
| `close_app` | Đóng ứng dụng theo tên tiến trình |
| `power` | Khoá màn hình, ngủ, tắt máy, khởi động lại |
| `get_time` | Ngày giờ hiện tại |
| Google Search | Tra cứu thông tin, có sẵn trong Gemini |

Giới hạn an toàn: trợ lý không chạy được lệnh tuỳ ý, không mở được `cmd`, `powershell` và các trình thông dịch tương tự, và chỉ tắt máy khi có cờ `-allow-power`.

## Cấu trúc

```
cmd/assistant        điểm vào, đọc cờ và .env
internal/assistant   phiên Gemini Live: gửi âm thanh/chữ, nhận trả lời, gọi hàm, tự kết nối lại
internal/audio       thu micro 16 kHz, phát loa 24 kHz
internal/tools       các hàm AI được gọi; platform_windows.go chứa phần điều khiển Windows
```

Thêm hàm mới: khai báo một `Tool` trong `internal/tools/platform_windows.go`.
