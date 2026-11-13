# SMART PRINT AGENT (Golang Windows Daemon)

Jembatan hardware percetakan berkecepatan tinggi, anti-hang, dan berjalan di **Windows System Tray** murni di latar belakang (background) untuk SIMRS.

Dirancang khusus untuk menggantikan aplikasi lama berbasis VB6 (Desktop Service Lite).

---

## 🎯 3 Masalah yang Terselesaikan Secara Tuntas

1. **Tanpa Jendela Terminal (Pure Background GUI)**:
   - Di-compile dengan flag `-H=windowsgui`.
   - Saat dijalankan (di-double-click), **TIDAK ADA jendela hitam/terminal CMD yang terbuka**. Aplikasi berjalan senyap di background.

2. **Windows System Tray Icon (Pojok Kanan Bawah)**:
   - Ikon agen muncul di area notifikasi Windows System Tray (dekat jam).
   - **Klik Kiri / Double-Click**: Langsung membuka halaman Dashboard Pengaturan Printer di browser (`http://127.0.0.1:1237/settings`).
   - **Klik Kanan**: Menampilkan menu popup konteks:
     - ⚙️ **Buka Pengaturan Printer...**
     - 📁 **Buka Folder PDF...** (Membuka folder `C:\SIMRS_PDF`)
     - 🔄 **Jalankan Otomatis saat Windows Boot** (Centang status Registry)
     - ❌ **Keluar dari Print Agent**

3. **Auto-Start di Windows (Auto Registry)**:
   - Begitu aplikasi pertama kali dijalankan, sistem secara otomatis mendaftarkan dirinya ke Registry Windows:  
     `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run\SmartPrintAgent`
   - Setiap kali komputer rumah sakit dinyalakan / di-restart, Print Agent akan **otomatis aktif di System Tray** tanpa perlu campur tangan pengguna.

---

## 🚀 Fitur Utama Lainnya

* **Kecepatan Sub-Milidetik**: Respon HTTP < 2 ms, waktu cetak kertas keluar 0.05 – 0.1 detik.
* **Native Drivers**:
  * **EPSON TM-T82X**: Native ESC/POS Thermal 80mm dengan font besar dan potong kertas otomatis.
  * **TSC Barcode**: Native TSPL untuk label etiket obat dan barcode presisi tinggi.
* **Auto PDF Fallback**: Jika printer fisik belum dicolok atau offline, otomatis tersimpan rapi sebagai file PDF di `C:\SIMRS_PDF\YYYY-MM-DD\` tanpa pop-up dialog.
* **Smart Sharing Printer**: Mendukung Direct USB Spooler, Network Socket IP:9100, dan Agent-to-Agent Relay.

---

## 📁 Struktur Direktori Project

```
smart-print-agent/
├── bin/
│   └── smart-print-agent.exe         # Binary Executable mandiri (Hanya ~7 MB!)
├── cmd/
│   └── agent/
│       └── main.go                   # Entry point aplikasi
├── configs/
│   └── config.json                   # Konfigurasi pemetaan printer
├── internal/
│   ├── autostart/
│   │   └── autostart.go              # Registry HKCU Windows Startup Manager
│   ├── tray/
│   │   └── tray.go                   # Native Win32 System Tray Icon & Popup Menu
│   ├── config/
│   │   └── config.go                 # Thread-safe config loader
│   ├── handler/
│   │   └── api.go                    # API routes & CORS handler
│   ├── model/
│   │   ├── payload.go                # Data model JSON request/response
│   │   └── printer.go                # Printer mapping model
│   ├── printer/
│   │   ├── escpos.go                 # ESC/POS Builder (Epson TM-T82X)
│   │   ├── tspl.go                   # TSPL Builder (TSC Barcode)
│   │   ├── winspool.go               # Win32 winspool.drv RAW Driver
│   │   ├── socket.go                 # Direct TCP Socket Driver (Port 9100)
│   │   ├── relay.go                  # HTTP Relay Antar-Agent (Sharing)
│   │   ├── pdf.go                    # Headless PDF Fallback Generator
│   │   └── manager.go                # Central Smart Router
│   ├── queue/
│   │   └── queue.go                  # Non-blocking worker queue & history
│   └── web/
│       ├── web.go                    # Go embed.FS static bundler
│       └── static/
│           └── index.html            # Web Dashboard Pengaturan Printer
├── go.mod
└── README.md
```

---

## 🖥️ Cara Menjalankan & Menguji

1. **Jalankan Aplikasi:**  
   Cukup double-click file:  
   👉 **`d:\Belajar\Printer\smart-print-agent\bin\smart-print-agent.exe`**
2. Perhatikan di pojok kanan bawah layar (dekat jam Windows / tombol tanda panah atas System Tray):  
   Ikon **Smart Print Agent** sudah aktif di sana!
3. **Buka Pengaturan:**  
   Cukup klik dua kali (atau klik kanan -> *Buka Pengaturan Printer...*) pada ikon tray tersebut.  
   Browser akan langsung membuka halaman pengaturan di **`http://127.0.0.1:1237/settings`**.
4. **Compile Ulang (Jika Mengubah Kode):**
   ```powershell
   cd "d:\Belajar\Printer\smart-print-agent"
   go build -ldflags "-H=windowsgui -s -w" -o bin/smart-print-agent.exe ./cmd/agent
   ```
