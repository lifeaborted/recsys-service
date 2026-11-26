package explain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// PublicText is supplied by a caller that has verified permission to send it to
// an external provider. Keep client history and closed dataset rows out of it.
type PublicText struct {
	Text      string `json:"text"`
	SourceURL string `json:"source_url"`
	Version   string `json:"version"`
	Approved  bool   `json:"approved"`
}

func (input PublicText) validate() error {
	source, err := url.Parse(input.SourceURL)
	if !input.Approved || strings.TrimSpace(input.Text) == "" || strings.TrimSpace(input.Version) == "" ||
		err != nil || source.Hostname() == "" || source.User != nil || (source.Scheme != "https" && source.Scheme != "http") {
		return fmt.Errorf("provider requires an approved, versioned public text")
	}
	return nil
}

func InputHash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

func configFromEnv(prefix string) Config {
	return Config{BaseURL: os.Getenv(prefix + "_BASE_URL"), APIKey: os.Getenv(prefix + "_API_KEY"), Model: os.Getenv(prefix + "_MODEL")}
}

const timeout = 15 * time.Second

func request(ctx context.Context, config Config, endpoint string, payload, response any) error {
	base, err := url.Parse(config.BaseURL)
	if err != nil || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" ||
		(base.Scheme != "https" && base.Scheme != "http") || strings.TrimSpace(config.Model) == "" {
		return fmt.Errorf("invalid provider configuration")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base.String(), "/")+endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cannot build provider request")
	}
	req.Header.Set("Content-Type", "application/json")
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("provider request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("provider returned HTTP %d", res.StatusCode)
	}
	const maxResponse = 32 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return fmt.Errorf("provider response exceeds limit or cannot be read")
	}
	if err := json.Unmarshal(data, response); err != nil {
		return fmt.Errorf("invalid provider response")
	}
	return nil
}
