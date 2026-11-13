package printer

import (
	"fmt"
	"strings"

	"smart-print-agent/internal/model"
)

// ApplyTemplate mengubah request berbasis template dan data menjadi daftar PrintLine terstruktur
func ApplyTemplate(templateName string, data map[string]interface{}) []model.PrintLine {
	if data == nil {
		data = make(map[string]interface{})
	}

	switch strings.ToLower(templateName) {
	case "antrian":
		return buildAntrianTemplate(data)
	case "resep_racikan":
		return buildResepRacikanTemplate(data)
	case "resep_non_racikan":
		return buildResepNonRacikanTemplate(data)
	case "label_pasien", "label", "gelang":
		return buildLabelPasienTemplate(data)
	default:
		return nil
	}
}

// buildAntrianTemplate menyusun struk antrian dengan tipografi serif elegan dan spasi presisi
func buildAntrianTemplate(d map[string]interface{}) []model.PrintLine {
	var lines []model.PrintLine

	// 1. Logo jika ada
	if logo, ok := d["logo"].(string); ok && logo != "" {
		lines = append(lines, model.PrintLine{
			Type:  "image",
			Data:  logo,
			Align: "center",
		})
	}

	// 2. Kop Rumah Sakit
	instansi := getString(d, "instansi", "RSU ASHAVIN")
	lines = append(lines, model.PrintLine{
		Text:  instansi,
		Align: "center",
		Bold:  true,
		Size:  "large",
		Font:  "serif",
	})

	alamat := getString(d, "alamat", "Jln. Raya Jepara - Bangsri Km. 09, No. 9 Ds. Jambu, Kec.\nMlonggo, Kab. Jepara, Jepara\nTelp: 082221455308\nEmail: rsuashavin5pandawagrup@gmail\nWebsite:")
	lines = append(lines, model.PrintLine{
		Text:  alamat,
		Align: "center",
		Size:  "small",
	})

	// 3. Divider
	lines = append(lines, model.PrintLine{Type: "divider", Style: "solid"})

	// 4. Waktu / Tanggal
	waktu := getString(d, "waktu", getString(d, "tanggal", ""))
	if waktu != "" {
		lines = append(lines, model.PrintLine{
			Text:  fmt.Sprintf("Tanggal : %s", waktu),
			Align: "left",
			Bold:  true,
			Size:  "normal",
		})
	}

	// 5. Nomor Antrian
	lines = append(lines, model.PrintLine{
		Text:  "Nomor Antrian :",
		Align: "center",
		Size:  "normal",
		Font:  "serif",
	})

	nomor := getString(d, "nomor", getString(d, "nomor_antrian", "A-001"))
	lines = append(lines, model.PrintLine{
		Text:  nomor,
		Align: "center",
		Bold:  true,
		Size:  "extra_large",
		Font:  "serif",
	})

	kategori := getString(d, "kategori", getString(d, "tipe_antrian", "UMUM"))
	lines = append(lines, model.PrintLine{
		Text:  kategori,
		Align: "center",
		Bold:  true,
		Size:  "large",
	})

	// 6. Footer Antrian
	pesan := getString(d, "pesan", "Silahkan menunggu nomor Anda dipanggil")
	lines = append(lines, model.PrintLine{
		Text:  pesan,
		Align: "center",
		Size:  "small",
	})

	sisaVal, hasSisa := d["sisa"]
	if hasSisa {
		lines = append(lines, model.PrintLine{
			Text:  fmt.Sprintf("Antrian yang belum dipanggil %v Orang", sisaVal),
			Align: "center",
			Size:  "small",
		})
	}

	lines = append(lines, model.PrintLine{Type: "cut"})
	return lines
}

// buildResepNonRacikanTemplate menyusun format resep obat jadi (tabel terstruktur)
func buildResepNonRacikanTemplate(d map[string]interface{}) []model.PrintLine {
	var lines []model.PrintLine

	instansi := getString(d, "instansi", "RSU ASHAVIN - FARMASI")
	lines = append(lines, model.PrintLine{
		Text:  instansi,
		Align: "center",
		Bold:  true,
		Size:  "large",
		Font:  "serif",
	})
	lines = append(lines, model.PrintLine{
		Text:  "INSTALASI FARMASI RAWAT JALAN",
		Align: "center",
		Size:  "small",
	})
	lines = append(lines, model.PrintLine{Type: "divider", Style: "double"})

	// Metadata Resep
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "No. Resep", Value: getString(d, "no_resep", "-")})
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "Tanggal", Value: getString(d, "tanggal", "-")})
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "No. RM", Value: getString(d, "no_rm", "-")})
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "Pasien", Value: getString(d, "pasien", "-")})
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "Dokter", Value: getString(d, "dokter", "-")})
	if poli := getString(d, "poli", ""); poli != "" {
		lines = append(lines, model.PrintLine{Type: "key_value", Label: "Poli/Unit", Value: poli})
	}

	lines = append(lines, model.PrintLine{Type: "divider", Style: "solid"})
	lines = append(lines, model.PrintLine{
		Text:  "RINCIAN OBAT JADI (NON-RACIKAN)",
		Align: "center",
		Bold:  true,
		Size:  "normal",
	})
	lines = append(lines, model.PrintLine{Type: "divider", Style: "dashed"})

	// Header Tabel Obat: [Nama Obat | Signa / Aturan | Jml]
	lines = append(lines, model.PrintLine{
		Type: "row",
		Bold: true,
		Columns: []model.TableColumn{
			{Text: "Nama Obat", Width: 46, Align: "left", Bold: true},
			{Text: "Signa / Aturan", Width: 38, Align: "left", Bold: true},
			{Text: "Jml", Width: 16, Align: "right", Bold: true},
		},
	})
	lines = append(lines, model.PrintLine{Type: "divider", Style: "dashed"})

	// Daftar Obat
	if obatList, ok := d["obat"].([]interface{}); ok {
		for i, item := range obatList {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			nama := getString(m, "nama", "-")
			signa := getString(m, "signa", "-")
			qty := getString(m, "qty", getString(m, "jumlah", "-"))

			lines = append(lines, model.PrintLine{
				Type: "row",
				Columns: []model.TableColumn{
					{Text: fmt.Sprintf("%d. %s", i+1, nama), Width: 46, Align: "left", Bold: true},
					{Text: signa, Width: 38, Align: "left"},
					{Text: qty, Width: 16, Align: "right", Bold: true},
				},
			})

			// Catatan khusus obat jika ada
			if catatan := getString(m, "catatan", ""); catatan != "" {
				lines = append(lines, model.PrintLine{
					Text:  fmt.Sprintf("   (%s)", catatan),
					Align: "left",
					Size:  "small",
				})
			}
		}
	}

	lines = append(lines, model.PrintLine{Type: "divider", Style: "solid"})
	catatanApotek := getString(d, "catatan_apotek", "Simpan obat di tempat sejuk dan kering, jauhkan dari jangkauan anak-anak.")
	lines = append(lines, model.PrintLine{
		Text:  catatanApotek,
		Align: "center",
		Size:  "small",
	})
	lines = append(lines, model.PrintLine{Type: "cut"})

	return lines
}

// buildResepRacikanTemplate menyusun format resep racikan kompleks (Puyer / Salep / Kapsul)
func buildResepRacikanTemplate(d map[string]interface{}) []model.PrintLine {
	var lines []model.PrintLine

	instansi := getString(d, "instansi", "RSU ASHAVIN - FARMASI")
	lines = append(lines, model.PrintLine{
		Text:  instansi,
		Align: "center",
		Bold:  true,
		Size:  "large",
		Font:  "serif",
	})
	lines = append(lines, model.PrintLine{
		Text:  "INSTALASI FARMASI RAWAT JALAN",
		Align: "center",
		Size:  "small",
	})
	lines = append(lines, model.PrintLine{Type: "divider", Style: "double"})

	// Metadata Resep
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "No. Resep", Value: getString(d, "no_resep", "-")})
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "Tanggal", Value: getString(d, "tanggal", "-")})
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "No. RM", Value: getString(d, "no_rm", "-")})
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "Pasien", Value: getString(d, "pasien", "-")})
	lines = append(lines, model.PrintLine{Type: "key_value", Label: "Dokter", Value: getString(d, "dokter", "-")})

	lines = append(lines, model.PrintLine{Type: "divider", Style: "solid"})
	lines = append(lines, model.PrintLine{
		Text:  "LEMBAR RACIKAN OBAT (FARMASI)",
		Align: "center",
		Bold:  true,
		Size:  "normal",
	})

	// Daftar Racikan
	if racikanList, ok := d["racikan"].([]interface{}); ok {
		for i, item := range racikanList {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			lines = append(lines, model.PrintLine{Type: "divider", Style: "dashed"})

			kode := getString(m, "kode", fmt.Sprintf("[R/ %d]", i+1))
			nama := getString(m, "nama", "Racikan")
			bentuk := getString(m, "bentuk", "Pulveres (Puyer)")
			jumlah := getString(m, "jumlah", "10 Bungkus")
			signa := getString(m, "signa", "-")

			// Header Racikan
			lines = append(lines, model.PrintLine{
				Text:  fmt.Sprintf("%s %s", kode, strings.ToUpper(nama)),
				Align: "left",
				Bold:  true,
				Size:  "normal",
			})
			lines = append(lines, model.PrintLine{
				Text:  fmt.Sprintf("Sediaan: %s | Jml: %s", bentuk, jumlah),
				Align: "left",
				Size:  "small",
			})
			lines = append(lines, model.PrintLine{
				Text:  fmt.Sprintf("Signa  : %s", signa),
				Align: "left",
				Bold:  true,
				Size:  "normal",
			})

			// Tabel Komposisi Bahan Racikan
			lines = append(lines, model.PrintLine{
				Text:  "Komposisi Bahan :",
				Align: "left",
				Size:  "small",
			})

			if komposisiList, ok := m["komposisi"].([]interface{}); ok {
				for _, komp := range komposisiList {
					km, ok := komp.(map[string]interface{})
					if !ok {
						continue
					}
					obatNama := getString(km, "obat", "-")
					dosis := getString(km, "dosis", "-")
					jml := getString(km, "jumlah", "-")

					lines = append(lines, model.PrintLine{
						Type: "row",
						Columns: []model.TableColumn{
							{Text: fmt.Sprintf(" • %s", obatNama), Width: 52, Align: "left"},
							{Text: dosis, Width: 26, Align: "right"},
							{Text: jml, Width: 22, Align: "right", Bold: true},
						},
					})
				}
			}
		}
	}

	lines = append(lines, model.PrintLine{Type: "divider", Style: "solid"})
	catatan := getString(d, "catatan_apotek", "Masa simpan racikan (Beyond Use Date / BUD) maks. 30 hari sejak dibuat.")
	lines = append(lines, model.PrintLine{
		Text:  catatan,
		Align: "center",
		Size:  "small",
	})
	lines = append(lines, model.PrintLine{Type: "cut"})

	return lines
}

// buildLabelPasienTemplate menyusun label barcode pasien / Rekam Medis (ukuran 50x30mm)
func buildLabelPasienTemplate(d map[string]interface{}) []model.PrintLine {
	var lines []model.PrintLine

	nama := getString(d, "nama", getString(d, "pasien", "NANI"))
	lines = append(lines, model.PrintLine{
		Text: strings.ToUpper(nama),
		Bold: true,
		Size: "large",
		Font: "serif",
	})

	noMR := getString(d, "no_mr", getString(d, "no_rm", "00150515"))
	tglLahir := getString(d, "tgl_lahir", "29/01/1934")
	mrText := fmt.Sprintf("No. MR :    %s    -    %s", noMR, tglLahir)
	lines = append(lines, model.PrintLine{
		Text: mrText,
		Bold: true,
		Size: "normal",
	})

	umur := getString(d, "umur", "92 tahun")
	lines = append(lines, model.PrintLine{
		Text: fmt.Sprintf("Umur   :    %s", umur),
		Size: "normal",
	})

	alamat := getString(d, "alamat", getString(d, "kp", "552 Don Roads East Jazminland, MA 46857"))
	lines = append(lines, model.PrintLine{
		Text: fmt.Sprintf("KP     :    %s", alamat),
		Size: "small",
	})

	barcodeVal := getString(d, "barcode", getString(d, "no_registrasi", "2611000008"))
	lines = append(lines, model.PrintLine{
		Type:   "barcode",
		Text:   barcodeVal,
		Height: 40,
	})

	waktu := getString(d, "waktu", getString(d, "tanggal", "2026-11-02 12:15:40"))
	lines = append(lines, model.PrintLine{
		Type: "row",
		Columns: []model.TableColumn{
			{Text: barcodeVal, Width: 45, Align: "left"},
			{Text: waktu, Width: 55, Align: "right"},
		},
	})

	return lines
}

func getString(m map[string]interface{}, key string, def string) string {
	if val, ok := m[key]; ok && val != nil {
		s := fmt.Sprintf("%v", val)
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return def
}

