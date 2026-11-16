package printer

import (
	"fmt"
	"strings"

	"smart-print-agent/internal/model"
)

// IsTwoColumnPaper mengecek apakah konfigurasi ukuran kertas bertipe 2 kolom (kanan-kiri)
func IsTwoColumnPaper(paperSize string) bool {
	p := strings.ToLower(paperSize)
	return strings.Contains(p, "2col") || strings.Contains(p, "2-up") ||
		strings.Contains(p, "twin") || strings.Contains(p, "75x20") ||
		strings.Contains(p, "70x15") || strings.Contains(p, "80x25") ||
		strings.Contains(p, "kanan") || strings.Contains(p, "ganda")
}

// BuildTSPL merakit serangkaian PrintLine menjadi perintah bahasa TSPL (TSC Barcode)
// Mendukung single column (1 stiker) dan dual column (2 stiker kanan-kiri / 2-up)
func BuildTSPL(lines []model.PrintLine, paperSize string, copies int) ([]byte, error) {
	var sb strings.Builder

	// 1. Tentukan ukuran label default
	is2Col := IsTwoColumnPaper(paperSize)
	widthMM := "50 mm"
	heightMM := "30 mm"
	if is2Col {
		widthMM = "75 mm"
		heightMM = "20 mm"
	}

	if paperSize != "" && strings.Contains(paperSize, "x") {
		parts := strings.Split(paperSize, "x")
		if len(parts) >= 2 {
			w := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[0]), "mm"))
			hPart := parts[1]
			// Handle format seperti 20mm_2col atau 20_2col
			hSub := strings.Split(hPart, "_")[0]
			h := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(hSub), "mm"))
			if w != "" {
				widthMM = w + " mm"
			}
			if h != "" {
				heightMM = h + " mm"
			}
		}
	}

	sb.WriteString(fmt.Sprintf("SIZE %s, %s\r\n", widthMM, heightMM))
	sb.WriteString("GAP 2 mm, 0 mm\r\n")
	sb.WriteString("DIRECTION 1\r\n")
	sb.WriteString("REFERENCE 0,0\r\n")
	sb.WriteString("CLS\r\n")

	// Offset kolom untuk roll 2 kolom (kanan-kiri)
	// Pada printer TSC 203 DPI: 1 mm = 8 dots.
	// Untuk stiker 35mm + gap 2.5mm = 37.5mm = 300 dots.
	colCount := 1
	colOffsetDots := 0
	dividerWidth := 360

	if is2Col {
		colCount = 2
		colOffsetDots = 300 // Offset horizontal kolom ke-2
		dividerWidth = 260  // Lebar garis divider per stiker agar tidak menyeberangi gap
	}

	// 2. Loop setiap baris dan petakan koordinat
	defaultY := 15
	stepY := 28

	for _, line := range lines {
		baseX := line.X
		y := line.Y
		if baseX <= 0 {
			baseX = 16 // Margin kiri ~2mm
		}
		if y <= 0 {
			y = defaultY
			defaultY += stepY
		}

		switch strings.ToLower(line.Type) {
		case "divider":
			for col := 0; col < colCount; col++ {
				curX := baseX + (col * colOffsetDots)
				sb.WriteString(fmt.Sprintf("BAR %d, %d, %d, 2\r\n", curX, y, dividerWidth))
			}

		case "barcode":
			bType := "128"
			bHeight := line.Height
			if bHeight <= 0 {
				if is2Col {
					bHeight = 35 // Tinggi pas untuk stiker 20mm
				} else {
					bHeight = 45
				}
			}
			for col := 0; col < colCount; col++ {
				curX := baseX + (col * colOffsetDots)
				sb.WriteString(fmt.Sprintf("BARCODE %d, %d, \"%s\", %d, 1, 0, 2, 2, \"%s\"\r\n",
					curX, y, bType, bHeight, escapeTSPL(line.Text)))
			}
			defaultY += bHeight + 12

		case "qrcode":
			for col := 0; col < colCount; col++ {
				curX := baseX + (col * colOffsetDots)
				sb.WriteString(fmt.Sprintf("QRCODE %d, %d, L, 4, A, 0, \"%s\"\r\n",
					curX, y, escapeTSPL(line.Text)))
			}
			defaultY += 50

		default: // "text"
			font := "2" // Default font TSPL (12x20)
			xMul := 1
			yMul := 1

			switch strings.ToLower(line.Size) {
			case "small":
				font = "1" // Font kecil (8x12)
			case "medium", "2x":
				font = "3"
			case "large", "3x":
				font = "3"
				xMul = 1
				yMul = 1
			case "extra_large", "4x":
				font = "4"
			}

			if line.Bold && font == "2" {
				font = "3"
			}

			for col := 0; col < colCount; col++ {
				curX := baseX + (col * colOffsetDots)
				sb.WriteString(fmt.Sprintf("TEXT %d, %d, \"%s\", 0, %d, %d, \"%s\"\r\n",
					curX, y, font, xMul, yMul, escapeTSPL(line.Text)))
			}
		}
	}

	// 3. Print command
	if copies <= 0 {
		copies = 1
	}
	sb.WriteString(fmt.Sprintf("PRINT 1, %d\r\n", copies))

	return []byte(sb.String()), nil
}

// BuildTSPLFromTemplate merender TemplateDefinition dinamis menjadi perintah bahasa TSPL langsung
// Mendukung pemosisian presisi mm, interpolasi data, dan duplikasi 2 kolom otomatis
func BuildTSPLFromTemplate(tmpl *model.TemplateDefinition, data map[string]interface{}, copies int) ([]byte, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("template nil")
	}

	var sb strings.Builder

	// Hitung dimensi fisik
	wMM := tmpl.Paper.WidthMM
	if wMM <= 0 {
		wMM = 75.0
	}
	hMM := tmpl.Paper.HeightMM
	if hMM <= 0 {
		hMM = 20.0
	}

	sb.WriteString(fmt.Sprintf("SIZE %.1f mm, %.1f mm\r\n", wMM, hMM))
	sb.WriteString("GAP 2 mm, 0 mm\r\n")
	sb.WriteString("DIRECTION 1\r\n")
	sb.WriteString("REFERENCE 0,0\r\n")
	sb.WriteString("CLS\r\n")

	// Multi-column offset
	colCount := 1
	colOffsetDots := 0
	if tmpl.Paper.Columns >= 2 {
		colCount = tmpl.Paper.Columns
		labelW := tmpl.Paper.LabelWidthMM
		if labelW <= 0 {
			labelW = (wMM - float64(colCount-1)*tmpl.Paper.ColumnGapMM) / float64(colCount)
		}
		gapMM := tmpl.Paper.ColumnGapMM
		if gapMM <= 0 {
			gapMM = 2.5
		}
		// 1 mm = 8 dots di 203 DPI
		colOffsetDots = int((labelW + gapMM) * 8.0)
	}

	interpolate := func(s string) string {
		if data == nil {
			return s
		}
		res := s
		for k, v := range data {
			placeholder := fmt.Sprintf("{{.%s}}", k)
			res = strings.ReplaceAll(res, placeholder, fmt.Sprintf("%v", v))
		}
		return res
	}

	// Render setiap elemen visual ke TSPL
	for _, el := range tmpl.Elements {
		baseXDots := int((el.X + tmpl.Paper.MarginLeft) * 8.0)
		yDots := int((el.Y + tmpl.Paper.MarginTop) * 8.0)

		switch strings.ToLower(el.Type) {
		case "box", "rect":
			wDots := int(el.Width * 8.0)
			if wDots <= 0 {
				wDots = 260
			}
			hDots := int(el.Height * 8.0)
			if hDots <= 0 {
				hDots = 20
			}
			thick := int(el.Thickness * 8.0)
			if thick <= 0 {
				thick = 2
			}
			for col := 0; col < colCount; col++ {
				x := baseXDots + (col * colOffsetDots)
				sb.WriteString(fmt.Sprintf("BOX %d, %d, %d, %d, %d\r\n", x, yDots, x+wDots, yDots+hDots, thick))
			}

		case "line", "divider":
			wDots := int(el.Width * 8.0)
			if wDots <= 0 {
				wDots = 260
			}
			thick := int(el.Thickness * 8.0)
			if thick <= 0 {
				thick = 2
			}
			for col := 0; col < colCount; col++ {
				x := baseXDots + (col * colOffsetDots)
				sb.WriteString(fmt.Sprintf("BAR %d, %d, %d, %d\r\n", x, yDots, wDots, thick))
			}

		case "barcode":
			barcodeVal := interpolate(el.Content)
			if barcodeVal == "" {
				continue
			}
			hDots := int(el.Height * 8.0)
			if hDots <= 0 {
				hDots = 35
			}
			showText := 0
			if el.ShowText {
				showText = 1
			}
			for col := 0; col < colCount; col++ {
				x := baseXDots + (col * colOffsetDots)
				sb.WriteString(fmt.Sprintf("BARCODE %d, %d, \"128\", %d, %d, 0, 2, 2, \"%s\"\r\n",
					x, yDots, hDots, showText, escapeTSPL(barcodeVal)))
			}

		case "qrcode":
			qrVal := interpolate(el.Content)
			if qrVal == "" {
				continue
			}
			cellW := 4
			if el.Width > 15 {
				cellW = 5
			}
			for col := 0; col < colCount; col++ {
				x := baseXDots + (col * colOffsetDots)
				sb.WriteString(fmt.Sprintf("QRCODE %d, %d, L, %d, A, 0, \"%s\"\r\n",
					x, yDots, cellW, escapeTSPL(qrVal)))
			}

		default: // "text"
			textVal := interpolate(el.Content)
			if textVal == "" {
				continue
			}

			// Tentukan font TSPL berdasarkan font_size dalam pt
			font := "2" // 12x20
			xMul := 1
			yMul := 1

			if el.FontSize <= 7.5 {
				font = "1" // 8x12 (sangat kecil & tajam)
			} else if el.FontSize >= 11 {
				font = "3" // 16x24
			} else if el.FontSize >= 14 {
				font = "4" // 24x32
			}

			if strings.EqualFold(el.FontWeight, "bold") && font == "2" {
				font = "3"
			}

			lines := strings.Split(textVal, "\n")
			lineStepDots := int(el.FontSize * 1.5 * 8.0 * 25.4 / 72.0)
			if lineStepDots < 16 {
				lineStepDots = 18
			}

			for lIdx, lStr := range lines {
				if lStr == "" {
					continue
				}
				curY := yDots + (lIdx * lineStepDots)
				for col := 0; col < colCount; col++ {
					x := baseXDots + (col * colOffsetDots)
					sb.WriteString(fmt.Sprintf("TEXT %d, %d, \"%s\", 0, %d, %d, \"%s\"\r\n",
						x, curY, font, xMul, yMul, escapeTSPL(lStr)))
				}
			}
		}
	}

	// Cetak
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
