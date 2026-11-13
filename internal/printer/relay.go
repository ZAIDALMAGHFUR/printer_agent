package printer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"smart-print-agent/internal/model"
)

// RelayPrintJob meneruskan pekerjaan cetak ke Print Agent di PC lain
func RelayPrintJob(relayTarget string, req model.PrintRequest, timeoutSec int) (*model.PrintResponse, error) {
	if timeoutSec <= 0 {
		timeoutSec = 5
	}

	targetURL := relayTarget
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}
	if !strings.Contains(targetURL, "/printgo/cetak") {
		targetURL = strings.TrimRight(targetURL, "/") + "/printgo/cetak"
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("gagal serialize request relay: %w", err)
	}

	client := &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
	}

	httpReq, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request HTTP relay: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi agent relay (%s): %w", targetURL, err)
	}
	defer resp.Body.Close()

	var printResp model.PrintResponse
	if err := json.NewDecoder(resp.Body).Decode(&printResp); err != nil {
		return nil, fmt.Errorf("gagal decode respon dari agent relay: %w", err)
	}

	return &printResp, nil
}

