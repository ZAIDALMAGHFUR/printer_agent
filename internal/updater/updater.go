package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"smart-print-agent/internal/printer"
)

const (
	CurrentVersion = "v1.2.0"
	DefaultRepoURL = "https://github.com/ZAIDALMAGHFUR/printer_agent"
)

type GitHubContentItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"` // "file", "dir"
}

type SyncStatus struct {
	CurrentVersion string    `json:"current_version"`
	RepoURL        string    `json:"repo_url"`
	LastCheckTime  time.Time `json:"last_check_time"`
	Status         string    `json:"status"` // "idle", "syncing", "success", "error"
	Message        string    `json:"message"`
	TemplatesCount int       `json:"templates_count"`
	HasAppUpdate   bool      `json:"has_app_update"`
	LatestAppVer   string    `json:"latest_app_ver,omitempty"`
}

type UpdaterService struct {
	mu           sync.RWMutex
	repoURL      string
	templateDir  string
	status       SyncStatus
	httpClient   *http.Client
	stopChan     chan struct{}
	syncInterval time.Duration
}

var (
	updaterInstance *UpdaterService
	updaterOnce     sync.Once
)

// resolveTemplateDir mencari folder templates yang valid
func resolveTemplateDir(dir string) string {
	candidates := []string{dir}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, dir),
			filepath.Join(exeDir, "..", dir),
		)
	}
	for _, p := range candidates {
		if stat, err := os.Stat(p); err == nil && stat.IsDir() {
			return p
		}
	}
	return dir
}

// GetUpdater mengembalikan instance singleton UpdaterService
func GetUpdater(repoURL string, templateDir string) *UpdaterService {
	updaterOnce.Do(func() {
		if repoURL == "" {
			repoURL = DefaultRepoURL
		}
		if templateDir == "" {
			templateDir = "templates"
		}
		resolvedDir := resolveTemplateDir(templateDir)
		updaterInstance = &UpdaterService{
			repoURL:      repoURL,
			templateDir:  resolvedDir,
			httpClient:   &http.Client{Timeout: 15 * time.Second},
			syncInterval: 60 * time.Minute,
			stopChan:     make(chan struct{}),
			status: SyncStatus{
				CurrentVersion: CurrentVersion,
				RepoURL:        repoURL,
				Status:         "idle",
				Message:        "Siap melakukan sinkronisasi dengan GitHub",
			},
		}
	})
	return updaterInstance
}

// GetStatus mengembalikan status sinkronisasi saat ini
func (u *UpdaterService) GetStatus() SyncStatus {
	u.mu.RLock()
	defer u.mu.RUnlock()
	st := u.status
	st.TemplatesCount = len(printer.GetTemplateManager().ListTemplates())
	return st
}

// StartBackgroundSync memulai background timer untuk sinkronisasi berkala
func (u *UpdaterService) StartBackgroundSync(intervalMinutes int) {
	if intervalMinutes > 0 {
		u.syncInterval = time.Duration(intervalMinutes) * time.Minute
	}

	go func() {
		// Jalankan sekali saat startup setelah 5 detik
		time.Sleep(5 * time.Second)
		_ = u.SyncTemplates()

		ticker := time.NewTicker(u.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = u.SyncTemplates()
			case <-u.stopChan:
				return
			}
		}
	}()
}

// Stop menghentikan background worker
func (u *UpdaterService) Stop() {
	close(u.stopChan)
}

// SyncTemplates mendownload template terbaru dari repository GitHub
func (u *UpdaterService) SyncTemplates() error {
	u.mu.Lock()
	u.status.Status = "syncing"
	u.status.LastCheckTime = time.Now()
	u.status.Message = "Memeriksa template di " + u.repoURL
	u.mu.Unlock()

	// Ekstrak owner & repo dari https://github.com/ZAIDALMAGHFUR/printer_agent
	parts := strings.Split(strings.TrimPrefix(u.repoURL, "https://github.com/"), "/")
	if len(parts) < 2 {
		err := fmt.Errorf("URL repository tidak valid: %s", u.repoURL)
		u.setStatusError(err)
		return err
	}
	owner := parts[0]
	repo := parts[1]

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/templates", owner, repo)
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "SmartPrintAgent/"+CurrentVersion)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		// Coba fallback langsung ke raw.githubusercontent.com untuk template standar
		fallbackErr := u.fallbackRawSync(owner, repo)
		if fallbackErr == nil {
			u.setStatusSuccess("Berhasil sinkronisasi template via GitHub Raw")
			return nil
		}
		u.setStatusError(fmt.Errorf("gagal terhubung ke GitHub: %w", err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Repo mungkin baru dibuat atau belum ada folder templates/ di GitHub, coba fallback raw
		fallbackErr := u.fallbackRawSync(owner, repo)
		if fallbackErr == nil {
			u.setStatusSuccess("Berhasil sinkronisasi template via GitHub Raw")
			return nil
		}
		msg := fmt.Sprintf("GitHub API returned %d. Pastikan repo %s/%s publik dan memiliki folder templates/", resp.StatusCode, owner, repo)
		errObj := fmt.Errorf("%s", msg)
		u.setStatusError(errObj)
		return errObj
	}

	var items []GitHubContentItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		u.setStatusError(err)
		return err
	}

	_ = os.MkdirAll(u.templateDir, 0755)
	downloadedCount := 0

	for _, item := range items {
		if item.Type == "file" && strings.HasSuffix(strings.ToLower(item.Name), ".json") {
			if item.DownloadURL == "" {
				item.DownloadURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/templates/%s", owner, repo, item.Name)
			}
			if err := u.downloadFile(item.DownloadURL, filepath.Join(u.templateDir, item.Name)); err == nil {
				downloadedCount++
			}
		}
	}

	// Muat ulang template ke memory
	_ = printer.GetTemplateManager().Reload()

	msg := fmt.Sprintf("Berhasil sinkronisasi: %d template diperbarui dari GitHub", downloadedCount)
	log.Printf("[UPDATER] %s", msg)
	u.setStatusSuccess(msg)
	return nil
}

func (u *UpdaterService) fallbackRawSync(owner, repo string) error {
	standardTemplates := []string{"label_pasien.json", "antrian.json", "bukti_pendaftaran.json", "gelang_pasien.json"}
	successCount := 0

	for _, name := range standardTemplates {
		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/templates/%s", owner, repo, name)
		destPath := filepath.Join(u.templateDir, name)
		if err := u.downloadFile(rawURL, destPath); err == nil {
			successCount++
		}
	}

	if successCount > 0 {
		_ = printer.GetTemplateManager().Reload()
		return nil
	}
	return fmt.Errorf("fallback sync gagal atau file belum tersedia di main branch")
}

func (u *UpdaterService) downloadFile(url string, destPath string) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "SmartPrintAgent/"+CurrentVersion)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download gagal: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Validasi JSON sebelum menyimpan
	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		return fmt.Errorf("konten bukan JSON valid: %w", err)
	}

	return os.WriteFile(destPath, body, 0644)
}

func (u *UpdaterService) setStatusSuccess(msg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status.Status = "success"
	u.status.Message = msg
	u.status.LastCheckTime = time.Now()
}

func (u *UpdaterService) setStatusError(err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status.Status = "error"
	u.status.Message = err.Error()
	u.status.LastCheckTime = time.Now()
	log.Printf("[UPDATER-ERROR] %v", err)
}
