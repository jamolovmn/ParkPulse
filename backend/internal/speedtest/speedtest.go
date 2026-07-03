// Package speedtest serverning internet tezligini o'lchaydi (fast.com kabi,
// lekin server tomonda — parking serverining haqiqiy kanali o'lchanadi).
// Manba: Cloudflare speed endpointlari (speed.cloudflare.com).
package speedtest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	base       = "https://speed.cloudflare.com"
	downBytes  = 26_214_400 // 25 MiB
	upBytes    = 8_388_608  // 8 MiB
	pingRounds = 3
)

type Result struct {
	PingMs       float64 `json:"ping_ms"`
	DownloadMbps float64 `json:"download_mbps"`
	UploadMbps   float64 `json:"upload_mbps"`
}

func Run(ctx context.Context) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client := &http.Client{}

	ping, err := measurePing(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	down, err := measureDownload(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	up, err := measureUpload(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	return &Result{PingMs: ping, DownloadMbps: down, UploadMbps: up}, nil
}

func measurePing(ctx context.Context, c *http.Client) (float64, error) {
	best := 0.0
	for i := 0; i < pingRounds; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/__down?bytes=0", nil)
		if err != nil {
			return 0, err
		}
		t0 := time.Now()
		resp, err := c.Do(req)
		if err != nil {
			return 0, err
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		ms := float64(time.Since(t0)) / float64(time.Millisecond)
		if best == 0 || ms < best {
			best = ms
		}
	}
	return best, nil
}

func measureDownload(ctx context.Context, c *http.Client) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/__down?bytes=%d", base, downBytes), nil)
	if err != nil {
		return 0, err
	}
	t0 := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}
	return mbps(n, time.Since(t0)), nil
}

func measureUpload(ctx context.Context, c *http.Client) (float64, error) {
	payload := make([]byte, upBytes)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/__up", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	t0 := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return mbps(upBytes, time.Since(t0)), nil
}

func mbps(n int64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) * 8 / d.Seconds() / 1e6
}
