package printer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"smart-print-agent/internal/model"
)

type pdfImageItem struct {
	widthDots  int
	heightDots int
	rgbData    []byte
	dispWidth  float64
	dispHeight float64
}

type pdfRenderItem struct {
	itemType    string // "text", "row", "key_value", "divider", "spacing", "image"
	text        string
	label       string
	value       string
	columns     []model.TableColumn
	colLines    [][]string // baris-baris per kolom yang sudah di-wrap
	colWidths   []float64  // lebar masing-masing kolom dalam pt
	style       string     // solid, dashed, double
	fontSize    float64
	fontName    string
	align       string
	beforeSpace float64
	afterSpace  float64
	imageInfo   *pdfImageItem
}

// GenerateReceiptPDF membuat file PDF struk thermal (ukuran 80mm) yang rapi, presisi, dan mendukung resep racikan & logo
func GenerateReceiptPDF(req model.PrintRequest, saveDir string) (string, error) {
	now := time.Now()
	dateFolder := now.Format("2006-01-02")
	targetDir := filepath.Join(saveDir, dateFolder)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori PDF: %w", err)
	}

	docName := req.DocumentType
	if docName == "" {
		docName = req.PrinterTarget
	}
	if docName == "" {
		docName = "Struk"
	}

	reqID := req.RequestID
	if reqID == "" {
		reqID = "REQ"
	}
	fileName := fmt.Sprintf("%s_%s_%s.pdf", docName, reqID, now.Format("150405"))
	fullPath := filepath.Join(targetDir, fileName)

	// Jika request menggunakan Template terstruktur, proses template terlebih dahulu
	linesToRender := req.Konten
	if req.Template != "" && len(linesToRender) == 0 {
		linesToRender = ApplyTemplate(req.Template, req.Data)
	}

	pageWidth := 226.0 // Lebar standar kertas thermal 80mm dalam PDF points (80 / 25.4 * 72 = ~226.77 pt)
	isLabel := strings.Contains(strings.ToLower(docName), "label") ||
		strings.Contains(strings.ToLower(req.Template), "label") ||
		strings.Contains(strings.ToLower(req.PrinterTarget), "label")
	if isLabel {
		pageWidth = 190.0 // Lebar proporsional untuk label stiker 50-60mm
	}
	pdfBytes := buildReceiptPDF(linesToRender, pageWidth)

	if err := os.WriteFile(fullPath, pdfBytes, 0644); err != nil {
		return "", fmt.Errorf("gagal menyimpan file PDF: %w", err)
	}

	return fullPath, nil
}

// buildReceiptPDF menyusun dokumen PDF struk thermal dengan layout presisi tanpa tabrakan font
func buildReceiptPDF(lines []model.PrintLine, width float64) []byte {
	var renderList []pdfRenderItem
	var embeddedImages []*pdfImageItem

	printableWidth := width - 28.0 // Margin kiri 14, margin kanan 14

	// 1. Fase Parsing & Layouting Element
	for _, line := range lines {
		lineType := strings.ToLower(line.Type)

		switch lineType {
		case "divider":
			style := line.Style
			if style == "" {
				style = "solid"
			}
			renderList = append(renderList, pdfRenderItem{
				itemType:    "divider",
				style:       style,
				beforeSpace: 6.0,
				afterSpace:  6.0,
			})

		case "cut", "feed", "newline":
			renderList = append(renderList, pdfRenderItem{
				itemType:   "spacing",
				afterSpace: 12.0,
			})

		case "image", "logo":
			if line.Data != "" {
				imgItem, err := decodeBase64Image(line.Data, printableWidth)
				if err == nil && imgItem != nil {
					embeddedImages = append(embeddedImages, imgItem)
					renderList = append(renderList, pdfRenderItem{
						itemType:    "image",
						imageInfo:   imgItem,
						align:       line.Align,
						beforeSpace: 4.0,
						afterSpace:  8.0,
					})
				}
			}

		case "barcode":
			bHeight := float64(line.Height)
			if bHeight <= 0 {
				bHeight = 36.0
			}
			renderList = append(renderList, pdfRenderItem{
				itemType:    "barcode",
				text:        line.Text,
				fontSize:    bHeight,
				beforeSpace: 4.0,
				afterSpace:  4.0,
			})

		case "key_value":
			// Baris terstruktur: "Label        : Nilai"
			fontSize := 8.5
			fontName := "/F1"
			if line.Bold {
				fontName = "/F2"
			}
			renderList = append(renderList, pdfRenderItem{
				itemType:    "key_value",
				label:       line.Label,
				value:       line.Value,
				fontSize:    fontSize,
				fontName:    fontName,
				beforeSpace: 1.5,
				afterSpace:  1.5,
			})

		case "row":
			// Baris tabel multi-kolom (khusus resep racikan / rincian obat)
			fontSize := 8.0
			fontName := "/F1"
			if line.Bold {
				fontName = "/F2"
			}

			numCols := len(line.Columns)
			if numCols == 0 {
				continue
			}

			colWidths := make([]float64, numCols)
			for i, col := range line.Columns {
				if col.Width > 0 {
					colWidths[i] = printableWidth * (col.Width / 100.0)
				} else {
					colWidths[i] = printableWidth / float64(numCols)
				}
			}

			colLines := make([][]string, numCols)
			maxLines := 1
			for i, col := range line.Columns {
				// Hitung batas karakter per kolom berdasarkan lebar pt
				approxChars := int(colWidths[i] / (fontSize * 0.52))
				if approxChars < 1 {
					approxChars = 1
				}
				wrapped := wrapWords(col.Text, approxChars)
				if len(wrapped) == 0 {
					wrapped = []string{""}
				}
				colLines[i] = wrapped
				if len(wrapped) > maxLines {
					maxLines = len(wrapped)
				}
			}

			renderList = append(renderList, pdfRenderItem{
				itemType:    "row",
				columns:     line.Columns,
				colLines:    colLines,
				colWidths:   colWidths,
				fontSize:    fontSize,
				fontName:    fontName,
				beforeSpace: 2.0,
				afterSpace:  2.0,
			})

		default: // "text"
			fontSize := 9.0
			fontName := "/F1" // Helvetica Regular

			// Dukungan font Serif (Times-Bold / Times-Roman) seperti di tiket rumah sakit
			isSerif := strings.ToLower(line.Font) == "serif"
			if strings.EqualFold(line.Text, "RSU ASHAVIN") || strings.Contains(strings.ToLower(line.Text), "nomor antrian") {
				isSerif = true
			}

			if line.Bold {
				if isSerif {
					fontName = "/F4" // Times-Bold
				} else {
					fontName = "/F2" // Helvetica-Bold
				}
			} else {
				if isSerif {
					fontName = "/F3" // Times-Roman
				} else {
					fontName = "/F1" // Helvetica
				}
			}

			maxChars := 42
			beforeSpace := 2.0
			afterSpace := 3.0

			switch strings.ToLower(line.Size) {
			case "small":
				fontSize = 7.5
				maxChars = 48
				beforeSpace = 1.0
				afterSpace = 2.0
			case "medium", "2x":
				fontSize = 11.5
				maxChars = 30
				beforeSpace = 3.0
				afterSpace = 4.0
			case "large", "3x":
				fontSize = 14.0
				maxChars = 24
				beforeSpace = 4.0
				afterSpace = 4.0
			case "extra_large", "4x", "huge":
				fontSize = 36.0  // Nomor antrian besar (U-002)
				fontName = "/F4" // Times-Bold
				maxChars = 10
				beforeSpace = 14.0 // Ruang lega sebelum angka antrian
				afterSpace = 14.0  // Ruang lega setelah angka antrian
			default:
				fontSize = 9.0
				maxChars = 40
			}

			// Pecah jika ada karakter enter (\n)
			rawSubLines := strings.Split(line.Text, "\n")
			for _, rawSub := range rawSubLines {
				trimmed := strings.TrimSpace(rawSub)
				if trimmed == "" {
					renderList = append(renderList, pdfRenderItem{
						itemType:   "spacing",
						afterSpace: 8.0,
					})
					continue
				}

				wrapped := wrapWords(trimmed, maxChars)
				for _, wText := range wrapped {
					renderList = append(renderList, pdfRenderItem{
						itemType:    "text",
						text:        wText,
						fontSize:    fontSize,
						fontName:    fontName,
						align:       line.Align,
						beforeSpace: beforeSpace,
						afterSpace:  afterSpace,
					})
				}
			}
		}
	}

	// 2. Hitung total tinggi halaman secara dinamis berdasarkan seluruh elemen
	topMargin := 18.0
	bottomMargin := 22.0
	contentHeight := 0.0

	for _, item := range renderList {
		switch item.itemType {
		case "divider":
			contentHeight += item.beforeSpace + 2.0 + item.afterSpace
		case "spacing":
			contentHeight += item.afterSpace
		case "image":
			if item.imageInfo != nil {
				contentHeight += item.beforeSpace + item.imageInfo.dispHeight + item.afterSpace
			}
		case "barcode":
			contentHeight += item.beforeSpace + item.fontSize + item.afterSpace
		case "key_value":
			contentHeight += item.beforeSpace + item.fontSize + item.afterSpace
		case "row":
			maxL := 1
			for _, cl := range item.colLines {
				if len(cl) > maxL {
					maxL = len(cl)
				}
			}
			contentHeight += item.beforeSpace + float64(maxL)*(item.fontSize+2.0) + item.afterSpace
		default: // "text"
			contentHeight += item.beforeSpace + item.fontSize + item.afterSpace
		}
	}
	pageHeight := topMargin + contentHeight + bottomMargin

	// 3. Render PDF Stream
	var stream bytes.Buffer
	currY := pageHeight - topMargin

	stream.WriteString("BT\n")

	for _, item := range renderList {
		switch item.itemType {
		case "divider":
			stream.WriteString("ET\n")
			currY -= item.beforeSpace

			lineX1 := 14.0
			lineX2 := width - 14.0

			if item.style == "double" {
				stream.WriteString("0.25 0.25 0.25 RG\n")
				stream.WriteString(fmt.Sprintf("0.7 w %.2f %.2f m %.2f %.2f l S\n", lineX1, currY, lineX2, currY))
				stream.WriteString(fmt.Sprintf("0.7 w %.2f %.2f m %.2f %.2f l S\n", lineX1, currY-2.5, lineX2, currY-2.5))
				stream.WriteString("0.00 0.00 0.00 RG\n")
				currY -= 2.5
			} else if item.style == "dashed" {
				stream.WriteString("0.35 0.35 0.35 RG\n")
				stream.WriteString(fmt.Sprintf("[3 2] 0 d 0.7 w %.2f %.2f m %.2f %.2f l S [] 0 d\n", lineX1, currY, lineX2, currY))
				stream.WriteString("0.00 0.00 0.00 RG\n")
			} else {
				// Solid clean line
				stream.WriteString("0.25 0.25 0.25 RG\n")
				stream.WriteString(fmt.Sprintf("0.7 w %.2f %.2f m %.2f %.2f l S\n", lineX1, currY, lineX2, currY))
				stream.WriteString("0.00 0.00 0.00 RG\n")
			}

			currY -= item.afterSpace
			stream.WriteString("BT\n")

		case "spacing":
			currY -= item.afterSpace

		case "image":
			if item.imageInfo != nil {
				stream.WriteString("ET\n")
				currY -= item.beforeSpace + item.imageInfo.dispHeight

				imgX := (width - item.imageInfo.dispWidth) / 2.0
				if strings.ToLower(item.align) == "left" {
					imgX = 14.0
				} else if strings.ToLower(item.align) == "right" {
					imgX = width - item.imageInfo.dispWidth - 14.0
				}

				// q dispW 0 0 dispH imgX currY cm /Im1 Do Q
				stream.WriteString(fmt.Sprintf("q %.2f 0 0 %.2f %.2f %.2f cm /Im1 Do Q\n",
					item.imageInfo.dispWidth, item.imageInfo.dispHeight, imgX, currY))

				currY -= item.afterSpace
				stream.WriteString("BT\n")
			}

		case "barcode":
			stream.WriteString("ET\n")
			currY -= item.beforeSpace + item.fontSize

			modules := EncodeCode128B(item.text)
			if len(modules) > 0 {
				moduleWidth := (printableWidth - 4.0) / float64(len(modules))
				if moduleWidth > 1.2 {
					moduleWidth = 1.2
				}
				totalBarcodeWidth := float64(len(modules)) * moduleWidth
				startX := 14.0
				if printableWidth > totalBarcodeWidth {
					startX = 14.0 + (printableWidth-totalBarcodeWidth)/2.0
				}

				stream.WriteString("0.00 0.00 0.00 rg\n")
				bX := startX
				for _, isBar := range modules {
					if isBar {
						stream.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re f\n", bX, currY, moduleWidth, item.fontSize))
					}
					bX += moduleWidth
				}
			}

			currY -= item.afterSpace
			stream.WriteString("BT\n")

		case "key_value":
			// Turunkan Y sebelum menggambar teks baseline
			currY -= (item.beforeSpace + item.fontSize)

			stream.WriteString(fmt.Sprintf("%s %.1f Tf\n", item.fontName, item.fontSize))

			// Kolom label (lebar 65pt)
			stream.WriteString(fmt.Sprintf("1 0 0 1 14.00 %.2f Tm\n", currY))
			stream.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFString(item.label)))

			// Titik dua
			stream.WriteString(fmt.Sprintf("1 0 0 1 80.00 %.2f Tm\n", currY))
			stream.WriteString("(:) Tj\n")

			// Nilai
			stream.WriteString(fmt.Sprintf("1 0 0 1 88.00 %.2f Tm\n", currY))
			stream.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFString(item.value)))

			currY -= item.afterSpace

		case "row":
			// Render baris tabel multi-kolom
			maxL := 1
			for _, cl := range item.colLines {
				if len(cl) > maxL {
					maxL = len(cl)
				}
			}

			currY -= item.beforeSpace

			for lineIdx := 0; lineIdx < maxL; lineIdx++ {
				currY -= item.fontSize + 1.5

				colX := 14.0
				for cIdx, col := range item.columns {
					colW := item.colWidths[cIdx]
					cellText := ""
					if lineIdx < len(item.colLines[cIdx]) {
						cellText = item.colLines[cIdx][lineIdx]
					}

					if cellText != "" {
						fName := item.fontName
						if col.Bold {
							fName = "/F2"
						}

						approxWidth := float64(len(cellText)) * (item.fontSize * 0.52)
						targetX := colX
						switch strings.ToLower(col.Align) {
						case "center":
							targetX = colX + (colW-approxWidth)/2.0
						case "right":
							targetX = colX + colW - approxWidth
						}

						stream.WriteString(fmt.Sprintf("%s %.1f Tf\n", fName, item.fontSize))
						stream.WriteString(fmt.Sprintf("1 0 0 1 %.2f %.2f Tm\n", targetX, currY))
						stream.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFString(cellText)))
					}

					colX += colW
				}
			}

			currY -= item.afterSpace

		default: // "text"
			// PERHITUNGAN BASELINE PRESISI: Turunkan Y setinggi font sebelum Tm Tj agar tidak menabrak baris atasnya
			currY -= (item.beforeSpace + item.fontSize)

			approxCharWidth := item.fontSize * 0.52
			textWidth := float64(len(item.text)) * approxCharWidth
			posX := 14.0

			switch strings.ToLower(item.align) {
			case "center":
				posX = (width - textWidth) / 2.0
				if posX < 8.0 {
					posX = 8.0
				}
			case "right":
				posX = width - textWidth - 14.0
			}

			stream.WriteString(fmt.Sprintf("%s %.1f Tf\n", item.fontName, item.fontSize))
			stream.WriteString(fmt.Sprintf("1 0 0 1 %.2f %.2f Tm\n", posX, currY))
			stream.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFString(item.text)))

			currY -= item.afterSpace
		}
	}

	stream.WriteString("ET\n")
	contentStream := stream.Bytes()

	// 4. Bangun Dokumen PDF v1.4 Valid dengan Font Type 1 lengkap & Image XObject
	var pdf bytes.Buffer
	var offsets []int

	pdf.WriteString("%PDF-1.4\n")

	hasImage := len(embeddedImages) > 0

	// Objek 1: Catalog
	offsets = append(offsets, pdf.Len())
	pdf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// Objek 2: Pages
	offsets = append(offsets, pdf.Len())
	pdf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// Objek 3: Page (Ukuran 80mm x pageHeight presisi)
	offsets = append(offsets, pdf.Len())
	resourceDict := "/Resources << /Font << /F1 4 0 R /F2 5 0 R /F3 6 0 R /F4 7 0 R >>"
	if hasImage {
		resourceDict += " /XObject << /Im1 9 0 R >>"
	}
	resourceDict += " >>"
	pdf.WriteString(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] %s /Contents 8 0 R >>\nendobj\n", width, pageHeight, resourceDict))

	// Objek 4: Font F1 (Helvetica Regular)
	offsets = append(offsets, pdf.Len())
	pdf.WriteString("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	// Objek 5: Font F2 (Helvetica Bold)
	offsets = append(offsets, pdf.Len())
	pdf.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>\nendobj\n")

	// Objek 6: Font F3 (Times Roman Regular)
	offsets = append(offsets, pdf.Len())
	pdf.WriteString("6 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman >>\nendobj\n")

	// Objek 7: Font F4 (Times Bold - Serif untuk Kop & Nomor Antrian)
	offsets = append(offsets, pdf.Len())
	pdf.WriteString("7 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Times-Bold >>\nendobj\n")

	// Objek 8: Contents Stream
	offsets = append(offsets, pdf.Len())
	pdf.WriteString(fmt.Sprintf("8 0 obj\n<< /Length %d >>\nstream\n", len(contentStream)))
	pdf.Write(contentStream)
	pdf.WriteString("\nendstream\nendobj\n")

	// Objek 9: Image XObject (jika ada logo)
	if hasImage {
		img := embeddedImages[0]
		offsets = append(offsets, pdf.Len())
		pdf.WriteString(fmt.Sprintf("9 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Length %d >>\nstream\n",
			img.widthDots, img.heightDots, len(img.rgbData)))
		pdf.Write(img.rgbData)
		pdf.WriteString("\nendstream\nendobj\n")
	}

	// Cross-Reference Table
	totalObjs := 8
	if hasImage {
		totalObjs = 9
	}

	xrefStart := pdf.Len()
	pdf.WriteString(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", totalObjs+1))
	for _, offset := range offsets {
		pdf.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}

	// Trailer
	pdf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", totalObjs+1, xrefStart))

	return pdf.Bytes()
}

// decodeBase64Image mengubah string Base64 gambar menjadi format RGB untuk PDF
func decodeBase64Image(data string, maxWidthPt float64) (*pdfImageItem, error) {
	idx := strings.Index(data, ",")
	rawBase64 := data
	if idx != -1 {
		rawBase64 = data[idx+1:]
	}

	rawBytes, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(rawBytes))
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Batasi ukuran tampilan di PDF agar proporsional
	dispW := float64(w) * 0.4
	dispH := float64(h) * 0.4
	if dispW > maxWidthPt {
		scale := maxWidthPt / dispW
		dispW = maxWidthPt
		dispH = dispH * scale
	}
	if dispH > 60.0 {
		scale := 60.0 / dispH
		dispH = 60.0
		dispW = dispW * scale
	}

	rgbData := make([]byte, w*h*3)
	pos := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			rgbData[pos] = c.R
			rgbData[pos+1] = c.G
			rgbData[pos+2] = c.B
			pos += 3
		}
	}

	return &pdfImageItem{
		widthDots:  w,
		heightDots: h,
		rgbData:    rgbData,
		dispWidth:  dispW,
		dispHeight: dispH,
	}, nil
}

// wrapWords memecah teks panjang menjadi beberapa baris berdasarkan batas karakter kata
func wrapWords(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var lines []string
	var current strings.Builder

	for _, w := range words {
		if current.Len() == 0 {
			current.WriteString(w)
		} else if current.Len()+1+len(w) <= maxChars {
			current.WriteString(" ")
			current.WriteString(w)
		} else {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(w)
		}
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	return lines
}

func escapePDFString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}
