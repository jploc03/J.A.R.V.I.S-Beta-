package main

// Sala dos Robôs: 5 robôs especialistas + 1 CEO que distribui as tarefas.

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

type Robot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Area    string `json:"area"`
	Color   string `json:"color"`
	persona string
	route   *regexp.Regexp // palavras que o CEO usa para escolher sem IA
}

var robots = []Robot{
	{ID: "nexus", Name: "NEXUS", Role: "CEO", Area: "Command desk", Color: "#ffc94a",
		persona: "You are NEXUS, the CEO robot of the student's robot team. You are confident, calm and brief, like a great leader."},
	{ID: "atlas", Name: "ATLAS", Role: "Homework & study", Area: "Study station", Color: "#3fd4ff",
		persona: "You are ATLAS, the study robot. You help with homework, explain school topics step by step, make study plans and quiz the student. Act like a good tutor: help them understand instead of handing over answers to copy.",
		route:   regexp.MustCompile(`(?i)homework|study|explain|lição|licao|dever|tarefa|estud|explica|prova|test|exam|quiz|math|matem|science|ciênc|history|histór|english|inglês|spanish|espanhol|geography|geograf|chemistry|quím|physics|físic|biology|biolog`)},
	{ID: "chrono", Name: "CHRONO", Role: "Schedule & meetings", Area: "Calendar wall", Color: "#a98bff",
		persona: "You are CHRONO, the schedule robot. You know the student's classes, meetings and timetable, and you help plan the day and week and organize time.",
		route:   regexp.MustCompile(`(?i)class|aula|schedule|horário|horario|meeting|reuni|calendar|agenda|plan|organiz|week|semana|today|hoje|tomorrow|amanh|when|quando`)},
	{ID: "nimbus", Name: "NIMBUS", Role: "Weather & alerts", Area: "Weather station", Color: "#34f5a4",
		persona: "You are NIMBUS, the weather robot. You report the weather, rain and temperature, and give practical advice (umbrella, jacket, sports outside).",
		route:   regexp.MustCompile(`(?i)weather|rain|tempo|chuva|chover|temperat|frio|calor|cold|hot|umbrella|guarda|sol\b|sun|wind|vento|alert|aviso`)},
	{ID: "forge", Name: "FORGE", Role: "Tools & calculations", Area: "Workbench", Color: "#ff9f0a",
		persona: "You are FORGE, the tools robot. You do calculations, unit conversions, step-by-step math, and you set timers. Show the working briefly and give a clear final answer.",
		route:   regexp.MustCompile(`(?i)timer|alarm|alarme|temporizador|cron[oô]metro|calc|calcul|convert|convers|quanto é|how much is|\d+\s*[-+*/x×÷]\s*\d+|percent|porcent|%`)},
	{ID: "pixel", Name: "PIXEL", Role: "Ideas & writing", Area: "Creative lab", Color: "#ff5fb7",
		persona: "You are PIXEL, the creative robot. You brainstorm ideas, help write texts, emails and stories, improve writing, and research topics in a fun way.",
		route:   regexp.MustCompile(`(?i)idea|ideia|write|escrev|text|texto|email|e-mail|story|histór|poem|poema|creative|criativ|research|pesquis|project|projeto|present|apresenta|draw|desenh`)},
}

func robotByID(id string) *Robot {
	for i := range robots {
		if robots[i].ID == id {
			return &robots[i]
		}
	}
	return nil
}

type RobotReply struct {
	Robot string `json:"robot"`
	Task  string `json:"task,omitempty"`
	Reply string `json:"reply"`
	Error string `json:"error,omitempty"`
}

// memória de conversa de cada robô
var (
	robotMu     sync.Mutex
	robotClaude = map[string][]anthropic.BetaMessageParam{}
	robotPlain  = map[string][]ollamaMsg{}
	robotLocks  = map[string]*sync.Mutex{}
)

func robotLock(id string) *sync.Mutex {
	robotMu.Lock()
	defer robotMu.Unlock()
	if robotLocks[id] == nil {
		robotLocks[id] = &sync.Mutex{}
	}
	return robotLocks[id]
}

func robotSystem(r *Robot, cfg Config) string {
	team := ""
	for _, x := range robots {
		team += "- " + x.Name + ": " + x.Role + "\n"
	}
	who := "the student"
	if cfg.UserName != "" {
		who = cfg.UserName
	}
	return r.persona + "\n\nYou live in the Robot Room of the J.A.R.V.I.S. assistant on " + who + "'s computer. The team:\n" + team +
		"\nRules: answer in the language the user writes in (English or Portuguese). Keep it short (2-5 sentences) unless a longer explanation is really needed. " +
		"Your answer may be read aloud, so avoid tables and heavy formatting. Each message starts with a <context> block with the user's real data (time, classes, homework, meetings, weather) — use it."
}

// robotAsk: um robô responde (ações locais primeiro, depois a IA).
func robotAsk(id, text string) RobotReply {
	r := robotByID(id)
	if r == nil {
		return RobotReply{Robot: id, Error: "unknown robot"}
	}
	cfg := getSettings()
	out := RobotReply{Robot: id, Task: text}
	if a, ok := timerCommand(text); ok && (id == "forge" || id == "nexus") {
		out.Reply = a
		return out
	}
	if !aiReady(cfg) {
		if a, ok := quickAnswer(text, appData, cfg, time.Now()); ok {
			out.Reply = a
		} else if isPT(text) {
			out.Reply = r.Name + " aqui. Para eu fazer isso preciso de um cérebro: conecte uma IA em Configurações → AI helper (a do GitHub é grátis)."
		} else {
			out.Reply = r.Name + " here. To do that I need a brain: connect an AI in Settings → AI helper (the GitHub one is free)."
		}
		return out
	}
	l := robotLock(id)
	l.Lock()
	defer l.Unlock()
	ctxText := "<context>\n" + contextSummary(appData, time.Now()) + "</context>\n\n" + text
	ans, err := aiTurn(cfg, id, robotSystem(r, cfg), ctxText)
	if err != nil {
		out.Error = err.Error()
	} else {
		out.Reply = ans
	}
	return out
}

// aiTurn conversa com a IA escolhida, guardando o histórico do robô.
func aiTurn(cfg Config, id, system, user string) (string, error) {
	switch cfg.AIProvider {
	case "claude":
		robotMu.Lock()
		hist := append(append([]anthropic.BetaMessageParam(nil), robotClaude[id]...), anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user)))
		robotMu.Unlock()
		p := betaParams(hist)
		p.System = []anthropic.BetaTextBlockParam{{Text: system}}
		p.OutputConfig.Effort = "low"
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		client := newAI(cfg.APIKey)
		resp, err := client.Beta.Messages.New(ctx, p)
		if err != nil {
			return "", errors.New(apiErrText(err))
		}
		if resp.StopReason == anthropic.BetaStopReasonRefusal {
			return "", errors.New("I can't help with that one.")
		}
		var b strings.Builder
		for _, c := range resp.Content {
			if t, ok := c.AsAny().(anthropic.BetaTextBlock); ok {
				b.WriteString(t.Text)
			}
		}
		robotMu.Lock()
		h := append(hist, resp.ToParam())
		if len(h) > 40 { // conversa muito longa: começa uma nova (não editamos o histórico do Claude)
			h = nil
		}
		robotClaude[id] = h
		robotMu.Unlock()
		return strings.TrimSpace(b.String()), nil
	case "github", "ollama":
		robotMu.Lock()
		hist := append(append([]ollamaMsg(nil), robotPlain[id]...), ollamaMsg{Role: "user", Content: user})
		robotMu.Unlock()
		msgs := append([]ollamaMsg{{Role: "system", Content: system}}, hist...)
		var ans string
		var err error
		if cfg.AIProvider == "github" {
			ans, err = githubChat(cfg.GitHubToken, cfg.GitHubModel, msgs)
		} else {
			ans, err = ollamaChat(cfg.OllamaModel, msgs)
		}
		if err != nil {
			return "", err
		}
		robotMu.Lock()
		h := append(hist, ollamaMsg{Role: "assistant", Content: ans})
		if len(h) > 20 {
			h = h[len(h)-20:]
		}
		robotPlain[id] = h
		robotMu.Unlock()
		return ans, nil
	}
	return "", errors.New("no AI connected")
}

// ---------------- CEO: decide quem faz o quê ----------------

type ceoPlan struct {
	Reply  string `json:"reply"`
	Assign []struct {
		Robot string `json:"robot"`
		Task  string `json:"task"`
	} `json:"assign"`
}

var ceoSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "reply": {"type": "string", "description": "What NEXUS says out loud to the user (1-2 short sentences, same language as the user)"},
    "assign": {"type": "array", "items": {"type": "object", "properties": {
        "robot": {"type": "string", "enum": ["atlas","chrono","nimbus","forge","pixel"]},
        "task": {"type": "string", "description": "Clear instruction for that robot, in the user's language"}},
      "required": ["robot","task"], "additionalProperties": false}}
  },
  "required": ["reply","assign"],
  "additionalProperties": false
}`)

func ceoPlanFor(text string) ceoPlan {
	cfg := getSettings()
	var plan ceoPlan
	if aiReady(cfg) {
		team := ""
		for _, r := range robots[1:] {
			team += "- " + r.ID + " (" + r.Name + "): " + r.Role + "\n"
		}
		sys := robotSystem(&robots[0], cfg) + "\n\nYour job: read the user's request and delegate it to the right robot(s). Team:\n" + team +
			"Pick 1-3 robots (only the ones really needed). Give each a clear task. If the request is just a greeting or chat, assign nobody and reply yourself. " +
			`Respond ONLY with JSON: {"reply": "...", "assign": [{"robot": "atlas", "task": "..."}]}`
		user := "<context>\n" + contextSummary(appData, time.Now()) + "</context>\n\n" + text
		if raw, err := ceoJSON(cfg, sys, user); err == nil {
			if json.Unmarshal([]byte(raw), &plan) == nil && (len(plan.Assign) > 0 || strings.TrimSpace(plan.Reply) != "") {
				return cleanPlan(plan, text)
			}
			plan = ceoPlan{}
		}
	}
	// sem IA (ou se a IA falhar): escolhe pelas palavras
	for _, r := range robots[1:] {
		if r.route != nil && r.route.MatchString(text) {
			plan.Assign = append(plan.Assign, struct {
				Robot string `json:"robot"`
				Task  string `json:"task"`
			}{r.ID, text})
		}
	}
	if len(plan.Assign) > 3 {
		plan.Assign = plan.Assign[:3]
	}
	if len(plan.Assign) == 0 {
		plan.Assign = append(plan.Assign, struct {
			Robot string `json:"robot"`
			Task  string `json:"task"`
		}{"pixel", text})
	}
	return cleanPlan(plan, text)
}

func cleanPlan(p ceoPlan, text string) ceoPlan {
	var ok []struct {
		Robot string `json:"robot"`
		Task  string `json:"task"`
	}
	seen := map[string]bool{}
	for _, a := range p.Assign {
		if r := robotByID(a.Robot); r != nil && a.Robot != "nexus" && !seen[a.Robot] {
			seen[a.Robot] = true
			if strings.TrimSpace(a.Task) == "" {
				a.Task = text
			}
			ok = append(ok, a)
		}
	}
	p.Assign = ok
	if strings.TrimSpace(p.Reply) == "" {
		var names []string
		for _, a := range ok {
			names = append(names, robotByID(a.Robot).Name)
		}
		if isPT(text) {
			p.Reply = "Entendido. " + strings.Join(names, " e ") + ", com vocês."
		} else {
			p.Reply = "Understood. " + strings.Join(names, " and ") + ", you're up."
		}
	}
	return p
}

// ceoJSON pede o plano em JSON (no Claude usa saída estruturada).
func ceoJSON(cfg Config, system, user string) (string, error) {
	if cfg.AIProvider == "claude" {
		p := betaParams([]anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user))})
		p.System = []anthropic.BetaTextBlockParam{{Text: system}}
		p.OutputConfig.Effort = "low"
		p.OutputConfig.Format = anthropic.BetaJSONOutputFormatParam{Schema: ceoSchema}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		client := newAI(cfg.APIKey)
		resp, err := client.Beta.Messages.New(ctx, p)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for _, c := range resp.Content {
			if t, ok := c.AsAny().(anthropic.BetaTextBlock); ok {
				b.WriteString(t.Text)
			}
		}
		return b.String(), nil
	}
	msgs := []ollamaMsg{{Role: "system", Content: system}, {Role: "user", Content: user}}
	var ans string
	var err error
	if cfg.AIProvider == "github" {
		ans, err = githubChat(cfg.GitHubToken, cfg.GitHubModel, msgs)
	} else {
		ans, err = ollamaChat(cfg.OllamaModel, msgs)
	}
	if err != nil {
		return "", err
	}
	// pega só o objeto JSON da resposta
	if i, j := strings.Index(ans, "{"), strings.LastIndex(ans, "}"); i >= 0 && j > i {
		return ans[i : j+1], nil
	}
	return "", errors.New("no JSON")
}

// ceoRun: o CEO planeja e os robôs trabalham ao mesmo tempo.
func ceoRun(text string) map[string]any {
	plan := ceoPlanFor(text)
	results := make([]RobotReply, len(plan.Assign))
	var wg sync.WaitGroup
	for i, a := range plan.Assign {
		wg.Add(1)
		go func(i int, robot, task string) {
			defer wg.Done()
			results[i] = robotAsk(robot, task)
			results[i].Task = task
		}(i, a.Robot, a.Task)
	}
	wg.Wait()
	return map[string]any{"reply": plan.Reply, "assign": plan.Assign, "results": results}
}
