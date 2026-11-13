package printer

import (
	"fmt"
	"strings"

	"smart-print-agent/internal/model"
)

// BuildTSPL merakit serangkaian PrintLine menjadi perintah bahasa TSPL (TSC Barcode)
func BuildTSPL(lines []model.PrintLine, paperSize string, copies int) ([]byte, error) {
	var sb strings.Builder

	// 1. Tentukan ukuran label default (misal 50 mm, 30 mm)
	widthMM := "50 mm"
	heightMM := "30 mm"
	if paperSize != "" && strings.Contains(paperSize, "x") {
		parts := strings.Split(paperSize, "x")
		if len(parts) == 2 {
			w := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[0]), "mm"))
			h := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "mm"))
			widthMM = w + " mm"
			heightMM = h + " mm"
		}
	}

	sb.WriteString(fmt.Sprintf("SIZE %s, %s\r\n", widthMM, heightMM))
	sb.WriteString("GAP 2 mm, 0 mm\r\n")
	sb.WriteString("DIRECTION 1\r\n")
	sb.WriteString("REFERENCE 0,0\r\n")
	sb.WriteString("CLS\r\n")

	// 2. Loop setiap baris dan petakan koordinat
	defaultY := 20
	stepY := 35

	for _, line := range lines {
		x := line.X
		y := line.Y
		if x <= 0 {
			x = 20 // Default margin kiri
		}
		if y <= 0 {
			y = defaultY
			defaultY += stepY
		}

		switch strings.ToLower(line.Type) {
		case "divider":
			// Garis horizontal TSPL: BAR x, y, width, height
			sb.WriteString(fmt.Sprintf("BAR %d, %d, 360, 2\r\n", x, y))

		case "barcode":
			// BARCODE x, y, "code_type", height, human_readable (1/0), rotation (0), narrow (2), wide (2), "data"
			bType := "128"
			bHeight := line.Height
			if bHeight <= 0 {
				bHeight = 50
			}
			sb.WriteString(fmt.Sprintf("BARCODE %d, %d, \"%s\", %d, 1, 0, 2, 2, \"%s\"\r\n",
				x, y, bType, bHeight, escapeTSPL(line.Text)))
			defaultY += bHeight + 15

		case "qrcode":
			// QRCODE x, y, ECC (L/M/Q/H), cell_width (4-8), mode (A), rotation (0), "data"
			sb.WriteString(fmt.Sprintf("QRCODE %d, %d, L, 5, A, 0, \"%s\"\r\n",
				x, y, escapeTSPL(line.Text)))
			defaultY += 60

		default: // "text"
			font := "2" // Font kecil TSPL (8x12)
			xMul := 1
			yMul := 1

			switch strings.ToLower(line.Size) {
			case "medium", "2x":
				font = "3"
				xMul = 1
				yMul = 1
			case "large", "3x":
				font = "4"
				xMul = 1
				yMul = 1
			case "extra_large", "4x":
				font = "4"
				xMul = 2
				yMul = 2
			}

			if line.Bold {
				font = "3" // Font 3 lebih tebal
			}

			// TEXT x, y, "font", rotation, x_mul, y_mul, "content"
			sb.WriteString(fmt.Sprintf("TEXT %d, %d, \"%s\", 0, %d, %d, \"%s\"\r\n",
				x, y, font, xMul, yMul, escapeTSPL(line.Text)))
		}
	}

	// 3. Print command
	if copies <= 0 {
		copies = 1
	}
	sb.WriteString(fmt.Sprintf("PRINT 1, %d\r\n", copies))

	return []byte(sb.String()), nil
}

func escapeTSPL(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\[\"]")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}
