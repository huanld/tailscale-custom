# Hướng dẫn Custom Tailscale Client

## Mục lục
1. [Tổng quan](#tổng-quan)
2. [Danh sách files đã thay đổi](#danh-sách-files-đã-thay-đổi)
3. [Chi tiết từng thay đổi](#chi-tiết-từng-thay-đổi)
4. [Build Windows Client](#build-windows-client)
5. [Build Linux Client](#build-linux-client)
6. [Tạo MSI Installer (Windows)](#tạo-msi-installer-windows)
7. [Tray App (Windows)](#tray-app-windows)
8. [Cách deploy lên máy mới](#cách-deploy-lên-máy-mới)

---

## Tổng quan

Custom Tailscale Client là bản fork từ Tailscale v1.97.0, được tuỳ chỉnh để:
- **Chạy song song** với Tailscale chính thức trên cùng 1 máy
- **Kết nối Headscale** tại `https://vpn.softs.business` (default)
- **Không xung đột** service name, named pipe, registry, TUN adapter, thư mục dữ liệu

### Namespace isolation (tránh xung đột)

| Thành phần | Tailscale gốc | Tailscale-Custom |
|------------|---------------|------------------|
| Service name | `Tailscale` | `Tailscale-Custom` |
| Named pipe | `Tailscale\tailscaled` | `Tailscale-Custom\tailscaled` |
| Registry | `SOFTWARE\Tailscale IPN` | `SOFTWARE\Tailscale-Custom IPN` |
| Policy registry | `Policies\Tailscale` | `Policies\Tailscale-Custom` |
| ProgramData | `C:\ProgramData\Tailscale` | `C:\ProgramData\Tailscale-Custom` |
| LocalAppData | `...\Tailscale` | `...\Tailscale-Custom` |
| TUN adapter | `Tailscale` | `Tailscale-Custom` |
| TUN GUID | `{37217669-...}` | `{47317669-...}` |
| Control URL | `https://controlplane.tailscale.com` | `https://vpn.softs.business` |

---

## Danh sách files đã thay đổi

### Nhóm 1: Đổi tên service / đường dẫn (Coexistence)

| File | Thay đổi |
|------|----------|
| `cmd/tailscaled/tailscaled_windows.go` | `serviceName = "Tailscale-Custom"` |
| `cmd/tailscaled/tailscaled.go` | `defaultTunName() = "Tailscale-Custom"` |
| `paths/paths.go` | Named pipe path, state file path |
| `paths/paths_windows.go` | ACL directory check |
| `util/winutil/winutil_windows.go` | Registry keys |
| `util/syspolicy/source/policy_store_windows.go` | Policy registry paths |
| `net/tstun/tun_windows.go` | WintunTunnelType + GUID |
| `logpolicy/logpolicy.go` | Log directories |
| `log/filelogger/log.go` | Log file path |
| `clientupdate/clientupdate_windows.go` | Update cache path |
| `envknob/envknob.go` | Env file path |
| `ipn/auditlog/store.go` | Audit log path |

### Nhóm 2: Đổi Control URL

| File | Thay đổi |
|------|----------|
| `ipn/prefs.go` | `DefaultControlURL = "https://vpn.softs.business"` |
| `ipn/conf.go` | Default URL trong comment |
| `control/controlclient/direct.go` | Server URL references |
| `cmd/tailscale/cli/debug.go` | Debug command default |
| `cmd/tailscaled/debug.go` | Debug URL |
| `net/netmon/state.go` | Login endpoint |
| `net/captivedetection/endpoints.go` | Captive portal check |
| `util/winutil/policy/policy_windows.go` | Policy default URL |
| `util/syspolicy/syspolicy.go` | System policy default |

### Nhóm 3: Thêm mới

| File | Mô tả |
|------|-------|
| `cmd/tailscale-tray/main.go` | Windows system tray app |
| `cmd/tailscale-tray/icon.go` | Icon generation |
| `installer/TailscaleCustom.wxs` | WiX v5 MSI installer |

---

## Chi tiết từng thay đổi

### 1. `ipn/prefs.go` - Control URL mặc định
```go
// TRƯỚC:
const DefaultControlURL = "https://controlplane.tailscale.com"

// SAU:
const DefaultControlURL = "https://vpn.softs.business"
```

Thêm `vpn.softs.business` vào `IsLoginServerSynonym()` để Tailscale nhận diện đây là server hợp lệ.

### 2. `cmd/tailscaled/tailscaled_windows.go` - Service name
```go
// TRƯỚC:
const serviceName = "Tailscale"

// SAU:
const serviceName = "Tailscale-Custom"
```

### 3. `cmd/tailscaled/tailscaled.go` - TUN device name
```go
// TRƯỚC (trong defaultTunName()):
case "windows": return "Tailscale"

// SAU:
case "windows": return "Tailscale-Custom"
```

### 4. `paths/paths.go` - Named pipe & state paths
```go
// TRƯỚC:
DefaultTailscaledSocket = `\\.\pipe\ProtectedPrefix\Administrators\Tailscale\tailscaled`
stateFileInProgramData  = `Tailscale\server-state.conf`

// SAU:
DefaultTailscaledSocket = `\\.\pipe\ProtectedPrefix\Administrators\Tailscale-Custom\tailscaled`
stateFileInProgramData  = `Tailscale-Custom\server-state.conf`
```

### 5. `net/tstun/tun_windows.go` - TUN adapter
```go
// TRƯỚC:
tun.WintunTunnelType = "Tailscale"
guid, _ := windows.GUIDFromString("{37217669-42da-4657-a55b-0d995d328250}")

// SAU:
tun.WintunTunnelType = "Tailscale-Custom"
guid, _ := windows.GUIDFromString("{47317669-42da-4657-a55b-0d995d328250}")
```

> **Quan trọng**: GUID phải khác để 2 TUN adapter cùng tồn tại.

### 6. `util/winutil/winutil_windows.go` - Registry
```go
// TRƯỚC:
regBase       = `SOFTWARE\Tailscale IPN`
regPolicyBase = `SOFTWARE\Policies\Tailscale`

// SAU:
regBase       = `SOFTWARE\Tailscale-Custom IPN`
regPolicyBase = `SOFTWARE\Policies\Tailscale-Custom`
```

### 7. `control/controlclient/direct.go` - Server validation
Cập nhật các URL reference từ `controlplane.tailscale.com` sang `vpn.softs.business`.

### 8. `logpolicy/logpolicy.go` - Log paths
```go
// TRƯỚC:
dir = filepath.Join(os.Getenv("ProgramData"), "Tailscale", "Logs")

// SAU:
dir = filepath.Join(os.Getenv("ProgramData"), "Tailscale-Custom", "Logs")
```

---

## Build Windows Client

### Yêu cầu
- Go 1.26+ (hoặc dùng `.\tool\go.exe` trong repo)
- GCC/MinGW (cho CGO, cần cho tray app)

### Build CLI + Daemon
```powershell
cd <repo_root>

# Build tailscale.exe (CLI)
.\tool\go.exe build -o .\dist\tailscale.exe `
  -ldflags "-X tailscale.com/version.longStamp=1.97.176-custom -X tailscale.com/version.shortStamp=1.97.176-custom" `
  tailscale.com/cmd/tailscale

# Build tailscaled.exe (daemon/service)
.\tool\go.exe build -o .\dist\tailscaled.exe `
  -ldflags "-X tailscale.com/version.longStamp=1.97.176-custom -X tailscale.com/version.shortStamp=1.97.176-custom" `
  tailscale.com/cmd/tailscaled
```

### Build Tray App
```powershell
# Cần CGO_ENABLED=1 (cho systray) và -H=windowsgui (ẩn console)
$env:CGO_ENABLED="1"
.\tool\go.exe build -o .\dist\tailscale-tray.exe `
  -ldflags "-H=windowsgui" `
  tailscale.com/cmd/tailscale-tray
```

### Cần thêm `wintun.dll`
Copy `wintun.dll` từ:
- https://www.wintun.net/ (tải chính thức)
- Hoặc từ `C:\Program Files\Tailscale\wintun.dll` nếu đã cài Tailscale gốc

```powershell
Copy-Item "C:\Program Files\Tailscale\wintun.dll" .\dist\
```

---

## Build Linux Client

### Build CLI + Daemon
```bash
cd <repo_root>

# Lấy Go toolchain
./tool/go build -o ./dist/tailscale \
  -ldflags "-X tailscale.com/version.longStamp=1.97.176-custom -X tailscale.com/version.shortStamp=1.97.176-custom" \
  tailscale.com/cmd/tailscale

./tool/go build -o ./dist/tailscaled \
  -ldflags "-X tailscale.com/version.longStamp=1.97.176-custom -X tailscale.com/version.shortStamp=1.97.176-custom" \
  tailscale.com/cmd/tailscaled
```

### Cài đặt trên Linux
```bash
# Copy binaries
sudo cp dist/tailscale /usr/local/bin/
sudo cp dist/tailscaled /usr/local/bin/

# Tạo systemd service
sudo cat > /etc/systemd/system/tailscale-custom.service << 'EOF'
[Unit]
Description=Tailscale-Custom VPN daemon
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/tailscaled --state=/var/lib/tailscale-custom/tailscaled.state --socket=/var/run/tailscale-custom/tailscaled.sock
Restart=on-failure
RuntimeDirectory=tailscale-custom
StateDirectory=tailscale-custom

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now tailscale-custom
```

### Kết nối
```bash
sudo tailscale up --login-server https://vpn.softs.business --accept-dns=false \
  --socket /var/run/tailscale-custom/tailscaled.sock
```

> **Lưu ý Linux**: Trên Linux, custom Tailscale dùng `--socket` và `--state` khác path để tránh xung đột với Tailscale gốc. Không cần đổi tên service vì Linux dùng socket file thay vì named pipe.

---

## Tạo MSI Installer (Windows)

### Yêu cầu
- .NET SDK
- WiX Toolset v5: `dotnet tool install --global wix`
- WiX Util extension: `wix extension add WixToolset.Util.wixext`

### Cấu trúc thư mục installer
```
dist/
├── tailscale.exe       # CLI
├── tailscaled.exe      # Daemon/service
├── tailscale-tray.exe  # System tray app
└── wintun.dll          # TUN driver

installer/
└── TailscaleCustom.wxs # WiX definition
```

### Build MSI
```powershell
wix build -o .\dist\TailscaleCustom.msi `
  -ext WixToolset.Util.wixext `
  .\installer\TailscaleCustom.wxs
```

### MSI bao gồm:
- Cài `tailscaled.exe` làm Windows Service (auto-start)
- Cài `tailscale.exe` (CLI) và thêm vào PATH
- Cài `tailscale-tray.exe` với auto-start qua Registry
- Cài `wintun.dll` (TUN driver)
- Shortcuts trên Desktop và Start Menu
- UpgradeCode riêng: `E1F2A3B4-C5D6-4E7F-8A9B-0C1D2E3F4A5B`

---

## Tray App (Windows)

### Tính năng
- **System tray icon**: Xanh = kết nối, xám = ngắt kết nối
- **Status**: Hiển thị trạng thái, IP, hostname
- **Connect / Disconnect**: Bật/tắt VPN
- **Account**: Submenu hiển thị tất cả profiles, chuyển đổi giữa các server
- **Add Server**: Dialog nhập URL server mới → tạo profile mới → mở trình duyệt login
- **Single instance**: Chỉ cho chạy 1 instance qua Windows Mutex

### Kiến trúc code
```
cmd/tailscale-tray/
├── main.go   # App logic, systray menu, Win32 dialogs
└── icon.go   # ICO generation (16x16, green/gray)
```

**Pattern**: Dùng per-item `onClick` goroutine (giống official Tailscale systray):
```go
func onClick(ctx context.Context, item *systray.MenuItem, fn func()) {
    go func() {
        for {
            select {
            case <-ctx.Done(): return
            case <-item.ClickedCh: fn()
            }
        }
    }()
}
```

### Dependency
- `fyne.io/systray` - Cross-platform system tray
- `golang.org/x/sys/windows` - Win32 API
- `tailscale.com/client/local` - IPC client đến tailscaled

---

## Cách deploy lên máy mới

### Windows - MSI Installer
```powershell
# Cài đặt (silent)
msiexec /i TailscaleCustom.msi /qn

# Kết nối (mở cmd/powershell)
tailscale up --login-server https://vpn.softs.business --accept-dns=false

# Hoặc dùng pre-auth key (tự động, không cần approve trên server)
tailscale up --login-server https://vpn.softs.business --accept-dns=false --authkey <KEY>
```

### Linux - Manual install
```bash
# Copy binaries
sudo cp tailscale tailscaled /usr/local/bin/

# Tạo service (xem phần Build Linux ở trên)
sudo systemctl enable --now tailscale-custom

# Kết nối
sudo tailscale up --login-server https://vpn.softs.business --accept-dns=false \
  --socket /var/run/tailscale-custom/tailscaled.sock

# Hoặc dùng pre-auth key
sudo tailscale up --login-server https://vpn.softs.business --accept-dns=false \
  --authkey <KEY> --socket /var/run/tailscale-custom/tailscaled.sock
```

### Quan trọng: `--accept-dns=false`
Luôn dùng flag này để tránh Tailscale ghi đè DNS config của máy, gây mất mạng internet.

---

## TODO cho phiên bản tiếp theo

### Web Admin App (Docker)
- [ ] Dashboard: tổng nodes, online/offline
- [ ] Nodes list: approve/reject/delete
- [ ] Users management: CRUD
- [ ] Pre-auth keys: tạo/quản lý
- [ ] Dùng Headscale REST API (xem `HEADSCALE_SETUP.md`)

### Hoàn thiện Linux Client
- [ ] Build script cho Linux (amd64, arm64)
- [ ] Tạo `.deb` / `.rpm` package
- [ ] Systemd service file đóng gói sẵn
- [ ] Test coexistence với Tailscale gốc trên Linux

### Hoàn thiện Windows Client
- [ ] Code signing cho exe/msi
- [ ] Auto-update mechanism
- [ ] Tray app: hiển thị danh sách peers
- [ ] Tray app: copy IP khi click
