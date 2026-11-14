# assistant-AI

Trợ lý AI viết bằng Go, điều khiển Windows bằng giọng nói hoặc gõ chữ, dùng Gemini Live API. Giao diện là một hình cầu chuyển động theo giọng nói.

App chỉ thu micro và phát loa. Nhận dạng giọng, trả lời, đọc ra tiếng, gọi hàm và tra Google đều do Gemini xử lý qua một kết nối WebSocket.

## Cài đặt

Cần Go 1.24 trở lên và một trình biên dịch C (thư viện âm thanh `malgo` dùng cgo). Trên Windows, cài MinGW-w64, ví dụ qua [MSYS2](https://www.msys2.org/) hoặc `winget install BrechtSanders.WinLibs.POSIX.UCRT`, rồi đảm bảo `gcc` có trong `PATH`.

```bash
go build -ldflags "-H=windowsgui" -o assistant.exe ./cmd/assistant
```

`-H=windowsgui` ẩn cửa sổ terminal khi mở app. Bỏ cờ này nếu muốn xem log.

Lấy API key tại <https://aistudio.google.com/apikey>, chép `.env.example` thành `.env` và điền `GEMINI_API_KEY`.

## Hai cách kết nối AI

| | Gemini Live | API tương thích OpenAI |
|---|---|---|
| Bật bằng | `GEMINI_API_KEY` | `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL` |
| Nghe và đọc | Gemini xử lý, thời gian thực, ngắt lời được | Cửa sổ Edge nhận giọng nói thành chữ và đọc câu trả lời |
| Tra Google | Có sẵn trong Gemini | Mở trang kết quả Google bằng trình duyệt |
| Độ trễ | Thấp | Vài giây mỗi câu |

Ở cách thứ hai, app chỉ gửi và nhận chữ qua `/chat/completions` kèm gọi hàm, nên dùng được với mọi máy chủ tương thích OpenAI có hỗ trợ tools. Lần đầu mở cần bấm vào cửa sổ một lần và cho phép dùng micro. Nhận giọng nói cần Edge hoặc Chrome; chế độ `-cli` ở cách này chỉ gõ chữ.

## Chạy

```bash
./assistant.exe
```

App mở một cửa sổ nhỏ có hình cầu ở giữa. Nói vào micro, hoặc gõ lệnh vào ô bên dưới. Đóng cửa sổ là app thoát.

- Hình cầu trắng, gợn nhẹ theo giọng bạn: đang nghe.
- Hình cầu xanh, gợn theo giọng trợ lý: đang nói.
- Hình cầu xám: đang kết nối hoặc đã tắt micro (nút micro ở góc dưới trái).
- Hình cầu đỏ kèm dòng chữ: có lỗi, ví dụ thiếu API key.

Cửa sổ nhỏ này nằm ở góc dưới phải màn hình và luôn nổi trên các ứng dụng khác; nút ghim ở góc trên phải (hoặc cờ `-no-top`) tắt việc đó. Khi trợ lý đang làm một yêu cầu, hình cầu chuyển tím và nút gửi đổi thành nút Dừng.

Cửa sổ là Microsoft Edge ở chế độ app (có sẵn trên Windows 10/11), hiển thị một trang do app phục vụ trên `127.0.0.1`. Nếu máy không có Edge, trang mở bằng trình duyệt mặc định.

| Cờ | Ý nghĩa |
|---|---|
| `-cli` | Chạy trong terminal, không mở cửa sổ (gõ `/quit` hoặc Ctrl+C để thoát) |
| `-text` | Chỉ gõ chữ, không dùng micro và loa |
| `-full-duplex` | Giữ micro mở khi trợ lý đang nói để ngắt lời được. Nên dùng tai nghe, nếu không trợ lý sẽ nghe lại chính nó |
| `-no-top` | Không cho cửa sổ nổi trên các ứng dụng khác |
| `-allow-power` | Cho phép tắt máy và khởi động lại |
| `-base-url` | Địa chỉ API tương thích OpenAI (hoặc biến `OPENAI_BASE_URL`) |
| `-model` | Tên model (hoặc biến `OPENAI_MODEL` / `GEMINI_MODEL`, mặc định `gemini-3.8-live`) |
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
| `list_windows`, `focus_window` | Xem các cửa sổ đang mở, chọn một cửa sổ để thao tác |
| `wait` | Chờ vài giây cho ứng dụng kịp mở |
| `run_command` | Chạy một lệnh PowerShell, sau khi bạn xem lệnh và bấm Cho phép trên cửa sổ |
| `screenshot`, `mouse_click`, `mouse_scroll` | Xem màn hình và dùng chuột (chỉ ở chế độ API tương thích OpenAI, model cần nhìn được ảnh) |
| `get_time` | Ngày giờ hiện tại |
| Google Search | Tra cứu thông tin, có sẵn trong Gemini |

### Yêu cầu nhiều bước

Trợ lý tự làm lần lượt nhiều bước cho một yêu cầu, ví dụ "mở Notepad, gõ danh sách việc hôm nay rồi lưu ra Desktop", tối đa 30 lượt gọi hàm. Nó kiểm tra kết quả bằng ảnh chụp màn hình và dùng `run_command` cho việc không có hàm riêng. Khi chụp màn hình, cửa sổ trợ lý tạm lánh ra ngoài để không che nội dung; phím bấm và chữ gõ luôn đi tới ứng dụng bạn dùng gần nhất chứ không rơi vào cửa sổ trợ lý.

### Giới hạn an toàn

- Mỗi lệnh `run_command` hiện nguyên văn trên cửa sổ và chỉ chạy khi bạn bấm Cho phép; không trả lời trong một phút được tính là từ chối. Ở chế độ `-cli` lệnh luôn bị từ chối.
- `open_app` không mở được `cmd`, `powershell` và các trình thông dịch tương tự, để không lách qua bước xác nhận.
- Tắt máy và khởi động lại chỉ chạy khi có cờ `-allow-power`.
- Bấm chuột, nhấn phím và gõ chữ không hỏi xác nhận. Hãy theo dõi khi giao việc và dùng nút Dừng nếu cần.

## Cấu trúc

```
cmd/assistant        điểm vào, đọc cờ và .env
internal/ui          giao diện hình cầu: máy chủ web cục bộ và trang index.html
internal/assistant   phiên Gemini Live: gửi âm thanh/chữ, nhận trả lời, gọi hàm, tự kết nối lại
internal/chat        chế độ API tương thích OpenAI: chat completions, gọi hàm
internal/win         quản lý cửa sổ Windows: nổi trên cùng, chọn cửa sổ, lánh khi chụp màn hình
internal/audio       thu micro 16 kHz, phát loa 24 kHz
internal/tools       các hàm AI được gọi; platform_windows.go chứa phần điều khiển Windows
```

Thêm hàm mới: khai báo một `Tool` trong `internal/tools/platform_windows.go`.
