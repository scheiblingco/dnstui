package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (p *cfProvider) newRequest(ctx context.Context, method, urlPath string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.settings.BaseURL+urlPath, bodyReader)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	if p.settings.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.settings.APIToken)
	} else {
		req.Header.Set("X-Auth-Key", p.settings.APIKey)
		req.Header.Set("X-Auth-Email", p.settings.APIEmail)
	}

	return req, nil
}

func (p *cfProvider) doRequest(ctx context.Context, method, urlPath string, body any) ([]byte, int, error) {
	const maxAttempts = 3

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := p.newRequest(ctx, method, urlPath, body)
		if err != nil {
			return nil, 0, err
		}

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP %s %s: %w", method, urlPath, err)
			continue
		}

		// Limit reads to 10 MB to guard against runaway responses.
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("reading response body: %w", readErr)
			continue
		}

		// Retry on server-side errors only.
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error %d on %s %s", resp.StatusCode, method, urlPath)
			continue
		}

		return b, resp.StatusCode, nil
	}

	return nil, 0, lastErr
}

func pagedPath(basePath string, page int) string {
	sep := "?"
	if strings.Contains(basePath, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%spage=%d&per_page=100", basePath, sep, page)
}

func getAllPages[T any](ctx context.Context, p *cfProvider, basePath string) ([]T, error) {
	var all []T

	for page := 1; ; page++ {
		b, _, err := p.doRequest(ctx, http.MethodGet, pagedPath(basePath, page), nil)
		if err != nil {
			return nil, err
		}

		var resp cfResponse[[]T]
		if err := json.Unmarshal(b, &resp); err != nil {
			return nil, fmt.Errorf("decoding response from %s: %w", basePath, err)
		}
		if !resp.Success {
			return nil, apiErrors(resp.Errors)
		}

		all = append(all, resp.Result...)

		if resp.ResultInfo == nil || page >= resp.ResultInfo.TotalPages {
			break
		}
	}

	return all, nil
}

func doJSON[T any](ctx context.Context, p *cfProvider, method, urlPath string, body any) (T, error) {
	b, _, err := p.doRequest(ctx, method, urlPath, body)
	if err != nil {
		var zero T
		return zero, err
	}

	var resp cfResponse[T]
	if err := json.Unmarshal(b, &resp); err != nil {
		var zero T
		return zero, fmt.Errorf("decoding response from %s %s: %w", method, urlPath, err)
	}
	if !resp.Success {
		var zero T
		return zero, apiErrors(resp.Errors)
	}

	return resp.Result, nil
}
