package model

// PaperConfig mendefinisikan dimensi fisik kertas dalam satuan milimeter
type PaperConfig struct {
	WidthMM     float64 `json:"width_mm"`     // Lebar kertas dalam mm (cth: 50, 70, 80, 100, 210)
	HeightMM    float64 `json:"height_mm"`    // Tinggi kertas dalam mm (cth: 30, 40, atau 0 untuk continuous roll)
	Orientation string  `json:"orientation"`  // "portrait" atau "landscape"
	MarginTop   float64 `json:"margin_top"`   // Margin atas dalam mm
	MarginLeft  float64 `json:"margin_left"`  // Margin kiri dalam mm
}

// VisualElement merepresentasikan satu komponen visual pada kanvas desain
type VisualElement struct {
	ID        string  `json:"id"`                  // ID unik elemen (cth: "el_1")
	Type      string  `json:"type"`                // "text", "barcode", "qrcode", "line", "box", "image"
	X         float64 `json:"x_mm"`                // Posisi horizontal dari kiri dalam mm
	Y         float64 `json:"y_mm"`                // Posisi vertikal dari atas dalam mm
	Width     float64 `json:"width_mm,omitempty"`  // Lebar elemen dalam mm
	Height    float64 `json:"height_mm,omitempty"` // Tinggi elemen dalam mm

	// Properti Teks
	Content    string  `json:"content,omitempty"`     // Teks statis atau template variable: "{{.nama}}"
	FontFamily string  `json:"font_family,omitempty"` // "Arial", "Times New Roman", "Courier"
	FontSize   float64 `json:"font_size,omitempty"`   // Ukuran font dalam pt (cth: 8, 10, 12, 16)
	FontWeight string  `json:"font_weight,omitempty"` // "normal" atau "bold"
	Align      string  `json:"align,omitempty"`       // "left", "center", "right"

	// Properti Barcode / QR Code
	Format   string `json:"format,omitempty"`    // "CODE128", "CODE39", "QR"
	ShowText bool   `json:"show_text,omitempty"` // Tampilkan teks angka di bawah barcode

	// Properti Garis & Box
	Thickness float64 `json:"thickness_mm,omitempty"` // Ketebalan garis dalam mm (default 0.3)
	Style     string  `json:"style,omitempty"`        // "solid", "dashed"

	// Properti Gambar
	Src string `json:"src,omitempty"` // Base64 data atau URL gambar
}

// TemplateDefinition mendefinisikan satu layout desain cetak lengkap
type TemplateDefinition struct {
	ID          string                 `json:"id"`                    // ID unik template (cth: "label_pasien", "antrian")
	Name        string                 `json:"name"`                  // Nama tampilan (cth: "Label Pasien 50x30mm")
	Category    string                 `json:"category"`              // "label", "struk", "gelang", "resep"
	Description string                 `json:"description,omitempty"` // Deskripsi penggunaan
	Version     string                 `json:"version"`               // Versi template (cth: "1.0.0")
	UpdatedAt   string                 `json:"updated_at,omitempty"`  // Tanggal update ISO
	Paper       PaperConfig            `json:"paper"`                 // Dimensi kertas
	Elements    []VisualElement        `json:"elements"`              // Daftar elemen desain
	SampleData  map[string]interface{} `json:"sample_data,omitempty"` // Data sampel untuk live preview & test cetak
}

