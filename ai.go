package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

const aiModel = "claude-opus-5-5"

// Screen é implementado em screen_windows.go (e um stub nos outros sistemas, para testes).
type Screen interface {
	// Capture tira um print da tela inteira, reduzido para caber em maxW x maxH. Devolve JPEG.
	Capture(maxW, maxH int) (jpeg []byte, w, h int, err error)
	// Do executa uma ação do computer toolset (coordenadas no espaço do último print).
	Do(action string, input map[string]any) (string, error)
	// EscPressed diz se o usuário apertou Esc desde a última checagem.
	EscPressed() bool
}

var screen Screen = noScreen{}

type noScreen struct{}

func (noScreen) Capture(int, int) ([]byte, int, int, error) { return nil, 0, 0, errors.New("screen not available") }
func (noScreen) Do(string, map[string]any) (string, error)   { return "", errors.New("screen not available") }
func (noScreen) EscPressed() bool                            { return false }

// Ganchos da interface (o main do Windows preenche).
var (
	hideOverlay = func() {}
	showOverlay = func() {}
)

func newAI(apiKey string) anthropic.Client {
	return anthropic.NewClient(option.WithAPIKey(apiKey), option.WithMaxRetries(2))
}

func betaParams(msgs []anthropic.BetaMessageParam) anthropic.BetaMessageNewParams {
	return anthropic.BetaMessageNewParams{
		Model:     aiModel,
		MaxTokens: 16000,
		Messages:  msgs,
		// Se o filtro de segurança recusar, o servidor tenta com outro modelo automaticamente.
		Betas:        []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks:    anthropic.BetaFallbacksParamOfDefault(),
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: "medium"},
	}
}

func apiErrText(err error) string {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case 401:
			return "The AI key is wrong. Open Settings and paste it again."
		case 402, 403:
			return "The AI account has no credit or no access. Check console.anthropic.com (Billing)."
		case 429:
			return "Too many requests right now. Wait a minute and try again."
		case 529, 500, 502, 503:
			return "The AI service is busy. Try again in a moment."
		}
		return fmt.Sprintf("AI error (%d).", apierr.StatusCode)
	}
	return "Couldn't reach the AI. Check your internet."
}

// ---------------- Timetable a partir de uma imagem ----------------

var timetableSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "classes": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "day": {"type": "string", "enum": ["Mon","Tue","Wed","Thu","Fri","Sat","Sun"]},
          "start": {"type": "string", "description": "24h HH:MM"},
          "end": {"type": "string", "description": "24h HH:MM"},
          "subject": {"type": "string"},
          "room": {"type": "string"}
        },
        "required": ["day","start","end","subject","room"],
        "additionalProperties": false
      }
    },
    "note": {"type": "string", "description": "Short note for the student if the image did not contain a timetable"}
  },
  "required": ["classes","note"],
  "additionalProperties": false
}`)

func extractTimetable(apiKey string, img []byte, mediaType string) ([]ClassSlot, string, error) {
	if apiKey == "" {
		return nil, "", errors.New("Add your AI key in Settings first.")
	}
	client := newAI(apiKey)
	today := time.Now().Format("Monday 2 January 2006")
	msg := anthropic.NewBetaUserMessage(
		anthropic.BetaContentBlockParamUnion{OfImage: &anthropic.BetaImageBlockParam{
			Source: anthropic.BetaImageBlockParamSourceUnion{OfBase64: &anthropic.BetaBase64ImageSourceParam{
				Data: base64.StdEncoding.EncodeToString(img), MediaType: anthropic.BetaBase64ImageSourceMediaType(mediaType)}}}},
		anthropic.NewBetaTextBlock("This is a screenshot or photo from a student's school portal or a picture of their class timetable. Today is "+today+
			". Extract the student's weekly class timetable: one entry per lesson, with weekday, start and end time (24h HH:MM), subject name as written, and room (empty string if not shown). "+
			"If the image shows a calendar of specific dates, map each lesson to its weekday. Ignore breaks, lunch, and homework or assignment items. "+
			"If there is no timetable in the image, return an empty list and explain briefly in note."),
	)
	p := betaParams([]anthropic.BetaMessageParam{msg})
	p.OutputConfig.Format = anthropic.BetaJSONOutputFormatParam{Schema: timetableSchema}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	resp, err := client.Beta.Messages.New(ctx, p)
	if err != nil {
		return nil, "", errors.New(apiErrText(err))
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return nil, "", errors.New("The AI couldn't read that image.")
	}
	var text string
	for _, b := range resp.Content {
		if t, ok := b.AsAny().(anthropic.BetaTextBlock); ok {
			text += t.Text
		}
	}
	var out struct {
		Classes []ClassSlot `json:"classes"`
		Note    string      `json:"note"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, "", errors.New("The AI answer was not understood. Try again.")
	}
	return out.Classes, out.Note, nil
}

// ---------------- Chat / ajudante ----------------

type ChatItem struct {
	N    int    `json:"n"`
	Kind string `json:"kind"` // user | assistant | action | error | info
	Text string `json:"text"`
}

type Assistant struct {
	mu      sync.Mutex
	history []anthropic.BetaMessageParam
	items   []ChatItem
	running bool
	stop    atomic.Bool
	look    atomic.Bool
	control atomic.Bool
	ollama  []ollamaMsg // histórico da IA grátis
}

var assistant = &Assistant{}

const systemPrompt = `You are a friendly study assistant (your name is given in the <context> block; usually Jarvis) running on a student's Windows computer. The student goes to an international school (Canvas is their learning platform). They may write in English or Portuguese: always answer in the language they used.

How to help with schoolwork:
- Act like a good tutor. Explain ideas clearly, give hints and worked examples, check the student's answers and point out mistakes, help plan and organize work.
- For graded homework, help the student understand and do it themselves rather than handing over finished answers to copy. If they just want the answer, give a short explanation with it so they learn.
- Keep answers short and clear, suitable for a teenager. Use simple formatting (short paragraphs, lists, **bold** for key points).

Each message from the student starts with a <context> block (date, time, homework due, next class) and a <permissions> block, and sometimes a <mode> block. Use the context when it helps, and follow <mode> when present.

Screen and computer control:
- You have a computer toolset that can take screenshots and use the mouse and keyboard on the student's real computer.
- Only take a screenshot if <permissions> says look=yes or control=yes. Only click, type, scroll or press keys if control=yes. If you need a permission you don't have, ask the student to tick the matching box ("Look at my screen" or "Let Jarvis control") and send the message again.
- When controlling the computer: take a screenshot first, work step by step, and verify with a new screenshot after important steps. Keep it to what the student asked.
- Never type passwords or enter personal or payment details, never buy anything, never send messages or emails, never submit or hand in an assignment, never delete files, and never change system or security settings. For anything like that, stop and tell the student to do that step themselves.
- The student can stop you at any time with Esc; if a tool result says you were stopped, stop and summarize what you did.`

func (a *Assistant) add(kind, text string) {
	a.items = append(a.items, ChatItem{N: len(a.items), Kind: kind, Text: text})
}

func (a *Assistant) Snapshot(since int) ([]ChatItem, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if since < 0 || since > len(a.items) {
		since = 0
	}
	return append([]ChatItem(nil), a.items[since:]...), a.running
}

func (a *Assistant) Reset() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return false
	}
	a.history = nil
	a.ollama = nil
	a.items = nil
	return true
}

func (a *Assistant) Stop() { a.stop.Store(true) }

// Send começa uma resposta em segundo plano.
// aiReady: tem alguma IA para conversar?
func aiReady(cfg Config) bool {
	return cfg.AIProvider == "ollama" || (cfg.AIProvider == "claude" && cfg.APIKey != "") || (cfg.AIProvider == "github" && cfg.GitHubToken != "")
}

// answerLocal responde na hora, sem IA (timer sempre; o resto só quando não há IA).
func (a *Assistant) answerLocal(cfg Config, text string, img []byte) bool {
	ans, ok := timerCommand(text)
	if !ok && !aiReady(cfg) {
		ans, ok = quickAnswer(text, appData, cfg, time.Now())
		if !ok {
			ans, ok = noAIHelp(text), true
		}
	}
	if !ok {
		return false
	}
	a.mu.Lock()
	shown := text
	if img != nil {
		shown += "  [picture attached]"
	}
	a.add("user", shown)
	a.add("assistant", ans)
	a.mu.Unlock()
	return true
}

func (a *Assistant) Send(cfg Config, contextText, text string, look, control, voice bool, img []byte, imgType string) error {
	a.mu.Lock()
	busy := a.running
	a.mu.Unlock()
	if !busy && a.answerLocal(cfg, text, img) {
		return nil
	}
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("Jarvis is still working. Wait or press Stop.")
	}
	a.running = true
	a.stop.Store(false)
	a.look.Store(look)
	a.control.Store(control)
	shown := text
	if img != nil {
		shown += "  [picture attached]"
	}
	a.add("user", shown)
	a.mu.Unlock()

	if cfg.AIProvider == "ollama" || cfg.AIProvider == "github" {
		go a.runOllama(cfg, contextText, text, look, voice, img)
	} else {
		go a.run(cfg, contextText, text, look, control, voice, img, imgType)
	}
	return nil
}

// runOllama conversa com a IA grátis local (sem controle da tela).
func (a *Assistant) runOllama(cfg Config, contextText, text string, look, voice bool, img []byte) {
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()
	msg := ollamaMsg{Role: "user", Content: "<context>\n" + contextText + "\n</context>\n" + voiceNote(voice) + "\n" + text}
	if img != nil {
		msg.Images = append(msg.Images, b64(img))
	}
	if look {
		if shot, _, _, err := screen.Capture(1280, 800); err == nil {
			msg.Images = append(msg.Images, b64(shot))
			msg.Content += "\n(A screenshot of the student's screen is attached.)"
		}
	}
	a.mu.Lock()
	a.ollama = append(a.ollama, msg)
	if len(a.ollama) > 30 { // mantém a conversa curta para a IA local
		a.ollama = a.ollama[len(a.ollama)-30:]
	}
	hist := append([]ollamaMsg{{Role: "system", Content: systemPrompt + "\n\nNote: you are running as a small local model without computer control. If asked to control the computer, explain that this needs the Claude AI in Settings."}}, a.ollama...)
	a.mu.Unlock()
	var ans string
	var err error
	if cfg.AIProvider == "github" {
		hist[0].Content = strings.Replace(hist[0].Content, "running as a small local model", "running on GitHub Models", 1)
		ans, err = githubChat(cfg.GitHubToken, cfg.GitHubModel, hist)
	} else {
		ans, err = ollamaChat(cfg.OllamaModel, hist)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.add("error", err.Error())
		a.ollama = a.ollama[:len(a.ollama)-1]
		return
	}
	a.ollama = append(a.ollama, ollamaMsg{Role: "assistant", Content: ans})
	a.add("assistant", ans)
}

func voiceNote(voice bool) string {
	if !voice {
		return ""
	}
	return "<mode>voice: the student is talking to you out loud and your answer will be read aloud. Answer like a friendly spoken conversation: 1-3 short sentences, no lists, no markdown, no emojis. If a longer explanation is needed, give the key idea and offer to continue.</mode>\n"
}

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func imageBlock(data []byte, mediaType string) anthropic.BetaContentBlockParamUnion {
	return anthropic.BetaContentBlockParamUnion{OfImage: &anthropic.BetaImageBlockParam{
		Source: anthropic.BetaImageBlockParamSourceUnion{OfBase64: &anthropic.BetaBase64ImageSourceParam{
			Data: base64.StdEncoding.EncodeToString(data), MediaType: anthropic.BetaBase64ImageSourceMediaType(mediaType)}}}}
}

func (a *Assistant) run(cfg Config, contextText, text string, look, control, voice bool, img []byte, imgType string) {
	defer func() {
		if r := recover(); r != nil {
			a.mu.Lock()
			a.add("error", fmt.Sprint("Something went wrong: ", r))
			a.mu.Unlock()
		}
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		if control {
			showOverlay()
		}
	}()
	if control {
		hideOverlay()
		screen.EscPressed() // limpa
	}

	var blocks []anthropic.BetaContentBlockParamUnion
	blocks = append(blocks, anthropic.NewBetaTextBlock(
		"<context>\n"+contextText+"\n</context>\n<permissions>look="+yesno(look||control)+" control="+yesno(control)+"</permissions>\n"+voiceNote(voice)+"\n"+text))
	if img != nil {
		blocks = append(blocks, imageBlock(img, imgType))
	}
	if look && !control {
		if shot, _, _, err := screen.Capture(1366, 900); err == nil {
			blocks = append(blocks, anthropic.NewBetaTextBlock("Screenshot of the student's screen right now:"), imageBlock(shot, "image/jpeg"))
		}
	}

	a.mu.Lock()
	a.history = append(a.history, anthropic.NewBetaUserMessage(blocks...))
	a.mu.Unlock()

	client := newAI(cfg.APIKey)
	tools := []anthropic.BetaToolUnionParam{{OfComputerToolset20260801: &anthropic.BetaComputerToolset20260801Param{}}}
	system := []anthropic.BetaTextBlockParam{{Text: systemPrompt, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()}}

	for step := 0; step < 60; step++ {
		a.mu.Lock()
		p := betaParams(append([]anthropic.BetaMessageParam(nil), a.history...))
		a.mu.Unlock()
		p.System = system
		p.Tools = tools
		p.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		resp, err := client.Beta.Messages.New(ctx, p)
		cancel()
		if err != nil {
			a.mu.Lock()
			a.add("error", apiErrText(err))
			// a mensagem do usuário fica no histórico; a próxima será juntada a ela
			a.mu.Unlock()
			return
		}

		a.mu.Lock()
		a.history = append(a.history, resp.ToParam())
		for _, b := range resp.Content {
			if t, ok := b.AsAny().(anthropic.BetaTextBlock); ok && strings.TrimSpace(t.Text) != "" {
				a.add("assistant", t.Text)
			}
		}
		if resp.StopReason == anthropic.BetaStopReasonRefusal {
			a.add("info", "Jarvis can't help with that one.")
		}
		if resp.StopReason == anthropic.BetaStopReasonMaxTokens {
			a.add("info", "(Answer cut short. Ask Jarvis to continue.)")
		}
		a.mu.Unlock()

		var uses []anthropic.BetaToolUseBlock
		for _, b := range resp.Content {
			if tu, ok := b.AsAny().(anthropic.BetaToolUseBlock); ok {
				uses = append(uses, tu)
			}
		}
		if len(uses) == 0 || resp.StopReason != anthropic.BetaStopReasonToolUse {
			return
		}

		results := a.runTools(uses)
		a.mu.Lock()
		a.history = append(a.history, anthropic.NewBetaUserMessage(results...))
		a.mu.Unlock()
		if a.stop.Load() {
			a.mu.Lock()
			a.add("info", "Stopped.")
			a.mu.Unlock()
			return
		}
	}
	a.mu.Lock()
	a.add("info", "Jarvis took many steps and paused. Send a message to let it continue.")
	a.mu.Unlock()
}

var actionLabels = map[string]string{
	"screenshot": "📸 Looking at the screen", "zoom": "🔍 Zooming in", "left_click": "🖱 Click", "right_click": "🖱 Right-click",
	"middle_click": "🖱 Middle-click", "double_click": "🖱 Double-click", "triple_click": "🖱 Triple-click",
	"left_click_drag": "🖱 Drag", "mouse_move": "🖱 Move mouse", "left_mouse_down": "🖱 Press", "left_mouse_up": "🖱 Release",
	"cursor_position": "🖱 Check mouse", "scroll": "🖱 Scroll", "type": "⌨ Type", "key": "⌨ Key", "hold_key": "⌨ Hold key", "wait": "⏳ Wait",
}

func (a *Assistant) runTools(uses []anthropic.BetaToolUseBlock) []anthropic.BetaContentBlockParamUnion {
	var out []anthropic.BetaContentBlockParamUnion
	failed := false
	for _, tu := range uses {
		res := anthropic.BetaToolResultBlockParam{ToolUseID: tu.ID}
		if tu.ToolsetName != "" {
			res.ToolsetName = anthropic.String(tu.ToolsetName)
		}
		setText := func(t string, isErr bool) {
			res.Content = []anthropic.BetaToolResultBlockParamContentUnion{{OfText: &anthropic.BetaTextBlockParam{Text: t}}}
			if isErr {
				res.IsError = anthropic.Bool(true)
			}
		}
		var input map[string]any
		_ = json.Unmarshal([]byte(tu.JSON.Input.Raw()), &input)

		switch {
		case failed:
			setText("Not executed: an earlier computer action in this turn failed.", true)
		case a.stop.Load() || screen.EscPressed():
			a.stop.Store(true)
			failed = true
			setText("Stopped by the student (Esc or Stop button). Do not continue; summarize what you did.", true)
		case tu.ToolsetName != "computer":
			failed = true
			setText("Unknown tool.", true)
		case tu.Name == "screenshot" || tu.Name == "zoom":
			if !a.look.Load() && !a.control.Load() {
				failed = true
				setText("Not allowed: the student has not ticked \"Look at my screen\". Ask them to.", true)
				break
			}
			a.logAction(tu.Name, input)
			var shot []byte
			var err error
			if tu.Name == "zoom" {
				shot, err = zoomShot(input)
			} else {
				shot, _, _, err = screen.Capture(1366, 900)
			}
			if err != nil {
				failed = true
				setText("Error: "+err.Error(), true)
				break
			}
			res.Content = []anthropic.BetaToolResultBlockParamContentUnion{{OfImage: &anthropic.BetaImageBlockParam{
				Source: anthropic.BetaImageBlockParamSourceUnion{OfBase64: &anthropic.BetaBase64ImageSourceParam{
					Data: base64.StdEncoding.EncodeToString(shot), MediaType: "image/jpeg"}}}}}
		default:
			if !a.control.Load() {
				failed = true
				setText("Not allowed: the student has not ticked \"Let Jarvis control\". Ask them to.", true)
				break
			}
			a.logAction(tu.Name, input)
			msg, err := screen.Do(tu.Name, input)
			if err != nil {
				if strings.Contains(err.Error(), "stopped by the student") {
					a.stop.Store(true)
				}
				failed = true
				setText("Error: "+err.Error(), true)
				break
			}
			if msg == "" {
				msg = "OK"
			}
			setText(msg, false)
			time.Sleep(250 * time.Millisecond)
		}
		out = append(out, anthropic.BetaContentBlockParamUnion{OfToolResult: &res})
	}
	return out
}

func zoomShot(input map[string]any) ([]byte, error) {
	if z, ok := screen.(interface {
		Zoom(region []float64) ([]byte, error)
	}); ok {
		var reg []float64
		if r, ok := input["region"].([]any); ok {
			for _, v := range r {
				f, _ := v.(float64)
				reg = append(reg, f)
			}
		}
		if len(reg) == 4 {
			return z.Zoom(reg)
		}
	}
	b, _, _, err := screen.Capture(1366, 900)
	return b, err
}

func (a *Assistant) logAction(name string, input map[string]any) {
	label := actionLabels[name]
	if label == "" {
		label = name
	}
	if name == "type" {
		if t, ok := input["text"].(string); ok {
			if len([]rune(t)) > 40 {
				t = string([]rune(t)[:40]) + "…"
			}
			label += ": \"" + t + "\""
		}
	} else if name == "key" {
		if t, ok := input["text"].(string); ok {
			label += ": " + t
		}
	}
	a.mu.Lock()
	// junta várias ações seguidas numa linha só
	if n := len(a.items); n > 0 && a.items[n-1].Kind == "action" && strings.Count(a.items[n-1].Text, "\n") < 30 {
		a.items[n-1].Text += "\n" + label
	} else {
		a.add("action", label)
	}
	a.mu.Unlock()
}

// Texto de contexto para cada mensagem: hora, lição, próxima aula.
func contextSummary(d *Data, now time.Time) string {
	d.mu.Lock()
	evs := append([]Event(nil), d.Events...)
	d.mu.Unlock()
	s := analyse(evs, now)
	var b strings.Builder
	fmt.Fprintf(&b, "Now: %s\n", now.Format("Monday 2 January 2006, 15:04"))
	list := func(label string, l []Event) {
		if len(l) == 0 {
			fmt.Fprintf(&b, "%s: nothing due\n", label)
			return
		}
		fmt.Fprintf(&b, "%s:\n", label)
		for _, e := range l {
			st := ""
			if e.Done {
				st = " (done)"
			}
			fmt.Fprintf(&b, "- %s %s%s%s\n", e.Start.Format("15:04"), e.Title, map[bool]string{true: " [" + e.Course + "]", false: ""}[e.Course != ""], st)
		}
	}
	list("Homework due today", s.HwToday)
	list("Homework due tomorrow", s.HwTomorrow)
	if s.Current != nil {
		fmt.Fprintf(&b, "Current class: %s until %s\n", s.Current.Title, s.Current.End.Format("15:04"))
	}
	if s.Next != nil {
		fmt.Fprintf(&b, "Next class: %s, %s at %s\n", s.Next.Title, s.Next.Start.Format("Monday"), s.Next.Start.Format("15:04"))
	}
	if ms := upcomingMeetings(evs, now); len(ms) > 0 {
		b.WriteString("Meetings / events this week:\n")
		for i, m := range ms {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&b, "- %s %s%s\n", m.Start.Format("Mon 15:04"), m.Title, map[bool]string{true: " (with " + m.With + ")", false: ""}[m.With != ""])
		}
	}
	return b.String()
}
