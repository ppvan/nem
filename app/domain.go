package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	domainShortURL       = "https://bit.ly/animevietsubtv"
	fallbackDomain       = "https://animevietsub.work"
	domainResolveTimeout = 8 * time.Second
)

func resolveDomain(ctx context.Context, shortURL string) (string, error) {
	var lastHop string

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			lastHop = req.URL.String()
			return nil
		},
	}

	do := func(method string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, shortURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("User-Agent",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "+
				"(KHTML, like Gecko) Chrome/126.0 Safari/537.36")
		return client.Do(req)
	}

	resp, err := do(http.MethodHead)
	if resp != nil {
		resp.Body.Close()
	}

	if err != nil || resp.StatusCode == http.StatusMethodNotAllowed ||
		resp.StatusCode == http.StatusNotImplemented {
		if resp2, err2 := do(http.MethodGet); resp2 != nil {
			resp2.Body.Close()
			resp, err = resp2, err2
		}
	}
	if err == nil && resp != nil {
		lastHop = resp.Request.URL.String()
	}

	if lastHop == "" {
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", shortURL, err)
		}
		return "", fmt.Errorf("resolve %s: no redirect", shortURL)
	}
	return originOf(lastHop, shortURL)
}

func originOf(raw, shortURL string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unexpected scheme %q in %s", u.Scheme, raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("no host in %s", raw)
	}

	if short, err := url.Parse(shortURL); err == nil &&
		strings.EqualFold(u.Host, short.Host) {
		return "", fmt.Errorf("%s did not redirect anywhere", shortURL)
	}
	return u.Scheme + "://" + u.Host, nil
}

func animeDomainCandidates() []string {
	ctx, cancel := context.WithTimeout(context.Background(), domainResolveTimeout)
	defer cancel()

	candidates := make([]string, 0, 2)
	if d, err := resolveDomain(ctx, domainShortURL); err == nil {
		candidates = append(candidates, d)
	}
	if len(candidates) == 0 || !strings.EqualFold(candidates[0], fallbackDomain) {
		candidates = append(candidates, fallbackDomain)
	}
	return candidates
}
