package main

// IA grátis do GitHub (GitHub Models): modelos fortes como GPT-4.1, com o token da sua conta GitHub.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ghEndpoint       = "https://models.github.ai/inference/chat/completions"
	ghLegacyEndpoint = "https://models.inference.ai.azure.com/chat/completions"
	ghDefaultModel   = "openai/gpt-4.1-mini"
)

func githubChat(token, model string, msgs []ollamaMsg) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", errors.New("Add your GitHub token in Settings → AI helper → GitHub.")
	}
	if model == "" {
		model = ghDefaultModel
	}
	// formato estilo OpenAI (texto + imagens)
	var out []map[string]any
	for _, m := range msgs {
		if len(m.Images) == 0 {
			out = append(out, map[string]any{"role": m.Role, "content": m.Content})
			continue
		}
		parts := []map[string]any{{"type": "text", "text": m.Content}}
		for _, im := range m.Images {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64," + im}})
		}
		out = append(out, map[string]any{"role": m.Role, "content": parts})
	}
	try := func(url, mdl string) (string, int, error) {
		body, _ := json.Marshal(map[string]any{"model": mdl, "messages": out, "max_tokens": 2000})
		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		cl := &http.Client{Timeout: 3 * time.Minute}
		resp, err := cl.Do(req)
		if err != nil {
			return "", 0, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if resp.StatusCode != 200 {
			var e struct {
				Error struct{ Message string } `json:"error"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(b, &e)
			msg := e.Error.Message
			if msg == "" {
				msg = e.Message
			}
			return msg, resp.StatusCode, nil
		}
		var r struct {
			Choices []struct {
				Message struct{ Content string } `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(b, &r); err != nil || len(r.Choices) == 0 {
			return "", 500, nil
		}
		return strings.TrimSpace(r.Choices[0].Message.Content), 200, nil
	}
	ans, code, err := try(ghEndpoint, model)
	if err != nil || code == 404 {
		// endereço antigo do GitHub Models (nomes sem "openai/")
		short := model
		if i := strings.LastIndex(short, "/"); i >= 0 {
			short = short[i+1:]
		}
		if a2, c2, e2 := try(ghLegacyEndpoint, short); e2 == nil && c2 == 200 {
			return a2, nil
		} else if err != nil {
			return "", errors.New("Couldn't reach GitHub's AI. Check your internet.")
		} else {
			ans, code = a2, c2
		}
	}
	switch code {
	case 200:
		return ans, nil
	case 401, 403:
		return "", errors.New("GitHub didn't accept the token. Create a new one with the \"Models\" permission (steps in Settings).")
	case 429:
		return "", errors.New("You reached GitHub's free limit for now. Try again later (or tomorrow), or pick a smaller model.")
	case 400, 404, 422:
		return "", fmt.Errorf("GitHub doesn't know the model %q. Try %s. %s", model, ghDefaultModel, ans)
	}
	return "", fmt.Errorf("GitHub AI error (%d). %s", code, ans)
}
