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

var (
	ghEndpoint       = "https://models.github.ai/inference/chat/completions"
	ghLegacyEndpoint = "https://models.inference.ai.azure.com/chat/completions"
)

const ghDefaultModel = "openai/gpt-4.1-mini"

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
		body, _ := json.Marshal(map[string]any{"model": mdl, "messages": out})
		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		cl := &http.Client{Timeout: 3 * time.Minute}
		resp, err := cl.Do(req)
		if err != nil {
			return "", 0, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if resp.StatusCode != 200 {
			var e struct {
				Error struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"error"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(b, &e)
			msg := e.Error.Message
			if msg == "" {
				msg = e.Message
			}
			if msg == "" {
				msg = snippet(b)
			}
			if e.Error.Code != "" && !strings.Contains(msg, e.Error.Code) {
				msg = e.Error.Code + ": " + msg
			}
			return msg, resp.StatusCode, nil
		}
		var r struct {
			Choices []struct {
				Message struct {
					Content any `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(b, &r); err != nil || len(r.Choices) == 0 {
			return "Unexpected reply: " + snippet(b), -1, nil
		}
		txt := ""
		switch c := r.Choices[0].Message.Content.(type) {
		case string:
			txt = c
		case []any:
			for _, p := range c {
				if pm, ok := p.(map[string]any); ok {
					if s, ok := pm["text"].(string); ok {
						txt += s
					}
				}
			}
		}
		txt = strings.TrimSpace(txt)
		if txt == "" {
			return "Empty reply: " + snippet(b), -1, nil
		}
		return txt, 200, nil
	}
	short := func(m string) string {
		if i := strings.LastIndex(m, "/"); i >= 0 {
			return m[i+1:]
		}
		return m
	}
	// tentativas: modelo escolhido nos dois endereços, depois modelos de reserva
	type attempt struct{ url, model string }
	var plan []attempt
	seen := map[string]bool{}
	for _, m := range []string{model, ghDefaultModel, "openai/gpt-4o-mini", "openai/gpt-4.1"} {
		if seen[m] {
			continue
		}
		seen[m] = true
		plan = append(plan, attempt{ghEndpoint, m}, attempt{ghLegacyEndpoint, short(m)})
	}
	var firstMsg string
	firstCode := 0
	netFails := 0
	for _, a := range plan {
		ans, code, err := try(a.url, a.model)
		if err != nil {
			netFails++
			continue
		}
		if code == 200 {
			return ans, nil
		}
		if firstCode == 0 {
			firstCode, firstMsg = code, ans
		}
		switch code {
		case 401:
			return "", errors.New("GitHub didn't accept the token (401). Create a new one with the \"Models\" permission set to Read-only (steps in Settings). " + ans)
		case 429:
			return "", errors.New("You reached GitHub's free limit for now. Try again later (or tomorrow), or pick a smaller model. " + ans)
		}
	}
	if firstCode == 0 && netFails > 0 {
		return "", errors.New("Couldn't reach GitHub's AI. Check your internet.")
	}
	switch firstCode {
	case 403:
		return "", errors.New("GitHub refused (403): the token needs the \"Models\" permission (Read-only). " + firstMsg)
	case 400, 404, 422:
		return "", fmt.Errorf("GitHub doesn't accept the model %q (%d). Try %s. %s", model, firstCode, ghDefaultModel, firstMsg)
	case -1:
		return "", errors.New("GitHub AI: " + firstMsg)
	}
	return "", fmt.Errorf("GitHub AI error (%d). %s", firstCode, firstMsg)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "(no details)"
	}
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}
