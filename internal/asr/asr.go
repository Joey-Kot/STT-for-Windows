package asr

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"stt/internal/config"
	"stt/internal/jsonpath"
)

// Client performs ASR uploads.
type Client struct {
	cfg            config.Config
	httpClient     *http.Client
	extraConfigMap map[string]interface{}
}

// New creates a new ASR client and parses ExtraConfig.
func New(cfg config.Config, httpClient *http.Client) (*Client, error) {
	c := &Client{cfg: cfg, httpClient: httpClient}
	if cfg.ExtraConfig != "" {
		c.extraConfigMap = make(map[string]interface{})
		if err := json.Unmarshal([]byte(cfg.ExtraConfig), &c.extraConfigMap); err != nil {
			return nil, fmt.Errorf("invalid extra-config JSON: %w", err)
		}
	}
	return c, nil
}

// Transcribe uploads the audio and returns extracted text and raw JSON.
func (c *Client) Transcribe(ctx context.Context, filePath string) (string, []byte, error) {
	if c.cfg.APIEndpoint == "" {
		return "", nil, fmt.Errorf("API endpoint is empty")
	}

	try := 0
	delay := c.cfg.RetryBaseDelay
	var lastResp []byte

	for {
		try++
		ok, res := c.doUpload(ctx, filePath)
		lastResp = res
		if ok {
			text := jsonpath.ExtractTextFromResponse(res, c.cfg.TEXTPath)
			return text, res, nil
		}

		if c.cfg.UPLOAD_DEBUG {
			fmt.Printf("[upload] attempt %d failed: %s\n", try, formatResponse(res))
		}
		if try >= c.cfg.MaxRetry {
			return "", lastResp, fmt.Errorf("exceeded max retries (%d)", c.cfg.MaxRetry)
		}
		time.Sleep(time.Duration(delay * float64(time.Second)))
		delay *= 2
	}
}

func (c *Client) doUpload(ctx context.Context, filePath string) (bool, []byte) {
	if c.cfg.UPLOAD_DEBUG {
		fmt.Printf("[upload] uploading %s -> %s\n", filePath, c.cfg.APIEndpoint)
	}
	f, err := os.Open(filePath)
	if err != nil {
		return false, []byte(fmt.Sprintf("open file error: %v", err))
	}
	defer f.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return false, []byte(fmt.Sprintf("create form file error: %v", err))
	}
	if _, err := io.Copy(part, f); err != nil {
		return false, []byte(fmt.Sprintf("copy file error: %v", err))
	}

	base := make(map[string]interface{})
	if c.cfg.Model != "" {
		base["model"] = c.cfg.Model
	}
	if c.cfg.Language != "" {
		base["language"] = c.cfg.Language
	}
	if c.cfg.Prompt != "" {
		base["prompt"] = c.cfg.Prompt
	}
	if c.extraConfigMap != nil {
		for k, v := range c.extraConfigMap {
			base[k] = v
		}
	}
	for k, v := range base {
		switch val := v.(type) {
		case string:
			_ = writer.WriteField(k, val)
		case bool:
			_ = writer.WriteField(k, fmt.Sprintf("%v", val))
		case float64:
			_ = writer.WriteField(k, fmt.Sprintf("%v", val))
		case int:
			_ = writer.WriteField(k, fmt.Sprintf("%v", val))
		default:
			if b, err := json.Marshal(val); err == nil {
				_ = writer.WriteField(k, string(b))
			} else {
				_ = writer.WriteField(k, fmt.Sprintf("%v", val))
			}
		}
	}
	_ = writer.Close()

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(c.cfg.RequestTimeout) * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.APIEndpoint, body)
	if err != nil {
		return false, []byte(fmt.Sprintf("new request error: %v", err))
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	req.Header.Set("User-Agent", "stt-go-client/1.0")

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if c.cfg.UPLOAD_DEBUG {
		fmt.Printf("[upload] request duration: %v\n", elapsed)
	}

	if err != nil {
		return false, []byte(fmt.Sprintf("request error: %v", err))
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return false, respBody
	}
	return true, respBody
}

func formatResponse(b []byte) string {
	if len(b) == 0 {
		return "<empty>"
	}
	const maxText = 1000
	const maxBin = 256

	if utf8.Valid(b) {
		s := string(b)
		if len(s) > maxText {
			return fmt.Sprintf("%s... (truncated, total %d bytes)", s[:maxText], len(b))
		}
		return s
	}

	if len(b) > maxBin {
		return fmt.Sprintf("<binary %d bytes, prefix hex: %s...>", len(b), hex.EncodeToString(b[:maxBin]))
	}
	return fmt.Sprintf("<binary %d bytes, hex: %s>", len(b), hex.EncodeToString(b))
}
