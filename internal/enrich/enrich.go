// Package enrich transcreve áudio e descreve imagem recebidos, usando uma API
// compatível com a OpenAI (OpenAI, Groq, etc. — só muda AI_BASE_URL). O
// resultado entra no payload do evento (`transcript` / `imageCaption`), então
// todo consumidor (webhook, n8n, WS) ganha texto de graça.
package enrich

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	http    *http.Client
	base    string // ex.: https://api.openai.com/v1
	key     string
	whisper string // modelo de transcrição
	vision  string // modelo de visão
}

func New(baseURL, apiKey, whisperModel, visionModel string) *Client {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if whisperModel == "" {
		whisperModel = "whisper-1"
	}
	if visionModel == "" {
		visionModel = "gpt-4o-mini"
	}
	return &Client{
		http: &http.Client{Timeout: 90 * time.Second},
		base: base, key: strings.TrimSpace(apiKey),
		whisper: whisperModel, vision: visionModel,
	}
}

func (c *Client) Enabled() bool { return c != nil && c.key != "" }

// Transcribe → texto de um áudio (POST /audio/transcriptions, multipart).
func (c *Client) Transcribe(ctx context.Context, data []byte, mime string) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", c.whisper)
	fw, err := mw.CreateFormFile("file", "audio."+extFor(mime))
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	_ = mw.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/audio/transcriptions", &buf)
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var out struct {
		Text  string `json:"text"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if out.Error.Message != "" {
		return "", fmt.Errorf("transcribe: %s", out.Error.Message)
	}
	return strings.TrimSpace(out.Text), nil
}

// Describe → uma frase descrevendo a imagem (POST /chat/completions, visão).
func (c *Client) Describe(ctx context.Context, data []byte, mime string) (string, error) {
	if mime == "" {
		mime = "image/jpeg"
	}
	dataURI := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	body := map[string]any{
		"model":      c.vision,
		"max_tokens": 200,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "Descreva esta imagem em uma ou duas frases, em português, para um atendente que não pode vê-la. Se houver texto legível, transcreva-o."},
				{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
			},
		}},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if out.Error.Message != "" {
		return "", fmt.Errorf("describe: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("ai http %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

func extFor(mime string) string {
	switch {
	case strings.Contains(mime, "ogg"):
		return "ogg"
	case strings.Contains(mime, "mp3"), strings.Contains(mime, "mpeg"):
		return "mp3"
	case strings.Contains(mime, "wav"):
		return "wav"
	case strings.Contains(mime, "m4a"), strings.Contains(mime, "mp4"):
		return "m4a"
	default:
		return "ogg"
	}
}
