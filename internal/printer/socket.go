package printer

import (
	"fmt"
	"net"
	"time"
)

// PrintRawSocket mengirim data langsung ke Network Printer ber-IP via TCP Raw Port (biasanya 9100)
func PrintRawSocket(targetAddress string, data []byte, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 3
	}

	// Dial dengan timeout ketat agar tidak freeze jika printer mati
	conn, err := net.DialTimeout("tcp", targetAddress, time.Duration(timeoutSec)*time.Second)
	if err != nil {
		return fmt.Errorf("gagal terhubung ke network printer (%s): %w", targetAddress, err)
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(time.Duration(timeoutSec) * time.Second))

	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("gagal mengirim data ke network printer (%s): %w", targetAddress, err)
	}

	return nil
}

