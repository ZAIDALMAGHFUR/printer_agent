package model

import "time"

// TableColumn merepresentasikan satu kolom dalam baris tabel (untuk resep racikan, rincian biaya, dll.)
type TableColumn struct {
	Text  string  `json:"text"`
	Width float64 `json:"width,omitempty"` // Lebar kolom dalam persen (cth: 50 untuk 50%)
	Align string  `json:"align,omitempty"` // left, center, right
	Bold  bool    `json:"bold,omitempty"`
}

// PrintLine merepresentasikan satu baris instruksi cetak (untuk struk thermal atau label barcode)
type PrintLine struct {
	Type        string        `json:"type,omitempty"`        // text, row, divider, image, logo, barcode, qrcode, cut, feed, newline, key_value
	Text        string        `json:"text,omitempty"`        // Isi teks (jika baris teks biasa)
	Label       string        `json:"label,omitempty"`       // Label untuk key_value (cth: "No. RM")
	Value       string        `json:"value,omitempty"`       // Nilai untuk key_value (cth: "RM-001234")
	Columns     []TableColumn `json:"columns,omitempty"`     // Daftar kolom (jika type == "row" untuk tabel resep racikan / rincian)
	Align       string        `json:"align,omitempty"`       // left, center, right
	Bold        bool          `json:"bold,omitempty"`        // true / false
	Size        string        `json:"size,omitempty"`        // small, normal, medium, large, extra_large
	Font        string        `json:"font,omitempty"`        // sans, serif
	Style       string        `json:"style,omitempty"`       // solid, dashed, double (untuk divider)
	Data        string        `json:"data,omitempty"`        // Base64 data untuk image atau nilai barcode/qrcode
	BarcodeType string        `json:"barcode_type,omitempty"` // CODE128, EAN13, QR
	Width       int           `json:"width,omitempty"`       // Lebar (dalam dot / mm jika spesifik)
	Height      int           `json:"height,omitempty"`      // Tinggi (dalam dot / baris)
	X           int           `json:"x,omitempty"`           // Posisi koordinat X (khusus label TSPL)
	Y           int           `json:"y,omitempty"`           // Posisi koordinat Y (khusus label TSPL)
}

// PrintOptions mendefinisikan opsi cetak tambahan
type PrintOptions struct {
	Copies         int  `json:"copies,omitempty"`           // Jumlah rangkap cetak (default: 1)
	CutPaper       bool `json:"cut_paper,omitempty"`        // Potong kertas otomatis (default: true)
	OpenCashDrawer bool `json:"open_cash_drawer,omitempty"` // Buka laci kasir (kick cash drawer)
	SoundBuzzer    bool `json:"sound_buzzer,omitempty"`     // Bunyikan buzzer printer jika ada
	View           bool `json:"view,omitempty"`             // Jika true, preview PDF di browser (tidak langsung cetak)
}

// PrintRequest adalah payload utama yang dikirim dari Web SIMRS ke Agent
type PrintRequest struct {
	RequestID     string                 `json:"request_id,omitempty"`     // ID unik request (cth: REQ-001)
	PrinterTarget string                 `json:"printer_target"`            // Cth: CetakAntrian, Kwitansi, LabelFarmasi, Billing
	DocumentType  string                 `json:"document_type,omitempty"`  // antrian, label, kwitansi, sep, billing
	Template      string                 `json:"template,omitempty"`       // antrian, resep_racikan, resep_non_racikan, etiket
	Format        string                 `json:"format,omitempty"`         // lines (default), raw, html
	Konten        []PrintLine            `json:"konten,omitempty"`         // Daftar baris (generic layout)
	RawData       string                 `json:"raw_data,omitempty"`       // Raw ESC/POS atau TSPL string/base64
	HTML          string                 `json:"html,omitempty"`           // Konten HTML untuk dokumen A4
	PDFURL        string                 `json:"pdf_url,omitempty"`        // URL PDF dari server backend untuk diunduh & dicetak
	Data          map[string]interface{} `json:"data,omitempty"`           // Key-value data mentah jika pakai template
	Options       PrintOptions           `json:"options,omitempty"`        // Opsi cetak tambahan
}

// PrintResponse adalah respon JSON standar yang dikembalikan Agen ke SIMRS
type PrintResponse struct {
	Status        string `json:"status"`                   // success, queued, saved_pdf, error
	JobID         string `json:"job_id,omitempty"`         // ID unik pekerjaan cetak
	Message       string `json:"message"`                  // Pesan status
	TargetPrinter string `json:"target_printer,omitempty"` // Nama printer yang digunakan
	TargetType    string `json:"target_type,omitempty"`    // usb, socket, relay, pdf
	FilePath      string `json:"file_path,omitempty"`      // Path file jika disimpan sebagai PDF
	DurationMs    int64  `json:"duration_ms,omitempty"`    // Durasi pemrosesan dalam milidetik
	Timestamp     string `json:"timestamp"`                // RFC3339 timestamp
}

// PrintJob adalah unit pekerjaan di dalam internal Queue agen
type PrintJob struct {
	ID        string
	Request   PrintRequest
	CreatedAt time.Time
}
