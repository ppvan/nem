package extractor

import (
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"
)

type adaptiveDownloader struct {
	client        *http.Client
	referer       string
	delay         time.Duration
	minDelay      time.Duration
	maxDelay      time.Duration
	successStreak int
}

func newAdaptiveDownloader(client *http.Client, referer string) *adaptiveDownloader {
	return &adaptiveDownloader{
		client:   client,
		referer:  referer,
		delay:    250 * time.Millisecond,
		minDelay: 120 * time.Millisecond,
		maxDelay: 2 * time.Second,
	}
}

func (ad *adaptiveDownloader) downloadSegments(urls []string, w io.Writer, callback func(float64)) error {
	for i, url := range urls {
		if err := ad.downloadSegment(url, w); err != nil {
			return fmt.Errorf("failed to download segment %d/%d: %w", i+1, len(urls), err)
		}

		if callback != nil {
			callback(float64(i+1) / float64(len(urls)))
		}

		ad.updateDelay()
		time.Sleep(ad.delay)
	}
	return nil
}

func (ad *adaptiveDownloader) downloadSegment(url string, w io.Writer) error {
	const maxRetries = 10

	for range maxRetries {
		content, shouldRetry, err := ad.fetchSegment(url)
		if err != nil && !shouldRetry {
			return err
		}

		if shouldRetry {
			ad.applyBackoff()
			continue
		}

		// Extract and write data
		segments, err := extractDataAfterIEND(content)
		if err != nil {
			return fmt.Errorf("failed to extract segments: %w", err)
		}

		if _, err := w.Write(segments); err != nil {
			return fmt.Errorf("failed to write segments: %w", err)
		}

		ad.successStreak++
		return nil
	}

	return fmt.Errorf("max retries exceeded for URL: %s", url)
}

func (ad *adaptiveDownloader) fetchSegment(url string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}

	req.Header.Set("Referer", ad.referer)
	req.Header.Set("User-Agent", USER_AGENT)

	resp, err := ad.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	// Handle rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, true, nil // Retry with backoff
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	return content, false, nil
}

func (ad *adaptiveDownloader) applyBackoff() {
	// Multiplicative backoff with jitter
	ad.delay = min(time.Duration(float64(ad.delay)*1.8), ad.maxDelay)
	jitter := ad.delay/2 + time.Duration(rand.Float64()*float64(ad.delay/2))
	time.Sleep(jitter)
	ad.successStreak = 0
}

func (ad *adaptiveDownloader) updateDelay() {
	if ad.successStreak >= 5 {
		ad.delay -= 10 * time.Millisecond
		if ad.delay < ad.minDelay {
			ad.delay = ad.minDelay
		}
		ad.successStreak = 0
	}
}
