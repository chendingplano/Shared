package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

type ImageStyle string

const (
	ImageStyleOpenAICompatible ImageStyle = "openai"
	ImageStyleDashScope        ImageStyle = "dashscope"
)

type ImageRequest struct {
	Style               ImageStyle
	Model, Prompt, Size string
	Capture             *RequestCapture
}

type imageCaptureContextKey struct{}

// WithImageRequestCapture attaches invocation attribution for adapters whose
// legacy image-provider interface does not carry a capture parameter.
func WithImageRequestCapture(ctx context.Context, capture *RequestCapture) context.Context {
	return context.WithValue(ctx, imageCaptureContextKey{}, capture)
}

func imageRequestCapture(ctx context.Context, capture *RequestCapture) *RequestCapture {
	if capture != nil {
		return capture
	}
	if fromContext, ok := ctx.Value(imageCaptureContextKey{}).(*RequestCapture); ok {
		return fromContext
	}
	return nil
}

type ImageResponse struct {
	Data        []byte
	ContentType string
	Usage       Usage
}
type ImageClient struct {
	BaseURL, APIKey string
	HTTPClient      *http.Client
}

const (
	dashScopeSubmitResponseMaxBytes int64 = 10 << 20
	dashScopePollResponseMaxBytes   int64 = 40 << 20
)

func NewImageClient(baseURL, apiKey string, client *http.Client) *ImageClient {
	return &ImageClient{BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), APIKey: strings.TrimSpace(apiKey), HTTPClient: client}
}

func (c *ImageClient) Generate(ctx context.Context, in ImageRequest) (out ImageResponse, err error) {
	in.Capture = imageRequestCapture(ctx, in.Capture)
	started := time.Now()
	rawIn, _ := json.Marshal(map[string]any{"model": in.Model, "prompt": in.Prompt, "n": 1})
	defer func() {
		out.Usage.EventID = captureUsageRecord(ctx, Request{Capture: in.Capture}, UsageCaptureInput{Provider: ProviderOpenAICompatible, BaseURL: c.BaseURL, APIKey: c.APIKey, ModelName: in.Model, PromptName: "image_generation", CallReason: "image_generation", CallLoc: "LLM_IMG_001", RequestStartedAt: started, RequestFinishedAt: time.Now(), InputBody: rawIn, OutputBody: out.Data, ErrorMessage: errorString(err)}, loggerutil.CreateDefaultLogger("20260920-201"))
	}()
	if c.BaseURL == "" || c.APIKey == "" {
		return out, fmt.Errorf("image generation provider is not configured")
	}
	if in.Style == ImageStyleDashScope {
		return c.dashScope(ctx, in)
	}
	return c.openAI(ctx, in)
}
func (c *ImageClient) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 125 * time.Second}
}
func (c *ImageClient) openAI(ctx context.Context, in ImageRequest) (ImageResponse, error) {
	body, _ := json.Marshal(map[string]any{"model": in.Model, "prompt": in.Prompt, "n": 1})
	endpoint := c.BaseURL + "/v1/images/generations"
	if strings.HasSuffix(c.BaseURL, "/v1") {
		endpoint = c.BaseURL + "/images/generations"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ImageResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return ImageResponse{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return ImageResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ImageResponse{}, fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return ImageResponse{}, err
	}
	if len(parsed.Data) == 0 {
		return ImageResponse{}, fmt.Errorf("provider returned no image data")
	}
	if parsed.Data[0].B64JSON != "" {
		data, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
		return ImageResponse{Data: data, ContentType: "image/png"}, err
	}
	return c.fetch(ctx, parsed.Data[0].URL)
}
func (c *ImageClient) dashScope(ctx context.Context, in ImageRequest) (ImageResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ImageResponse{}, fmt.Errorf("invalid image generation base URL")
	}
	host := u.Scheme + "://" + u.Host
	body, _ := json.Marshal(map[string]any{"model": in.Model, "input": map[string]any{"prompt": in.Prompt}, "parameters": map[string]any{"size": firstImageNonEmpty(in.Size, "1024*1024"), "n": 1}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/api/v1/services/aigc/text2image/image-synthesis", bytes.NewReader(body))
	if err != nil {
		return ImageResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")
	resp, err := c.client().Do(req)
	if err != nil {
		return ImageResponse{}, err
	}
	raw, readErr := readImageResponse(resp.Body, dashScopeSubmitResponseMaxBytes, "DashScope submit response too large")
	resp.Body.Close()
	if readErr != nil {
		return ImageResponse{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ImageResponse{}, fmt.Errorf("DashScope submit returned status %d", resp.StatusCode)
	}
	var task struct {
		Output struct {
			TaskID     string `json:"task_id"`
			TaskStatus string `json:"task_status"`
			Results    []struct {
				URL string `json:"url"`
			} `json:"results"`
			Message string `json:"message"`
		} `json:"output"`
	}
	if err = json.Unmarshal(raw, &task); err != nil {
		return ImageResponse{}, err
	}
	if task.Output.TaskID == "" {
		return ImageResponse{}, fmt.Errorf("DashScope response did not include a task id")
	}
	for {
		select {
		case <-ctx.Done():
			return ImageResponse{}, ctx.Err()
		case <-time.After(3 * time.Second):
		}
		poll, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/api/v1/tasks/"+url.PathEscape(task.Output.TaskID), nil)
		if err != nil {
			return ImageResponse{}, err
		}
		poll.Header.Set("Authorization", "Bearer "+c.APIKey)
		r, err := c.client().Do(poll)
		if err != nil {
			return ImageResponse{}, err
		}
		raw, readErr = readImageResponse(r.Body, dashScopePollResponseMaxBytes, "DashScope poll response too large")
		r.Body.Close()
		if readErr != nil {
			return ImageResponse{}, readErr
		}
		if r.StatusCode < 200 || r.StatusCode >= 300 {
			return ImageResponse{}, fmt.Errorf("DashScope task returned status %d", r.StatusCode)
		}
		if err = json.Unmarshal(raw, &task); err != nil {
			return ImageResponse{}, err
		}
		if task.Output.TaskStatus == "SUCCEEDED" {
			if len(task.Output.Results) == 0 {
				return ImageResponse{}, fmt.Errorf("DashScope task succeeded without an image URL")
			}
			return c.fetch(ctx, task.Output.Results[0].URL)
		}
		if task.Output.TaskStatus == "FAILED" || task.Output.TaskStatus == "CANCELED" || task.Output.TaskStatus == "UNKNOWN" {
			return ImageResponse{}, fmt.Errorf("DashScope task %s: %s", task.Output.TaskStatus, task.Output.Message)
		}
	}
}

func readImageResponse(body io.Reader, limit int64, oversizedError string) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New(oversizedError)
	}
	return raw, nil
}
func (c *ImageClient) fetch(ctx context.Context, imageURL string) (ImageResponse, error) {
	if imageURL == "" {
		return ImageResponse{}, fmt.Errorf("provider returned no image URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return ImageResponse{}, err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return ImageResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ImageResponse{}, fmt.Errorf("image URL returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	return ImageResponse{Data: data, ContentType: firstImageNonEmpty(resp.Header.Get("Content-Type"), "image/png")}, err
}
func firstImageNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
