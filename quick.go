package main

// Respostas rápidas sem IA (funcionam sem chave de API e sem internet para a IA).

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ptWords   = regexp.MustCompile(`(?i)[ãõçáéêíóôú]|\b(que|qual|quais|minha|meu|tenho|hoje|amanh|aula|lição|licao|tempo|horas|reuni|próxim|proxim|oi|olá|ola|quanto|vai|dever|tarefa)\b`)
	reTimer   = regexp.MustCompile(`(?i)\b(timer|temporizador|cron[oô]metro|alarm|alarme|contador)\b`)
	reDur     = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(h|hr|hrs|hour|hours|hora|horas|m|min|mins|minute|minutes|minuto|minutos|s|sec|secs|second|seconds|seg|segundo|segundos)\b`)
	reStop    = regexp.MustCompile(`(?i)\b(stop|cancel|para|pare|parar|cancela|cancelar|desliga)\b`)
	reNext    = regexp.MustCompile(`(?i)(next class|what class|which class|próxima aula|proxima aula|que aula|qual aula|next lesson)`)
	reToday   = regexp.MustCompile(`(?i)(classes today|today'?s classes|my timetable|aulas (de )?hoje|meu horário|meu horario|timetable)`)
	reHW      = regexp.MustCompile(`(?i)(homework|assignment|lição|licao|licoes|lições|dever|tarefa|tpc|due)`)
	reWeather = regexp.MustCompile(`(?i)(weather|rain|raining|temperature|umbrella|tempo|chuva|chover|chovendo|temperatura|frio|calor|guarda-chuva|sol\b)`)
	reTime    = regexp.MustCompile(`(?i)(what time|time is it|que horas|what day|what'?s the date|que dia|data de hoje)`)
	reMeet    = regexp.MustCompile(`(?i)(meeting|meetings|calendar|agenda|teams|reuni|compromisso|evento)`)
	reHello   = regexp.MustCompile(`(?i)^\s*(hi|hello|hey|oi|olá|ola|e aí|eai|bom dia|boa tarde|boa noite|good morning|good afternoon|good evening)\b`)
)

func isPT(s string) bool { return ptWords.MatchString(s) }

// timerCommand: "timer 10 min", "põe um timer de 5 minutos", "stop the timer"
func timerCommand(text string) (string, bool) {
	if !reTimer.MatchString(text) {
		return "", false
	}
	pt := isPT(text)
	if reStop.MatchString(text) {
		timer.Cancel()
		if pt {
			return "⏱ Timer cancelado.", true
		}
		return "⏱ Timer cancelled.", true
	}
	var total time.Duration
	for _, m := range reDur.FindAllStringSubmatch(text, -1) {
		v, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		u := strings.ToLower(m[2])
		switch {
		case strings.HasPrefix(u, "h"):
			total += time.Duration(v * float64(time.Hour))
		case u == "m" || strings.HasPrefix(u, "min"):
			total += time.Duration(v * float64(time.Minute))
		default:
			total += time.Duration(v * float64(time.Second))
		}
	}
	if total < time.Second {
		if pt {
			return "Por quanto tempo? Ex.: \"timer de 10 minutos\".", true
		}
		return "For how long? For example: \"timer 10 minutes\".", true
	}
	timer.Start(total, "")
	end := time.Now().Add(total).Format("15:04")
	if pt {
		return fmt.Sprintf("⏱ Timer de %s começou. Toca às %s.", humanDur(total, true), end), true
	}
	return fmt.Sprintf("⏱ Timer set for %s. It rings at %s.", humanDur(total, false), end), true
}

func humanDur(d time.Duration, pt bool) string {
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	var parts []string
	unit := func(n int, en, enp, p, pp string) {
		if n == 0 {
			return
		}
		w := en
		if pt {
			w = p
		}
		if n > 1 {
			w = enp
			if pt {
				w = pp
			}
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, w))
	}
	unit(h, "hour", "hours", "hora", "horas")
	unit(m, "minute", "minutes", "minuto", "minutos")
	unit(s, "second", "seconds", "segundo", "segundos")
	return strings.Join(parts, " ")
}

// quickAnswer responde perguntas simples com os dados do Jarvis.
func quickAnswer(text string, d *Data, cfg Config, now time.Time) (string, bool) {
	if a, ok := timerCommand(text); ok {
		return a, true
	}
	pt := isPT(text)
	d.mu.Lock()
	evs := append([]Event(nil), d.Events...)
	w := d.Weather
	d.mu.Unlock()
	s := analyse(evs, now)
	T := func(en, p string) string {
		if pt {
			return p
		}
		return en
	}
	var out []string
	matched := false

	if reHello.MatchString(text) {
		_, name := assistantName(cfg.Theme)
		who := ""
		if cfg.UserName != "" {
			who = ", " + cfg.UserName
		}
		out = append(out, T(greeting(now)+who+"! I'm "+name+".", map[bool]string{true: "Bom dia", false: map[bool]string{true: "Boa tarde", false: "Boa noite"}[now.Hour() < 19]}[now.Hour() < 12]+who+"! Eu sou "+name+"."))
		matched = true
	}
	if reTime.MatchString(text) {
		out = append(out, T("It's "+now.Format("15:04")+", "+now.Format("Monday 2 January")+".", "São "+now.Format("15:04")+"."))
		matched = true
	}
	if reNext.MatchString(text) {
		matched = true
		if s.Current != nil {
			out = append(out, T("Right now you have "+s.Current.Title+" until "+s.Current.End.Format("15:04")+".",
				"Agora você está em "+s.Current.Title+" até "+s.Current.End.Format("15:04")+"."))
		}
		if s.Next != nil {
			where := ""
			if s.Next.Location != "" {
				where = T(", in ", ", na sala ") + s.Next.Location
			}
			when := s.Next.Start.Format("15:04")
			if !sameDay(s.Next.Start, now) {
				when = T(s.Next.Start.Format("Monday")+" at "+when, "dia "+s.Next.Start.Format("02/01")+" às "+when)
			} else {
				when = T("at "+when, "às "+when)
			}
			out = append(out, T("Your next class is ", "Sua próxima aula é ")+s.Next.Title+" "+when+where+".")
		} else {
			out = append(out, T("I don't see any upcoming classes. Add your timetable in Settings.", "Não encontrei próximas aulas. Coloque seu horário nas Configurações."))
		}
	}
	if reToday.MatchString(text) && !reNext.MatchString(text) {
		matched = true
		var names []string
		for _, e := range evs {
			if e.Kind == "class" && sameDay(e.Start, now) {
				names = append(names, e.Start.Format("15:04")+" "+e.Title)
			}
		}
		if len(names) == 0 {
			out = append(out, T("No classes today! 🎉", "Sem aulas hoje! 🎉"))
		} else {
			out = append(out, T("Today: ", "Hoje: ")+strings.Join(names, ", ")+".")
		}
	}
	if reHW.MatchString(text) {
		matched = true
		list := func(l []Event) string {
			var n []string
			for _, e := range l {
				if !e.Done {
					n = append(n, e.Title)
				}
			}
			return strings.Join(n, ", ")
		}
		t, m := list(s.HwToday), list(s.HwTomorrow)
		switch {
		case t == "" && m == "":
			out = append(out, T("No homework due today or tomorrow. ✓", "Nenhuma lição para hoje ou amanhã. ✓"))
		default:
			if t != "" {
				out = append(out, T("Due today: ", "Para hoje: ")+t+".")
			}
			if m != "" {
				out = append(out, T("Due tomorrow: ", "Para amanhã: ")+m+".")
			}
		}
		if d.CanvasState == "notoken" {
			out = append(out, T("(Add your Canvas token in Settings so I can see all your homework.)", "(Coloque seu token do Canvas nas Configurações para eu ver todas as lições.)"))
		}
	}
	if reWeather.MatchString(text) {
		matched = true
		if !w.OK {
			out = append(out, T("I couldn't get the weather right now.", "Não consegui ver o tempo agora."))
		} else {
			rain := ""
			for _, h := range w.Hourly {
				if h.Time.After(now) && h.Time.Before(now.Add(6*time.Hour)) && (h.Prob >= 50 || isRainCode(h.Code)) {
					rain = T(fmt.Sprintf(" Rain is likely around %s (%d%%) — take an umbrella.", h.Time.Format("15:04"), h.Prob),
						fmt.Sprintf(" Deve chover por volta das %s (%d%%) — leve guarda-chuva.", h.Time.Format("15:04"), h.Prob))
					break
				}
			}
			out = append(out, T(fmt.Sprintf("In %s it's %.0f° and %s. Today %.0f° to %.0f°.", cfg.City, w.Temp, strings.ToLower(weatherText(w.Code)), w.TodayMin, w.TodayMax),
				fmt.Sprintf("Em %s está %.0f°, %s. Hoje entre %.0f° e %.0f°.", cfg.City, w.Temp, weatherPT(w.Code), w.TodayMin, w.TodayMax))+rain)
		}
	}
	if reMeet.MatchString(text) {
		matched = true
		ms := upcomingMeetings(evs, now)
		if len(ms) == 0 {
			out = append(out, T("No meetings coming up.", "Nenhuma reunião marcada."))
		} else {
			var n []string
			for i, m := range ms {
				if i >= 4 {
					break
				}
				when := m.Start.Format("15:04")
				if !sameDay(m.Start, now) {
					when = m.Start.Format("Mon 15:04")
				}
				n = append(n, when+" "+m.Title)
			}
			out = append(out, T("Coming up: ", "Próximas: ")+strings.Join(n, "; ")+".")
		}
	}
	if !matched {
		return "", false
	}
	return strings.Join(out, " "), true
}

func weatherPT(code int) string {
	switch {
	case code == 0:
		return "céu limpo"
	case code == 1:
		return "quase limpo"
	case code == 2:
		return "parcialmente nublado"
	case code == 3:
		return "nublado"
	case code == 45 || code == 48:
		return "com nevoeiro"
	case code >= 51 && code <= 57:
		return "com chuvisco"
	case code >= 61 && code <= 67, code >= 80 && code <= 82:
		return "chovendo"
	case code >= 71 && code <= 77:
		return "nevando"
	case code >= 95:
		return "com trovoada"
	}
	return ""
}

func noAIHelp(text string) string {
	if isPT(text) {
		return "Sem uma IA conectada eu respondo o básico: próxima aula, aulas de hoje, lição, tempo, reuniões, horas e timers (ex.: \"timer de 10 minutos\"). Para conversar sobre qualquer coisa, conecte uma IA em Configurações → AI helper (Claude, a IA grátis do GitHub, ou Ollama)."
	}
	return "Without an AI connected I can answer the basics: next class, today's classes, homework, weather, meetings, time and timers (e.g. \"timer 10 minutes\"). To talk about anything, connect an AI in Settings → AI helper (Claude, the free GitHub AI, or Ollama)."
}

// ---------------- IA grátis local (Ollama) ----------------

const ollamaURL = "http://127.0.0.1:11434"

type ollamaMsg struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

func ollamaChat(model string, msgs []ollamaMsg) (string, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "messages": msgs, "stream": false,
		"options": map[string]any{"num_ctx": 8192}})
	cl := &http.Client{Timeout: 4 * time.Minute}
	resp, err := cl.Post(ollamaURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", errors.New("The free AI (Ollama) isn't running. Install it from ollama.com and open it, then try again.")
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == 404 {
		return "", fmt.Errorf("The model %q isn't downloaded yet. Open a terminal and run:  ollama pull %s", model, model)
	}
	if resp.StatusCode != 200 {
		var e struct{ Error string }
		_ = json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return "", errors.New("Free AI error: " + e.Error)
	}
	var r struct {
		Message ollamaMsg `json:"message"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return "", errors.New("Free AI sent an answer I couldn't read.")
	}
	return strings.TrimSpace(r.Message.Content), nil
}

func ollamaModels() ([]string, error) {
	cl := &http.Client{Timeout: 4 * time.Second}
	resp, err := cl.Get(ollamaURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Models []struct{ Name string } `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	var out []string
	for _, m := range r.Models {
		out = append(out, m.Name)
	}
	return out, nil
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
