package main

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"syscall"
	"time"
)

const thumbnailFetchTimeout = 10 * time.Second

func newHiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	return cmd
}

func launchMPV(url string) error {
	return newHiddenCmd("mpv", url).Start()
}

func fetchThumbnail(url string) ([]byte, error) {
	if url == "" {
		return nil, nil
	}

	client := http.Client{Timeout: thumbnailFetchTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch thumbnail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch thumbnail: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read thumbnail: %w", err)
	}
	return data, nil
}
