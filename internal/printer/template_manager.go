package printer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"smart-print-agent/internal/model"
)

// TemplateManager mengelola penyimpanan, pembacaan, dan render template dinamis
type TemplateManager struct {
	mu          sync.RWMutex
	templateDir string
	templates   map[string]*model.TemplateDefinition
}

var (
	tmplMgrInstance *TemplateManager
	tmplMgrOnce     sync.Once
)

// NewTemplateManager membuat instance TemplateManager baru untuk direktori tertentu
func NewTemplateManager(dir string) *TemplateManager {
	if dir == "" {
		dir = "templates"
	}
	tm := &TemplateManager{
		templateDir: dir,
		templates:   make(map[string]*model.TemplateDefinition),
	}
	_ = tm.Reload()
	return tm
}

// GetTemplateManager mengembalikan instance singleton TemplateManager
func GetTemplateManager(customDir ...string) *TemplateManager {
	tmplMgrOnce.Do(func() {
		dir := "templates"
		if len(customDir) > 0 && customDir[0] != "" {
			dir = customDir[0]
		}
		tmplMgrInstance = NewTemplateManager(dir)
	})
	return tmplMgrInstance
}

// Reload membaca ulang seluruh file .json dari folder templates
func (tm *TemplateManager) Reload() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if err := os.MkdirAll(tm.templateDir, 0755); err != nil {
		return err
	}

	files, err := os.ReadDir(tm.templateDir)
	if err != nil {
		return err
	}

	tm.templates = make(map[string]*model.TemplateDefinition)
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".json") {
			filePath := filepath.Join(tm.templateDir, f.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			var def model.TemplateDefinition
			if err := json.Unmarshal(data, &def); err == nil && def.ID != "" {
				tm.templates[strings.ToLower(def.ID)] = &def
			}
		}
	}
	return nil
}

// ListTemplates mengembalikan daftar seluruh template yang tersedia
func (tm *TemplateManager) ListTemplates() []model.TemplateDefinition {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	list := make([]model.TemplateDefinition, 0, len(tm.templates))
	for _, t := range tm.templates {
		list = append(list, *t)
	}
	return list
}

// GetTemplate mengambil template berdasarkan ID
func (tm *TemplateManager) GetTemplate(id string) (*model.TemplateDefinition, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	t, exists := tm.templates[strings.ToLower(id)]
	if exists {
		cpy := *t
		return &cpy, true
	}
	return nil, false
}

// SaveTemplate menyimpan atau memperbarui template ke disk
func (tm *TemplateManager) SaveTemplate(tmpl model.TemplateDefinition) error {
	if tmpl.ID == "" {
		return fmt.Errorf("ID template tidak boleh kosong")
	}
	tmpl.ID = strings.ToLower(strings.TrimSpace(tmpl.ID))
	tmpl.UpdatedAt = time.Now().Format(time.RFC3339)

	filePath := filepath.Join(tm.templateDir, tmpl.ID+".json")
	data, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return err
	}

	tm.mu.Lock()
	tm.templates[tmpl.ID] = &tmpl
	tm.mu.Unlock()
	return nil
}

// DeleteTemplate menghapus template dari disk dan memori
func (tm *TemplateManager) DeleteTemplate(id string) error {
	id = strings.ToLower(strings.TrimSpace(id))
	filePath := filepath.Join(tm.templateDir, id+".json")

	_ = os.Remove(filePath)

	tm.mu.Lock()
	delete(tm.templates, id)
	tm.mu.Unlock()
	return nil
}

// RenderToPDF mengonversi template dan data masukan menjadi file PDF fisik dengan dimensi mm akurat
func (tm *TemplateManager) RenderToPDF(tmpl *model.TemplateDefinition, data map[string]interface{}, saveDir string, jobID string) (string, error) {
	if tmpl == nil {
		return "", fmt.Errorf("template nil")
	}

	now := time.Now()
	dateFolder := now.Format("2006-01-02")
	targetDir := filepath.Join(saveDir, dateFolder)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}

	if jobID == "" {
		jobID = fmt.Sprintf("JOB-%d", now.UnixNano()%1000000)
	}

	fileName := fmt.Sprintf("%s_%s_%s.pdf", tmpl.ID, jobID, now.Format("150405"))
	fullPath := filepath.Join(targetDir, fileName)

	// Hitung dimensi halaman dalam pt (1 mm = 72 / 25.4 pt = ~2.8346 pt)
	const mm2pt = 72.0 / 25.4
	widthPt := tmpl.Paper.WidthMM * mm2pt
	heightPt := tmpl.Paper.HeightMM * mm2pt
	if widthPt <= 0 {
		widthPt = 80 * mm2pt // default 80mm
	}
	if heightPt <= 0 {
		heightPt = 120 * mm2pt // default 120mm jika roll
	}

	var contentStream bytes.Buffer
	var fontIndices = map[string]int{"F1": 1, "F2": 2, "F3": 3} // Times, Helvetica, Courier

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

	// Render setiap elemen visual
	for _, el := range tmpl.Elements {
		xPt := el.X * mm2pt
		// PDF Y dihitung dari bawah ke atas
		yPt := heightPt - (el.Y * mm2pt)

		switch el.Type {
		case "text":
			textVal := interpolate(el.Content)
			if textVal == "" {
				continue
			}
			fontSize := el.FontSize
			if fontSize <= 0 {
				fontSize = 9.0
			}

			fontTag := "F1" // Times
			if strings.EqualFold(el.FontFamily, "arial") || strings.EqualFold(el.FontFamily, "helvetica") {
				fontTag = "F2"
			} else if strings.EqualFold(el.FontFamily, "courier") {
				fontTag = "F3"
			}

			safeText := strings.ReplaceAll(textVal, "\\", "\\\\")
			safeText = strings.ReplaceAll(safeText, "(", "\\(")
			safeText = strings.ReplaceAll(safeText, ")", "\\)")

			contentStream.WriteString("BT\n")
			contentStream.WriteString(fmt.Sprintf("/%s %.2f Tf\n", fontTag, fontSize))

			// Hitung alignment
			renderX := xPt
			estWidth := float64(len(textVal)) * fontSize * 0.52
			if el.Align == "center" {
				renderX = xPt - (estWidth / 2)
				if renderX < 2 {
					renderX = 2
				}
			} else if el.Align == "right" {
				renderX = xPt - estWidth
			}

			contentStream.WriteString(fmt.Sprintf("%.2f %.2f Td\n", renderX, yPt))
			contentStream.WriteString(fmt.Sprintf("(%s) Tj\n", safeText))
			contentStream.WriteString("ET\n")

		case "line", "divider":
			lineWidthPt := el.Width * mm2pt
			if lineWidthPt <= 0 {
				lineWidthPt = widthPt - (2 * xPt)
			}
			thicknessPt := el.Thickness * mm2pt
			if thicknessPt <= 0 {
				thicknessPt = 0.5
			}

			contentStream.WriteString(fmt.Sprintf("%.2f w\n", thicknessPt))
			if el.Style == "dashed" {
				contentStream.WriteString("[3 3] 0 d\n")
			} else {
				contentStream.WriteString("[] 0 d\n")
			}
			contentStream.WriteString(fmt.Sprintf("%.2f %.2f m\n", xPt, yPt))
			contentStream.WriteString(fmt.Sprintf("%.2f %.2f l\nS\n", xPt+lineWidthPt, yPt))

		case "barcode":
			barcodeVal := interpolate(el.Content)
			if barcodeVal == "" {
				continue
			}
			bcHeightPt := el.Height * mm2pt
			if bcHeightPt <= 0 {
				bcHeightPt = 20.0
			}
			bcWidthPt := el.Width * mm2pt
			if bcWidthPt <= 0 {
				bcWidthPt = widthPt - (2 * xPt)
			}

			bits := EncodeCode128B(barcodeVal)
			if len(bits) > 0 {
				moduleWidth := bcWidthPt / float64(len(bits))
				contentStream.WriteString("0 0 0 rg\n")
				for i, isBar := range bits {
					if isBar {
						bx := xPt + (float64(i) * moduleWidth)
						by := yPt - bcHeightPt
						contentStream.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re\nf\n", bx, by, moduleWidth, bcHeightPt))
					}
				}
			}
		}
	}

	// Bungkus PDF 1.4 murni
	pdfBytes := buildMinimalPDF(widthPt, heightPt, contentStream.Bytes(), fontIndices)
	if err := os.WriteFile(fullPath, pdfBytes, 0644); err != nil {
		return "", err
	}

	return fullPath, nil
}

func buildMinimalPDF(widthPt, heightPt float64, streamBytes []byte, fontIndices map[string]int) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	var offsets []int
	addObj := func() int {
		offsets = append(offsets, buf.Len())
		return len(offsets)
	}

	// 1: Catalog
	addObj()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// 2: Pages
	addObj()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// 3: Page
	addObj()
	buf.WriteString(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Contents 4 0 R /Resources << /Font << /F1 5 0 R /F2 6 0 R /F3 7 0 R >> >> >>\nendobj\n", widthPt, heightPt))

	// 4: Stream
	addObj()
	buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n", len(streamBytes)))
	buf.Write(streamBytes)
	buf.WriteString("\nendstream\nendobj\n")

	// 5: Font Times
	addObj()
	buf.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman >>\nendobj\n")

	// 6: Font Helvetica
	addObj()
	buf.WriteString("6 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	// 7: Font Courier
	addObj()
	buf.WriteString("7 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>\nendobj\n")

	// Xref
	xrefOffset := buf.Len()
	buf.WriteString(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1))
	for _, o := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", o))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xrefOffset))

	return buf.Bytes()
}
