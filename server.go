package main

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed web/*
var webFS embed.FS

// Ganchos da plataforma (o main do Windows preenche).
var (
	getSettings     = func() Config { return defaultSettings() }
	applySettings   = func(c Config) error { return nil } // salva e aplica (atalho, refresh…)
	startupGet      = func() bool { return false }
	startupSet      = func(bool) {}
	appData         = &Data{}
	hotkeyAvailable = func(string) bool { return true }
	goMini          = func() {}
	refreshNow      = func() { go refreshData(appData, getSettings()) }
	isBusy          = func() bool { return false }
	platform        = "windows"
	quitApp         = func() {}
)

const appVersion = "4.4"

type Server struct {
	Port   int
	Secret string
	ln     net.Listener
}

func startServer() (*Server, error) { return startServerAt("127.0.0.1:0", "") }

// startServerAt: endereço fixo e segredo guardado (usado no Mac).
func startServerAt(addr, secret string) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if secret == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		secret = hex.EncodeToString(b)
	}
	s := &Server{Port: ln.Addr().(*net.TCPAddr).Port, Secret: secret, ln: ln}
	go http.Serve(ln, s.handler())
	return s, nil
}

func (s *Server) URL(page string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/%s?k=%s", s.Port, page, s.Secret)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(400)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type settingsView struct {
	Config
	Platform       string `json:"platform"`
	APIKeySet      bool `json:"apiKeySet"`
	GitHubTokenSet bool `json:"githubTokenSet"`
	CanvasTokenSet bool `json:"canvasTokenSet"`
	StartWithWin   bool `json:"startWithWindows"`
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()

	page := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("k") != s.Secret {
				http.Error(w, "forbidden", 403)
				return
			}
			b, _ := webFS.ReadFile("web/" + name)
			theme := getSettings().Theme
			html := strings.Replace(string(b), `<html lang="en"`, `<html lang="en" data-theme="`+theme+`"`, 1)
			if dotted, _ := assistantName(theme); dotted != "J.A.R.V.I.S." {
				html = strings.ReplaceAll(html, "J.A.R.V.I.S.", dotted)
			}
			// título da janela (aparece na barra de tarefas) + ícone do reator
			html = regexp.MustCompile(`<title>[^<]*</title>`).ReplaceAllString(html,
				"<title>"+pageTitle(strings.TrimSuffix(name, ".html"), theme)+`</title><link rel="icon" type="image/png" href="/icon.png?t=`+theme+`">`)
			b = []byte(html)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(b)
		}
	}
	mux.HandleFunc("GET /settings", page("settings.html"))
	mux.HandleFunc("GET /chat", page("chat.html"))
	mux.HandleFunc("GET /home", page("home.html"))
	mux.HandleFunc("GET /welcome", page("welcome.html"))
	mux.HandleFunc("GET /robots", page("robots.html"))
	mux.HandleFunc("GET /three/{path...}", func(w http.ResponseWriter, r *http.Request) {
		p := r.PathValue("path")
		if strings.Contains(p, "..") {
			http.NotFound(w, r)
			return
		}
		b, err := webFS.ReadFile("web/three/" + p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript")
		w.Header().Set("Cache-Control", "max-age=86400")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /fonts/{name}", func(w http.ResponseWriter, r *http.Request) {
		b, err := webFS.ReadFile("web/fonts/" + r.PathValue("name"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "font/woff2")
		w.Header().Set("Cache-Control", "max-age=86400")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /icon.png", func(w http.ResponseWriter, r *http.Request) {
		b, err := webFS.ReadFile("web/icon-" + getSettings().Theme + ".png")
		if err != nil {
			b, _ = webFS.ReadFile("web/icon-jarvis.png")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /robot.js", func(w http.ResponseWriter, r *http.Request) {
		b, _ := webFS.ReadFile("web/robot.js")
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /jarvis.css", func(w http.ResponseWriter, r *http.Request) {
		b, _ := webFS.ReadFile("web/jarvis.css")
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write(b)
	})

	api := http.NewServeMux()
	api.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		c := getSettings()
		v := settingsView{Config: c, APIKeySet: c.APIKey != "", CanvasTokenSet: c.CanvasToken != "", GitHubTokenSet: c.GitHubToken != "", StartWithWin: startupGet(), Platform: platform}
		v.APIKey, v.CanvasToken, v.GitHubToken = "", "", ""
		writeJSON(w, v)
	})
	api.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Config
			ClearAPIKey      bool `json:"clearApiKey"`
			ClearCanvasToken bool `json:"clearCanvasToken"`
			ClearGitHubToken bool `json:"clearGithubToken"`
			FinishOnboarding bool `json:"finishOnboarding"`
			StartWithWin     bool `json:"startWithWindows"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
			jsonErr(w, "bad data")
			return
		}
		old := getSettings()
		c := in.Config
		// campos que não vêm da tela
		c.HasPos, c.WinX, c.WinY, c.Zoom, c.Mode = old.HasPos, old.WinX, old.WinY, old.Zoom, old.Mode
		c.Onboarded = old.Onboarded || in.FinishOnboarding
		if strings.TrimSpace(c.APIKey) == "" && !in.ClearAPIKey {
			c.APIKey = old.APIKey
		}
		if strings.TrimSpace(c.GitHubToken) == "" && !in.ClearGitHubToken {
			c.GitHubToken = old.GitHubToken
		}
		if strings.TrimSpace(c.CanvasToken) == "" && !in.ClearCanvasToken {
			c.CanvasToken = old.CanvasToken
		}
		if _, _, ok := parseHotkey(c.Hotkey); !ok {
			jsonErr(w, "The shortcut isn't valid. Example: Ctrl+Shift+L")
			return
		}
		if platform != "windows" {
			c.Hotkey = old.Hotkey
		} else if c.Hotkey != old.Hotkey && !hotkeyAvailable(c.Hotkey) {
			jsonErr(w, "Another program already uses "+c.Hotkey+". Pick another shortcut.")
			return
		}
		var slots []ClassSlot
		for _, sl := range c.Timetable {
			if strings.TrimSpace(sl.Subject) == "" {
				continue
			}
			if _, ok := weekdayOf(sl.Day); !ok {
				jsonErr(w, "Pick a day for \""+sl.Subject+"\".")
				return
			}
			if _, _, ok := parseHM(sl.Start); !ok {
				jsonErr(w, "Start time for \""+sl.Subject+"\" should look like 08:30.")
				return
			}
			slots = append(slots, sl)
		}
		c.Timetable = slots
		if err := applySettings(c); err != nil {
			jsonErr(w, err.Error())
			return
		}
		startupSet(in.StartWithWin)
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("GET /api/geocode", func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeJSON(w, []any{})
			return
		}
		resp, err := httpClient.Get("https://geocoding-api.open-meteo.com/v1/search?count=6&language=en&name=" + url.QueryEscape(q))
		if err != nil {
			jsonErr(w, "No internet?")
			return
		}
		defer resp.Body.Close()
		var j struct {
			Results []struct {
				Name    string  `json:"name"`
				Country string  `json:"country"`
				Admin1  string  `json:"admin1"`
				Lat     float64 `json:"latitude"`
				Lon     float64 `json:"longitude"`
			} `json:"results"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&j)
		writeJSON(w, j.Results)
	})
	api.HandleFunc("POST /api/test-github", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Token, Model string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in)
		tok := strings.TrimSpace(in.Token)
		if tok == "" {
			tok = getSettings().GitHubToken
		}
		ans, err := githubChat(tok, strings.TrimSpace(in.Model), []ollamaMsg{{Role: "user", Content: "Reply with exactly: Jarvis online."}})
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": ans})
	})
	api.HandleFunc("GET /api/test-ollama", func(w http.ResponseWriter, r *http.Request) {
		ms, err := ollamaModels()
		if err != nil {
			writeJSON(w, map[string]any{"running": false})
			return
		}
		if ms == nil {
			ms = []string{}
		}
		writeJSON(w, map[string]any{"running": true, "models": ms})
	})
	api.HandleFunc("POST /api/test-canvas", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL, Token string
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in)
		c := getSettings()
		if strings.TrimSpace(in.URL) != "" {
			c.CanvasURL = strings.TrimRight(strings.TrimSpace(in.URL), "/")
		}
		if strings.TrimSpace(in.Token) != "" {
			c.CanvasToken = strings.TrimSpace(in.Token)
		}
		evs, st, msg := fetchCanvas(c)
		n := 0
		for _, e := range evs {
			if e.Kind == "homework" {
				n++
			}
		}
		writeJSON(w, map[string]any{"state": st, "message": msg, "homework": n})
	})
	api.HandleFunc("POST /api/timetable/from-screen", func(w http.ResponseWriter, r *http.Request) {
		delay, _ := time.ParseDuration(r.URL.Query().Get("delay") + "s")
		if delay < 0 || delay > 20*time.Second {
			delay = 5 * time.Second
		}
		time.Sleep(delay)
		shot, _, _, err := screen.Capture(2000, 1300)
		if err != nil {
			jsonErr(w, "Couldn't take a screenshot: "+err.Error())
			return
		}
		slots, note, err := extractTimetable(getSettings().APIKey, shot, "image/jpeg")
		if err != nil {
			jsonErr(w, err.Error())
			return
		}
		writeJSON(w, map[string]any{"classes": slots, "note": note})
	})
	api.HandleFunc("POST /api/timetable/from-image", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Data, Type string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 25<<20)).Decode(&in); err != nil {
			jsonErr(w, "bad image")
			return
		}
		img, err := base64.StdEncoding.DecodeString(in.Data)
		if err != nil || !validImageType(in.Type) {
			jsonErr(w, "Use a PNG, JPG, GIF or WEBP picture.")
			return
		}
		slots, note, err := extractTimetable(getSettings().APIKey, img, in.Type)
		if err != nil {
			jsonErr(w, err.Error())
			return
		}
		writeJSON(w, map[string]any{"classes": slots, "note": note})
	})

	// Chat
	api.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text      string `json:"text"`
			Look      bool   `json:"look"`
			Control   bool   `json:"control"`
			Voice     bool   `json:"voice"`
			Image     string `json:"image"`
			ImageType string `json:"imageType"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 25<<20)).Decode(&in); err != nil {
			jsonErr(w, "bad data")
			return
		}
		if strings.TrimSpace(in.Text) == "" && in.Image == "" {
			jsonErr(w, "Type a message first.")
			return
		}
		var img []byte
		if in.Image != "" {
			var err error
			img, err = base64.StdEncoding.DecodeString(in.Image)
			if err != nil || !validImageType(in.ImageType) {
				jsonErr(w, "Use a PNG, JPG, GIF or WEBP picture.")
				return
			}
		}
		dotted, plain := assistantName(getSettings().Theme)
		ctxText := "Your name: " + plain + " (" + dotted + ")\n"
		if n := getSettings().UserName; n != "" {
			ctxText += "The user's name: " + n + "\n"
		}
		if getSettings().Use == "work" {
			ctxText += "The user uses you for work (meetings, email, tasks), not school.\n"
		}
		ctxText += contextSummary(appData, time.Now())
		if err := assistant.Send(getSettings(), ctxText, in.Text, in.Look, in.Control, in.Voice, img, in.ImageType); err != nil {
			jsonErr(w, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("GET /api/chat/log", func(w http.ResponseWriter, r *http.Request) {
		var since int
		fmt.Sscanf(r.URL.Query().Get("since"), "%d", &since)
		items, running := assistant.Snapshot(since)
		if items == nil {
			items = []ChatItem{}
		}
		writeJSON(w, map[string]any{"items": items, "running": running, "hasKey": aiReady(getSettings()), "theme": getSettings().Theme, "use": getSettings().Use})
	})
	api.HandleFunc("POST /api/chat/stop", func(w http.ResponseWriter, r *http.Request) {
		assistant.Stop()
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/chat/new", func(w http.ResponseWriter, r *http.Request) {
		if !assistant.Reset() {
			jsonErr(w, "Stop Jarvis first.")
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/open-url", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ URL string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in)
		u, err := url.Parse(in.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			jsonErr(w, "bad link")
			return
		}
		openURL(u.String())
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("GET /api/summary", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, buildSummary(time.Now()))
	})
	api.HandleFunc("GET /api/timer", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, timer.View())
	})
	api.HandleFunc("POST /api/timer", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Action  string  `json:"action"`
			Seconds float64 `json:"seconds"`
			Label   string  `json:"label"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&in)
		if len([]rune(in.Label)) > 40 {
			in.Label = string([]rune(in.Label)[:40])
		}
		switch in.Action {
		case "start":
			timer.Start(time.Duration(in.Seconds*float64(time.Second)), strings.TrimSpace(in.Label))
		case "pause":
			timer.Pause()
		case "resume":
			timer.Resume()
		case "cancel", "dismiss":
			timer.Cancel()
		default:
			jsonErr(w, "unknown action")
			return
		}
		writeJSON(w, timer.View())
	})
	api.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		refreshNow()
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/mini", func(w http.ResponseWriter, r *http.Request) {
		goMini()
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("GET /api/robots", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"robots": robots, "aiReady": aiReady(getSettings()), "provider": getSettings().AIProvider})
	})
	api.HandleFunc("POST /api/robots/ask", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Robot, Text string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil || strings.TrimSpace(in.Text) == "" {
			jsonErr(w, "Say something first.")
			return
		}
		if in.Robot == "nexus" {
			writeJSON(w, ceoRun(strings.TrimSpace(in.Text)))
			return
		}
		writeJSON(w, robotAsk(in.Robot, strings.TrimSpace(in.Text)))
	})
	api.HandleFunc("POST /api/update", func(w http.ResponseWriter, r *http.Request) {
		if err := installUpdate(); err != nil {
			jsonErr(w, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); quitApp() }()
	})
	api.HandleFunc("POST /api/open-robots", func(w http.ResponseWriter, r *http.Request) {
		openRobotsWindow()
		writeJSON(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/open-settings", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Hash string }
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&in)
		if !strings.HasPrefix(in.Hash, "#") || len(in.Hash) > 20 {
			in.Hash = ""
		}
		openSettingsWindow(in.Hash)
		writeJSON(w, map[string]bool{"ok": true})
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// só aceita o próprio endereço (protege contra outros sites)
		if r.Host != fmt.Sprintf("127.0.0.1:%d", s.Port) {
			http.Error(w, "forbidden", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Header.Get("X-Jarvis-Key") != s.Secret {
				http.Error(w, "forbidden", 403)
				return
			}
			api.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func validImageType(t string) bool {
	switch t {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// abrir janelas (implementado por plataforma)
var openSettingsWindow = func(string) {}
var openURL = func(string) {}
var openRobotsWindow = func() {}

// ---------------- Resumo para a janela principal ----------------

type evJSON struct {
	Title    string `json:"title"`
	Course   string `json:"course,omitempty"`
	Location string `json:"location,omitempty"`
	Start    string `json:"start"`
	End      string `json:"end,omitempty"`
	Done     bool   `json:"done,omitempty"`
	Missing  bool   `json:"missing,omitempty"`
	URL      string `json:"url,omitempty"`
	Source   string `json:"source"`
}

func toJSON(e *Event) *evJSON {
	if e == nil {
		return nil
	}
	j := &evJSON{Title: e.Title, Course: e.Course, Location: e.Location, Start: e.Start.Format(time.RFC3339),
		Done: e.Done, Missing: e.Missing, URL: e.URL, Source: e.Source}
	if !e.End.IsZero() {
		j.End = e.End.Format(time.RFC3339)
	}
	return j
}

type meetJSON struct {
	evJSON
	With string `json:"with,omitempty"`
	Link string `json:"link,omitempty"`
	App  string `json:"app,omitempty"`
}

func meetingsJSON(l []Event) []meetJSON {
	out := []meetJSON{}
	for i := range l {
		out = append(out, meetJSON{evJSON: *toJSON(&l[i]), With: l[i].With, Link: l[i].Link, App: meetingApp(l[i].Link)})
	}
	return out
}

func listJSON(l []Event) []*evJSON {
	out := []*evJSON{}
	for i := range l {
		out = append(out, toJSON(&l[i]))
	}
	return out
}

func buildSummary(now time.Time) map[string]any {
	d := appData
	d.mu.Lock()
	evs := append([]Event(nil), d.Events...)
	w := d.Weather
	state, cerr, upd := d.CanvasState, d.CanvasErr, d.UpdatedAt
	d.mu.Unlock()
	s := analyse(evs, now)
	cfg := getSettings()

	// lição dos próximos dias (depois de amanhã)
	var later []Event
	dayAfter := time.Date(now.Year(), now.Month(), now.Day()+2, 0, 0, 0, 0, time.Local)
	for _, e := range evs {
		if e.Kind == "homework" && !e.Start.Before(dayAfter) && e.Start.Before(dayAfter.AddDate(0, 0, 6)) {
			later = append(later, e)
		}
	}
	sort.Slice(later, func(i, j int) bool { return later[i].Start.Before(later[j].Start) })
	// aulas de hoje
	var today []Event
	for _, e := range evs {
		if e.Kind == "class" && sameDay(e.Start, now) && !(e.Start.Hour() == 0 && e.Start.Minute() == 0) {
			today = append(today, e)
		}
	}
	sort.Slice(today, func(i, j int) bool { return today[i].Start.Before(today[j].Start) })

	sky, wcond := sceneFor(w, now)
	wj := map[string]any{"ok": w.OK, "city": cfg.City, "sky": sky, "cond": wcond}
	if !w.Sunrise.IsZero() {
		wj["sunrise"], wj["sunset"] = w.Sunrise.Format(time.RFC3339), w.Sunset.Format(time.RFC3339)
	}
	var hours []map[string]any
	for _, h := range w.Hourly {
		if h.Time.After(now.Add(-time.Hour)) && h.Time.Before(now.Add(12*time.Hour)) {
			hours = append(hours, map[string]any{"t": h.Time.Format(time.RFC3339), "p": h.Prob, "c": h.Code, "temp": h.Temp})
		}
	}
	wj["hourly"] = hours
	if w.OK {
		wj["temp"], wj["text"], wj["code"] = w.Temp, weatherText(w.Code), w.Code
		wj["min"], wj["max"], wj["rain"] = w.TodayMin, w.TodayMax, w.TodayRain
		wj["tomMax"], wj["tomMin"], wj["tomRain"], wj["tomText"], wj["tomCode"] = w.TomMax, w.TomMin, w.TomRain, weatherText(w.TomCode), w.TomCode
	}
	updated := ""
	if !upd.IsZero() {
		updated = upd.Format(time.RFC3339)
	}
	return map[string]any{
		"version": appVersion, "updatedAt": updated, "busy": isBusy(), "canvasState": state, "canvasError": cerr,
		"weather": wj, "hwToday": listJSON(s.HwToday), "hwTomorrow": listJSON(s.HwTomorrow), "hwLater": listJSON(later),
		"current": toJSON(s.Current), "next": toJSON(s.Next), "today": listJSON(today), "timetable": cfg.Timetable,
		"hasKey": aiReady(cfg), "hotkey": cfg.Hotkey,
		"alerts": recentAlerts(), "timer": timer.View(), "noAnimation": cfg.NoAnimation, "theme": cfg.Theme, "userName": cfg.UserName, "use": cfg.Use, "onboarded": cfg.Onboarded, "platform": platform, "update": currentUpdate(), "meetings": meetingsJSON(upcomingMeetings(evs, now)),
	}
}

// pageTitle: título de cada janela (o Windows usa para achar a janela já aberta).
func pageTitle(page, theme string) string {
	dotted, _ := assistantName(theme)
	switch page {
	case "settings":
		return dotted + " Settings"
	case "chat":
		return "Ask " + dotted
	case "welcome":
		return "Welcome to " + dotted
	case "robots":
		return "Robot Room · " + dotted
	}
	return dotted + " " + appVersion
}
