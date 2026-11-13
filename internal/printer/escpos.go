package printer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"smart-print-agent/internal/model"
)

// ESC/POS Command Constants
var (
	cmdInit       = []byte{0x1B, 0x40}             // ESC @ (Inisialisasi printer)
	cmdAlignLeft  = []byte{0x1B, 0x61, 0x00}       // ESC a 0 (Rata Kiri)
	cmdAlignCenter = []byte{0x1B, 0x61, 0x01}      // ESC a 1 (Rata Tengah)
	cmdAlignRight = []byte{0x1B, 0x61, 0x02}       // ESC a 2 (Rata Kanan)
	cmdBoldOn     = []byte{0x1B, 0x45, 0x01}       // ESC E 1 (Bold ON)
	cmdBoldOff    = []byte{0x1B, 0x45, 0x00}       // ESC E 0 (Bold OFF)
	cmdCutFull    = []byte{0x1D, 0x56, 0x00}       // GS V 0 (Potong Kertas Penuh)
	cmdCutPartial = []byte{0x1D, 0x56, 0x01}       // GS V 1 (Potong Kertas Sebagian)
	cmdKickDrawer = []byte{0x1B, 0x70, 0x00, 0x19, 0xFA} // ESC p 0 (Buka Laci Kasir)

	// Font sizes via GS ! n
	cmdSizeNormal     = []byte{0x1D, 0x21, 0x00} // 1x1
	cmdSizeMedium     = []byte{0x1D, 0x21, 0x11} // 2x2
	cmdSizeLarge      = []byte{0x1D, 0x21, 0x22} // 3x3
	cmdSizeExtraLarge = []byte{0x1D, 0x21, 0x33} // 4x4
)

// BuildESCPOS merakit serangkaian PrintLine menjadi raw bytes ESC/POS
func BuildESCPOS(lines []model.PrintLine, autoCut bool, openDrawer bool) ([]byte, error) {
	buf := new(bytes.Buffer)

	// 1. Inisialisasi printer
	buf.Write(cmdInit)

	// 2. Buka laci kasir jika diminta
	if openDrawer {
		buf.Write(cmdKickDrawer)
	}

	// 3. Proses setiap baris
	for _, line := range lines {
		switch strings.ToLower(line.Type) {
		case "divider":
			// Garis pembatas horizontal
			buf.Write(cmdAlignLeft)
			buf.Write(cmdBoldOff)
			buf.Write(cmdSizeNormal)
			switch strings.ToLower(line.Style) {
			case "double":
				buf.WriteString("==========================================\n")
			case "dashed":
				buf.WriteString("- - - - - - - - - - - - - - - - - - - - - \n")
			case "dot":
				buf.WriteString("..........................................\n")
			default:
				buf.WriteString("------------------------------------------\n")
			}

		case "row":
			// Baris tabel multi-kolom (khusus resep racikan, rincian biaya, atau tabel obat)
			totalWidth := 42
			if strings.ToLower(line.Size) == "small" {
				totalWidth = 48
			}
			buf.Write(cmdAlignLeft)
			if line.Bold {
				buf.Write(cmdBoldOn)
			} else {
				buf.Write(cmdBoldOff)
			}
			buf.Write(cmdSizeNormal)
			buf.WriteString(formatTableRowESC(line.Columns, totalWidth))
			buf.Write(cmdBoldOff)

		case "key_value":
			// Format terstruktur: "Label        : Nilai" dengan titik dua sejajar rapi
			buf.Write(cmdAlignLeft)
			if line.Bold {
				buf.Write(cmdBoldOn)
			} else {
				buf.Write(cmdBoldOff)
			}
			buf.Write(cmdSizeNormal)
			labelWidth := 14
			lbl := line.Label
			if len(lbl) < labelWidth {
				lbl = lbl + strings.Repeat(" ", labelWidth-len(lbl))
			}
			buf.WriteString(fmt.Sprintf("%s : %s\n", lbl, line.Value))
			buf.Write(cmdBoldOff)

		case "cut":
			// Perintah potong kertas di tengah konten
			buf.Write([]byte("\n\n\n"))
			buf.Write(cmdCutFull)

		case "feed", "newline":
			n := line.Height
			if n <= 0 {
				n = 1
			}
			for i := 0; i < n; i++ {
				buf.WriteByte('\n')
			}

		case "image", "logo":
			// Render image jika ada base64 data
			if line.Data != "" {
				imgBytes, err := renderRasterImageESC(line.Data, line.Align)
				if err == nil {
					buf.Write(imgBytes)
				}
			}

		case "barcode":
			// Barcode Code128 ESC/POS
			if line.Text != "" {
				buf.Write(renderBarcodeESC(line.Text, line.Align))
			}

		case "qrcode":
			if line.Text != "" {
				buf.Write(renderQRCodeESC(line.Text, line.Align))
			}

		default: // "text"
			// Atur alignment
			switch strings.ToLower(line.Align) {
			case "center":
				buf.Write(cmdAlignCenter)
			case "right":
				buf.Write(cmdAlignRight)
			default:
				buf.Write(cmdAlignLeft)
			}

			// Atur Bold
			if line.Bold {
				buf.Write(cmdBoldOn)
			} else {
				buf.Write(cmdBoldOff)
			}

			// Atur Ukuran Font
			maxChars := 42
			switch strings.ToLower(line.Size) {
			case "small":
				maxChars = 48
			case "medium", "2x":
				buf.Write(cmdSizeMedium)
				maxChars = 28
			case "large", "3x":
				buf.Write(cmdSizeLarge)
				maxChars = 21
			case "extra_large", "4x", "huge":
				buf.Write(cmdSizeExtraLarge)
				maxChars = 14
			default:
				buf.Write(cmdSizeNormal)
			}

			// Pecah per baris \n dan word-wrap agar rapi di kertas thermal
			subLines := strings.Split(line.Text, "\n")
			for _, sub := range subLines {
				trimmed := strings.TrimRight(sub, "\r")
				if trimmed == "" {
					buf.WriteByte('\n')
					continue
				}
				wrapped := wrapWords(trimmed, maxChars)
				for _, w := range wrapped {
					buf.WriteString(w)
					buf.WriteByte('\n')
				}
			}

			// Reset formatting kembali ke normal
			buf.Write(cmdBoldOff)
			buf.Write(cmdSizeNormal)
		}
	}

	// 4. Feed & Potong Kertas di akhir
	if autoCut {
		buf.Write([]byte("\n\n\n\n"))
		buf.Write(cmdCutFull)
	}

	return buf.Bytes(), nil
}

// renderBarcodeESC menghasilkan perintah Code128 ESC/POS
func renderBarcodeESC(data string, align string) []byte {
	buf := new(bytes.Buffer)
	switch strings.ToLower(align) {
	case "center":
		buf.Write(cmdAlignCenter)
	case "right":
		buf.Write(cmdAlignRight)
	default:
		buf.Write(cmdAlignLeft)
	}

	// GS h 80 (Tinggi barcode 80 dots)
	buf.Write([]byte{0x1D, 0x68, 0x50})
	// GS w 2 (Lebar bar 2 dots)
	buf.Write([]byte{0x1D, 0x77, 0x02})
	// GS H 2 (Tampilkan teks angka barcode di bawah)
	buf.Write([]byte{0x1D, 0x48, 0x02})
	// GS k 73 len data (CODE128)
	buf.Write([]byte{0x1D, 0x6B, 0x49, byte(len(data))})
	buf.WriteString(data)
	buf.WriteByte('\n')
	return buf.Bytes()
}

// renderQRCodeESC menghasilkan perintah QR Code ESC/POS
func renderQRCodeESC(data string, align string) []byte {
	buf := new(bytes.Buffer)
	switch strings.ToLower(align) {
	case "center":
		buf.Write(cmdAlignCenter)
	case "right":
		buf.Write(cmdAlignRight)
	default:
		buf.Write(cmdAlignLeft)
	}

	// Model 2
	buf.Write([]byte{0x1D, 0x28, 0x6B, 0x04, 0x00, 0x31, 0x41, 0x32, 0x00})
	// Size (module size = 6)
	buf.Write([]byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x43, 0x06})
	// Error correction level L
	buf.Write([]byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x45, 0x30})

	// Store data: len + 3
	dataLen := len(data) + 3
	pL := byte(dataLen & 0xFF)
	pH := byte((dataLen >> 8) & 0xFF)
	buf.Write([]byte{0x1D, 0x28, 0x6B, pL, pH, 0x31, 0x50, 0x30})
	buf.WriteString(data)

	// Print QR
	buf.Write([]byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x51, 0x30})
	buf.WriteByte('\n')
	return buf.Bytes()
}

// renderRasterImageESC mengubah base64 image menjadi perintah ESC/POS GS v 0
func renderRasterImageESC(base64Data string, align string) ([]byte, error) {
	// Bersihkan header data URI jika ada (data:image/png;base64,...)
	idx := strings.Index(base64Data, ",")
	rawBase64 := base64Data
	if idx != -1 {
		rawBase64 = base64Data[idx+1:]
	}

	dec, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(dec))
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Batasi lebar maksimum thermal 80mm (~384-512 dots)
	if width > 384 {
		width = 384
	}

	// Konversi ke monokrom bitmap (1 bit per pixel)
	bytesPerLine := (width + 7) / 8
	rasterData := make([]byte, bytesPerLine*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.GrayModel.Convert(img.At(x+bounds.Min.X, y+bounds.Min.Y)).(color.Gray)
			// Threshold: jika lebih gelap dari 128, jadikan titik hitam (bit = 1)
			if c.Y < 128 {
				byteIdx := y*bytesPerLine + (x / 8)
				bitIdx := uint(7 - (x % 8))
				rasterData[byteIdx] |= (1 << bitIdx)
			}
		}
	}

	buf := new(bytes.Buffer)
	switch strings.ToLower(align) {
	case "center":
		buf.Write(cmdAlignCenter)
	case "right":
		buf.Write(cmdAlignRight)
	default:
		buf.Write(cmdAlignLeft)
	}

	// GS v 0 m xL xH yL yH
	xL := byte(bytesPerLine & 0xFF)
	xH := byte((bytesPerLine >> 8) & 0xFF)
	yL := byte(height & 0xFF)
	yH := byte((height >> 8) & 0xFF)

	buf.Write([]byte{0x1D, 0x76, 0x30, 0x00, xL, xH, yL, yH})
	buf.Write(rasterData)
	buf.WriteByte('\n')

	return buf.Bytes(), nil
}

// formatTableRowESC memformat baris tabel multi-kolom dengan padding spasi rata sempurna
func formatTableRowESC(columns []model.TableColumn, totalWidth int) string {
	if len(columns) == 0 {
		return ""
	}

	numCols := len(columns)
	colWidths := make([]int, numCols)
	usedWidth := 0

	for i, col := range columns {
		if col.Width > 0 {
			colWidths[i] = int(float64(totalWidth) * (col.Width / 100.0))
		} else {
			colWidths[i] = totalWidth / numCols
		}
		if colWidths[i] < 1 {
			colWidths[i] = 1
		}
		usedWidth += colWidths[i]
	}

	// Sesuaikan selisih lebar pada kolom terakhir agar tepat totalWidth
	diff := totalWidth - usedWidth
	colWidths[numCols-1] += diff
	if colWidths[numCols-1] < 1 {
		colWidths[numCols-1] = 1
	}

	// Word wrap setiap kolom sesuai lebarnya
	colLines := make([][]string, numCols)
	maxLines := 1
	for i, col := range columns {
		lines := wrapWords(col.Text, colWidths[i])
		if len(lines) == 0 {
			lines = []string{""}
		}
		colLines[i] = lines
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}

	var sb strings.Builder
	for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
		for i, col := range columns {
			w := colWidths[i]
			text := ""
			if lineIdx < len(colLines[i]) {
				text = colLines[i][lineIdx]
			}

			// Format padding alignment
			runeCount := len([]rune(text))
			if runeCount > w {
				text = string([]rune(text)[:w])
				runeCount = w
			}
			padSpaces := w - runeCount

			switch strings.ToLower(col.Align) {
			case "right":
				sb.WriteString(strings.Repeat(" ", padSpaces))
				sb.WriteString(text)
			case "center":
				leftPad := padSpaces / 2
				rightPad := padSpaces - leftPad
				sb.WriteString(strings.Repeat(" ", leftPad))
				sb.WriteString(text)
				sb.WriteString(strings.Repeat(" ", rightPad))
			default: // left
				sb.WriteString(text)
				sb.WriteString(strings.Repeat(" ", padSpaces))
			}
		}
		sb.WriteByte('\n')
	}

	return sb.String()
}


