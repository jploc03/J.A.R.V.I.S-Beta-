package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------- Settings ----------------

type ClassSlot struct {
	Day     string `json:"day"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Subject string `json:"subject"`
	Room    string `json:"room"`
}

type Config struct {
	CanvasURL   string      `json:"canvasUrl"`
	CanvasToken string      `json:"canvasToken"`
	ICS         []string    `json:"ics"`
	City        string      `json:"city"`
	Lat         float64     `json:"lat"`
	Lon         float64     `json:"lon"`
	Hotkey      string      `json:"hotkey"`
	Timetable   []ClassSlot `json:"timetable"`
	APIKey      string      `json:"apiKey"`
	HasPos      bool        `json:"hasPos"`
	WinX        int32       `json:"winX"`
	WinY        int32       `json:"winY"`
	Zoom        float64     `json:"zoom"`
	Mode        string      `json:"mode"` // "window" (janela normal) ou "mini" (quadrado)
	// avisos e fundo (false = ligado)
	NoAlertRain     bool `json:"noAlertRain"`
	NoAlertHomework bool `json:"noAlertHomework"`
	NoAlertClasses  bool `json:"noAlertClasses"`
	NoAnimation     bool `json:"noAnimation"`
	Theme           string `json:"theme"` // jarvis | friday | ironman | emerald | galaxy
	NoAlertMeetings bool   `json:"noAlertMeetings"`
	Meetings        []Meeting `json:"meetings"`
	Calendars       []CalLink `json:"calendars"`
	AIProvider      string `json:"aiProvider"` // claude | github | ollama | none
	GitHubToken     string `json:"githubToken"`
	UserName        string `json:"userName"`  // como o Jarvis te chama
	Use             string `json:"use"`       // school | work
	Onboarded       bool   `json:"onboarded"` // já fez o tutorial
	GitHubModel     string `json:"githubModel"`
	OllamaModel     string `json:"ollamaModel"`
}

var themeNames = map[string]bool{"jarvis": true, "friday": true, "ironman": true, "emerald": true, "galaxy": true}

// nome do assistente conforme o tema
func assistantName(theme string) (dotted, plain string) {
	if theme == "friday" {
		return "F.R.I.D.A.Y.", "Friday"
	}
	return "J.A.R.V.I.S.", "Jarvis"
}

func defaultSettings() Config {
	return Config{CanvasURL: "https://canvas.inspirededucationschools.com", City: "Cascais", Lat: 38.6979, Lon: -9.4215,
		Hotkey: "Ctrl+Shift+L", Zoom: 1, ICS: []string{}, Timetable: []ClassSlot{}}
}

func (c *Config) normalize() {
	d := defaultSettings()
	c.CanvasURL = strings.TrimRight(strings.TrimSpace(c.CanvasURL), "/")
	if c.CanvasURL == "" {
		c.CanvasURL = d.CanvasURL
	}
	if strings.TrimSpace(c.Hotkey) == "" {
		c.Hotkey = d.Hotkey
	}
	if c.Meetings == nil {
		c.Meetings = []Meeting{}
	}
	if c.Calendars == nil {
		c.Calendars = []CalLink{}
	}
	if strings.TrimSpace(c.GitHubModel) == "" {
		c.GitHubModel = ghDefaultModel
	}
	c.GitHubToken = strings.TrimSpace(c.GitHubToken)
	c.UserName = strings.TrimSpace(c.UserName)
	if len([]rune(c.UserName)) > 30 {
		c.UserName = string([]rune(c.UserName)[:30])
	}
	if c.Use != "work" {
		c.Use = "school"
	}
	if c.AIProvider != "ollama" && c.AIProvider != "none" && c.AIProvider != "github" {
		c.AIProvider = "claude"
	}
	if strings.TrimSpace(c.OllamaModel) == "" {
		c.OllamaModel = "llama3.2"
	}
	if !themeNames[c.Theme] {
		c.Theme = "jarvis"
	}
	if c.Mode != "mini" {
		c.Mode = "window"
	}
	if c.Zoom < 0.6 || c.Zoom > 2 {
		c.Zoom = 1
	}
	if c.Lat == 0 && c.Lon == 0 {
		c.Lat, c.Lon, c.City = d.Lat, d.Lon, d.City
	}
	var ics []string
	for _, u := range c.ICS {
		if u = strings.TrimSpace(u); u != "" {
			ics = append(ics, u)
		}
	}
	c.ICS = ics
	if c.ICS == nil {
		c.ICS = []string{}
	}
	if c.Timetable == nil {
		c.Timetable = []ClassSlot{}
	}
	c.CanvasToken = strings.TrimSpace(c.CanvasToken)
	c.APIKey = strings.TrimSpace(c.APIKey)
}

var settingsMu sync.Mutex

// loadSettings lê settings.json; se não existir, importa o antigo jarvis-settings.txt.
func loadSettings(path string) (Config, bool) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	c := defaultSettings()
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &c)
		c.normalize()
		return c, true
	}
	old := strings.TrimSuffix(path, "settings.json") + "jarvis-settings.txt"
	if _, err := os.Stat(old); err == nil {
		c = loadOldConfig(old)
	}
	c.normalize()
	return c, false
}

func saveSettings(path string, c Config) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	c.normalize()
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Formato antigo (versão 1, arquivo de texto)
func loadOldConfig(path string) Config {
	c := defaultSettings()
	f, err := os.Open(path)
	if err != nil {
		return c
	}
	defer f.Close()
	inTT := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.EqualFold(line, "[timetable]") {
			inTT = true
			continue
		}
		if inTT {
			if s, ok := parseSlotLine(line); ok {
				c.Timetable = append(c.Timetable, s)
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch k {
		case "canvas_url":
			c.CanvasURL = v
		case "canvas_token":
			c.CanvasToken = v
		case "ics":
			if v != "" {
				c.ICS = append(c.ICS, v)
			}
		case "city":
			c.City = v
		case "lat":
			c.Lat, _ = strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
		case "lon":
			c.Lon, _ = strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
		case "hotkey":
			c.Hotkey = v
		}
	}
	return c
}

// ---------------- Model ----------------

type Event struct {
	Kind     string // "homework" | "class"
	Source   string
	Title    string
	Course   string
	Location string
	Start    time.Time
	End      time.Time // zero = none
	URL      string
	Done     bool
	Missing  bool
	Link     string // link da reunião (Teams, Meet, Zoom)
	With     string
}

type Weather struct {
	OK                  bool
	Temp, Feels         float64
	Code                int
	TodayMin, TodayMax  float64
	TodayRain           int
	TomMax, TomMin      float64
	TomRain, TomCode    int
	IsDay               bool
	Sunrise, Sunset     time.Time
	TomSunrise          time.Time
	Hourly              []HourW
}

type HourW struct {
	Time time.Time
	Prob int
	Code int
	Temp float64
}

type Data struct {
	mu          sync.Mutex
	Events      []Event
	Weather     Weather
	CanvasState string // "ok" | "notoken" | "badtoken" | "error"
	CanvasErr   string
	UpdatedAt   time.Time
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

var homeworkRe = regexp.MustCompile(`(?i)homework|home work|assignment|tpc|trabalho de casa|li[cç][aã]o|tarefa|deadline|\bdue\b|entrega|worksheet|quiz|\btest\b|teste|exam`)

// ---------------- Canvas ----------------

func fetchCanvas(c Config) ([]Event, string, string) {
	if strings.TrimSpace(c.CanvasToken) == "" {
		return nil, "notoken", ""
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 8)
	u := fmt.Sprintf("%s/api/v1/planner/items?start_date=%s&end_date=%s&per_page=100", c.CanvasURL,
		url.QueryEscape(start.UTC().Format(time.RFC3339)), url.QueryEscape(end.UTC().Format(time.RFC3339)))
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.CanvasToken))
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "error", "no internet?"
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return nil, "badtoken", ""
	}
	if resp.StatusCode != 200 {
		return nil, "error", fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	body = []byte(strings.TrimPrefix(string(body), "while(1);"))
	var items []struct {
		PlannableType string `json:"plannable_type"`
		PlannableDate string `json:"plannable_date"`
		ContextName   string `json:"context_name"`
		HTMLURL       string `json:"html_url"`
		Plannable     struct {
			Title        string `json:"title"`
			Name         string `json:"name"`
			DueAt        string `json:"due_at"`
			TodoDate     string `json:"todo_date"`
			StartAt      string `json:"start_at"`
			EndAt        string `json:"end_at"`
			LocationName string `json:"location_name"`
		} `json:"plannable"`
		Submissions json.RawMessage `json:"submissions"`
		Override    *struct {
			MarkedComplete bool `json:"marked_complete"`
		} `json:"planner_override"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, "error", "unexpected reply"
	}
	var out []Event
	for _, it := range items {
		p := it.Plannable
		link := it.HTMLURL
		if link != "" && !strings.HasPrefix(link, "http") {
			link = c.CanvasURL + link
		}
		title := p.Title
		if title == "" {
			title = p.Name
		}
		if it.PlannableType == "calendar_event" {
			s := parseTime(first(p.StartAt, it.PlannableDate))
			if s.IsZero() {
				continue
			}
			out = append(out, Event{Kind: "class", Source: "Canvas", Title: title, Course: it.ContextName,
				Location: p.LocationName, Start: s, End: parseTime(p.EndAt), URL: link})
			continue
		}
		s := parseTime(first(p.DueAt, p.TodoDate, it.PlannableDate))
		if s.IsZero() {
			continue
		}
		var sub struct{ Submitted, Graded, Excused, Missing bool }
		_ = json.Unmarshal(it.Submissions, &sub)
		done := sub.Submitted || sub.Graded || sub.Excused || (it.Override != nil && it.Override.MarkedComplete)
		if title == "" {
			title = "Task"
		}
		out = append(out, Event{Kind: "homework", Source: "Canvas", Title: title, Course: it.ContextName,
			Start: s, URL: link, Done: done, Missing: sub.Missing})
	}
	return out, "ok", ""
}

func first(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, l := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t.Local()
		}
	}
	return time.Time{}
}

// ---------------- ICS ----------------

var icsDateRe = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})(?:T(\d{2})(\d{2})(\d{2})?(Z)?)?$`)

func parseICSDate(v string) time.Time {
	m := icsDateRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return time.Time{}
	}
	a := func(s string) int { n, _ := strconv.Atoi(s); return n }
	loc := time.Local
	if m[7] == "Z" {
		loc = time.UTC
	}
	return time.Date(a(m[1]), time.Month(a(m[2])), a(m[3]), a(m[4]), a(m[5]), a(m[6]), 0, loc).Local()
}

func parseICS(text string) []Event {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n ", "")
	text = strings.ReplaceAll(text, "\n\t", "")
	var evs []map[string]string
	var cur map[string]string
	for _, line := range strings.Split(text, "\n") {
		switch line {
		case "BEGIN:VEVENT":
			cur = map[string]string{}
			continue
		case "END:VEVENT":
			if cur != nil {
				evs = append(evs, cur)
			}
			cur = nil
			continue
		}
		if cur == nil {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, _, _ = strings.Cut(k, ";")
		v = strings.NewReplacer(`\n`, " ", `\,`, ",", `\;`, ";").Replace(v)
		cur[strings.ToUpper(k)] = v
	}
	now := time.Now()
	from, to := now.AddDate(0, 0, -1), now.AddDate(0, 0, 8)
	var out []Event
	for _, e := range evs {
		s := parseICSDate(e["DTSTART"])
		if s.IsZero() {
			continue
		}
		en := parseICSDate(e["DTEND"])
		var dur time.Duration
		if !en.IsZero() {
			dur = en.Sub(s)
		}
		title := first(e["SUMMARY"], "Event")
		kind := "class"
		if homeworkRe.MatchString(title + " " + e["DESCRIPTION"]) {
			kind = "homework"
		}
		add := func(st time.Time) {
			if st.Before(from) || st.After(to) {
				return
			}
			ev := Event{Kind: kind, Source: "Calendar", Title: title, Location: e["LOCATION"], Start: st,
				Link: findMeetLink(e["X-MICROSOFT-SKYPETEAMSMEETINGURL"], e["X-GOOGLE-CONFERENCE"], e["LOCATION"], e["URL"], e["DESCRIPTION"])}
			if dur > 0 {
				ev.End = st.Add(dur)
			}
			out = append(out, ev)
		}
		rr := map[string]string{}
		if e["RRULE"] != "" {
			for _, p := range strings.Split(e["RRULE"], ";") {
				k, v, _ := strings.Cut(p, "=")
				rr[k] = v
			}
		}
		if rr["FREQ"] == "WEEKLY" || rr["FREQ"] == "DAILY" {
			until := parseICSDate(rr["UNTIL"])
			count := 1 << 30
			if rr["COUNT"] != "" {
				count, _ = strconv.Atoi(rr["COUNT"])
			}
			interval := 1
			if rr["INTERVAL"] != "" {
				interval, _ = strconv.Atoi(rr["INTERVAL"])
				if interval < 1 {
					interval = 1
				}
			}
			var days []time.Weekday
			for _, d := range strings.Split(rr["BYDAY"], ",") {
				if len(d) >= 2 {
					if i := strings.Index("SUMOTUWETHFRSA", d[len(d)-2:]); i >= 0 && i%2 == 0 {
						days = append(days, time.Weekday(i/2))
					}
				}
			}
			d := s
			for i := 0; i < 4000 && !d.After(to) && count > 0; i++ {
				if !until.IsZero() && d.After(until) {
					break
				}
				ok := false
				if rr["FREQ"] == "DAILY" {
					ok = i%interval == 0
				} else {
					week := int(d.Sub(s).Hours() / (24 * 7))
					match := d.Weekday() == s.Weekday()
					if len(days) > 0 {
						match = false
						for _, wd := range days {
							if wd == d.Weekday() {
								match = true
							}
						}
					}
					ok = match && week%interval == 0
				}
				if ok {
					add(d)
					count--
				}
				d = d.AddDate(0, 0, 1)
			}
		} else {
			add(s)
		}
	}
	return out
}

func fetchICS(c Config) []Event {
	var out []Event
	for _, u := range c.ICS {
		u = strings.TrimSpace(u)
		if strings.HasPrefix(u, "webcal://") {
			u = "https://" + strings.TrimPrefix(u, "webcal://")
		}
		resp, err := httpClient.Get(u)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			out = append(out, parseICS(string(b))...)
		}
	}
	return out
}

// ---------------- Timetable ----------------

var ttRe = regexp.MustCompile(`(?i)^(\S+)\s+(\d{1,2})[:h.](\d{2})\s*-\s*(\d{1,2})[:h.](\d{2})\s+(.+)$`)
var dayMap = map[string]time.Weekday{"sun": 0, "dom": 0, "mon": 1, "seg": 1, "tue": 2, "ter": 2, "wed": 3, "qua": 3,
	"thu": 4, "qui": 4, "fri": 5, "sex": 5, "sat": 6, "sab": 6, "sáb": 6}

func parseSlotLine(l string) (ClassSlot, bool) {
	m := ttRe.FindStringSubmatch(strings.TrimSpace(l))
	if m == nil {
		return ClassSlot{}, false
	}
	title, room, _ := strings.Cut(m[6], "|")
	return ClassSlot{Day: m[1], Start: m[2] + ":" + m[3], End: m[4] + ":" + m[5],
		Subject: strings.TrimSpace(title), Room: strings.TrimSpace(room)}, true
}

func parseHM(s string) (int, int, bool) {
	s = strings.NewReplacer("h", ":", "H", ":", ".", ":").Replace(strings.TrimSpace(s))
	h, m, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, false
	}
	hh, e1 := strconv.Atoi(h)
	mm, e2 := strconv.Atoi(m)
	if e1 != nil || e2 != nil || hh > 23 || mm > 59 || hh < 0 || mm < 0 {
		return 0, 0, false
	}
	return hh, mm, true
}

func weekdayOf(day string) (time.Weekday, bool) {
	key := strings.ToLower(strings.TrimSpace(day))
	if r := []rune(key); len(r) > 3 {
		key = string(r[:3])
	}
	wd, ok := dayMap[key]
	return wd, ok
}

func timetableEvents(slots []ClassSlot) []Event {
	var out []Event
	now := time.Now()
	for _, sl := range slots {
		wd, ok := weekdayOf(sl.Day)
		sh, sm, ok1 := parseHM(sl.Start)
		eh, em, ok2 := parseHM(sl.End)
		if !ok || !ok1 || strings.TrimSpace(sl.Subject) == "" {
			continue
		}
		for k := 0; k < 8; k++ {
			d := now.AddDate(0, 0, k)
			if d.Weekday() != wd {
				continue
			}
			ev := Event{Kind: "class", Source: "Timetable", Title: strings.TrimSpace(sl.Subject),
				Location: strings.TrimSpace(sl.Room), Start: time.Date(d.Year(), d.Month(), d.Day(), sh, sm, 0, 0, time.Local)}
			if ok2 {
				ev.End = time.Date(d.Year(), d.Month(), d.Day(), eh, em, 0, 0, time.Local)
			}
			out = append(out, ev)
		}
	}
	return out
}

// ---------------- Weather ----------------

func fetchWeather(c Config) Weather {
	u := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
		"&current=temperature_2m,apparent_temperature,weather_code,is_day"+
		"&hourly=precipitation_probability,weather_code,temperature_2m"+
		"&daily=temperature_2m_max,temperature_2m_min,precipitation_probability_max,weather_code,sunrise,sunset&timezone=auto&forecast_days=2", c.Lat, c.Lon)
	resp, err := httpClient.Get(u)
	if err != nil {
		return Weather{}
	}
	defer resp.Body.Close()
	var j struct {
		Current struct {
			T     float64 `json:"temperature_2m"`
			F     float64 `json:"apparent_temperature"`
			Code  int     `json:"weather_code"`
			IsDay int     `json:"is_day"`
		} `json:"current"`
		Hourly struct {
			Time []string  `json:"time"`
			Prob []*int    `json:"precipitation_probability"`
			Code []int     `json:"weather_code"`
			Temp []float64 `json:"temperature_2m"`
		} `json:"hourly"`
		Daily struct {
			Max     []float64 `json:"temperature_2m_max"`
			Min     []float64 `json:"temperature_2m_min"`
			Rain    []*int    `json:"precipitation_probability_max"`
			Code    []int     `json:"weather_code"`
			Sunrise []string  `json:"sunrise"`
			Sunset  []string  `json:"sunset"`
		} `json:"daily"`
	}
	if json.NewDecoder(resp.Body).Decode(&j) != nil || len(j.Daily.Max) < 2 || len(j.Daily.Rain) < 2 || len(j.Daily.Code) < 2 {
		return Weather{}
	}
	return parseWeather(j.Current.T, j.Current.F, j.Current.Code, j.Current.IsDay, j.Hourly.Time, j.Hourly.Prob, j.Hourly.Code, j.Hourly.Temp,
		j.Daily.Max, j.Daily.Min, j.Daily.Rain, j.Daily.Code, j.Daily.Sunrise, j.Daily.Sunset)
}

func parseWeather(t, f float64, code, isDay int, hTime []string, hProb []*int, hCode []int, hTemp []float64,
	dMax, dMin []float64, dRain []*int, dCode []int, sunrise, sunset []string) Weather {
	ri := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	lt := func(s string) time.Time {
		x, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local)
		if err != nil {
			return time.Time{}
		}
		return x
	}
	w := Weather{OK: true, Temp: t, Feels: f, Code: code, IsDay: isDay == 1,
		TodayMin: dMin[0], TodayMax: dMax[0], TodayRain: ri(dRain[0]),
		TomMin: dMin[1], TomMax: dMax[1], TomRain: ri(dRain[1]), TomCode: dCode[1]}
	if len(sunrise) > 0 && len(sunset) > 0 {
		w.Sunrise, w.Sunset = lt(sunrise[0]), lt(sunset[0])
	}
	if len(sunrise) > 1 {
		w.TomSunrise = lt(sunrise[1])
	}
	for i, ts := range hTime {
		h := HourW{Time: lt(ts)}
		if i < len(hProb) {
			h.Prob = ri(hProb[i])
		}
		if i < len(hCode) {
			h.Code = hCode[i]
		}
		if i < len(hTemp) {
			h.Temp = hTemp[i]
		}
		w.Hourly = append(w.Hourly, h)
	}
	return w
}

// ---------------- Cena do fundo (céu) ----------------

func isRainCode(c int) bool  { return (c >= 51 && c <= 67) || (c >= 80 && c <= 82) || c >= 95 }
func isSnowCode(c int) bool  { return (c >= 71 && c <= 77) || c == 85 || c == 86 }
func isStormCode(c int) bool { return c >= 95 }

// sceneFor devolve (céu, tempo): céu = day|dawn|sunset|night ; tempo = clear|partly|cloudy|fog|rain|storm|snow
func sceneFor(w Weather, now time.Time) (string, string) {
	sky := "day"
	if !w.Sunrise.IsZero() && !w.Sunset.IsZero() {
		near := func(t time.Time) bool { d := now.Sub(t); return d > -40*time.Minute && d < 40*time.Minute }
		switch {
		case near(w.Sunset):
			sky = "sunset"
		case near(w.Sunrise):
			sky = "dawn"
		case now.Before(w.Sunrise) || now.After(w.Sunset):
			sky = "night"
		}
	} else if h := now.Hour(); h < 7 || h >= 19 {
		sky = "night"
	}
	if !w.OK {
		return sky, "clear"
	}
	c := w.Code
	switch {
	case isStormCode(c):
		return sky, "storm"
	case isSnowCode(c):
		return sky, "snow"
	case isRainCode(c):
		return sky, "rain"
	case c == 45 || c == 48:
		return sky, "fog"
	case c == 3:
		return sky, "cloudy"
	case c == 2:
		return sky, "partly"
	}
	return sky, "clear"
}

func weatherText(code int) string {
	switch {
	case code == 0:
		return "Clear sky"
	case code == 1:
		return "Mostly clear"
	case code == 2:
		return "Partly cloudy"
	case code == 3:
		return "Cloudy"
	case code == 45 || code == 48:
		return "Fog"
	case code >= 51 && code <= 57:
		return "Drizzle"
	case code >= 61 && code <= 67:
		return "Rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Showers"
	case code >= 95:
		return "Thunderstorm"
	}
	return ""
}

// ---------------- Refresh ----------------

func refreshData(d *Data, c Config) {
	var wg sync.WaitGroup
	var canvas []Event
	var state, errMsg string
	var ics []Event
	var w Weather
	wg.Add(3)
	go func() { defer wg.Done(); canvas, state, errMsg = fetchCanvas(c) }()
	var meet []Event
	wg.Add(1)
	go func() { defer wg.Done(); meet = fetchMeetingCalendars(c) }()
	go func() { defer wg.Done(); ics = fetchICS(c) }()
	go func() { defer wg.Done(); w = fetchWeather(c) }()
	wg.Wait()

	d.mu.Lock()
	defer d.mu.Unlock()
	var evs []Event
	if state == "ok" {
		evs = append(evs, canvas...)
	} else if state == "error" {
		for _, e := range d.Events { // mantém dados antigos do Canvas se a internet falhar
			if e.Source == "Canvas" {
				evs = append(evs, e)
			}
		}
	}
	evs = append(evs, ics...)
	evs = append(evs, timetableEvents(c.Timetable)...)
	evs = append(evs, meet...)
	evs = append(evs, manualMeetings(c.Meetings, time.Now())...)
	d.Events = evs
	if w.OK || !d.Weather.OK {
		d.Weather = w
	}
	d.CanvasState, d.CanvasErr = state, errMsg
	d.UpdatedAt = time.Now()
}

// ---------------- Analysis ----------------

type Summary struct {
	HwToday, HwTomorrow []Event
	Current, Next       *Event
	RestToday           int
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func analyse(evs []Event, now time.Time) Summary {
	var s Summary
	tom := now.AddDate(0, 0, 1)
	seen := map[string]bool{}
	var classes []Event
	for _, e := range evs {
		key := strings.ToLower(e.Title) + e.Start.Format("2006-01-02") + e.Kind
		if e.Kind == "class" {
			key += e.Start.Format("15:04")
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if e.Kind == "homework" {
			if sameDay(e.Start, now) {
				s.HwToday = append(s.HwToday, e)
			} else if sameDay(e.Start, tom) {
				s.HwTomorrow = append(s.HwTomorrow, e)
			}
		} else if e.Kind == "class" {
			allDay := e.Start.Hour() == 0 && e.Start.Minute() == 0 && (e.End.IsZero() || e.End.Sub(e.Start) >= 23*time.Hour)
			if !allDay {
				classes = append(classes, e)
			}
		}
	}
	byTime := func(l []Event) { sort.Slice(l, func(i, j int) bool { return l[i].Start.Before(l[j].Start) }) }
	byTime(s.HwToday)
	byTime(s.HwTomorrow)
	byTime(classes)
	for i := range classes {
		c := &classes[i]
		if !c.Start.After(now) && !c.End.IsZero() && c.End.After(now) && s.Current == nil {
			s.Current = c
		}
		if c.Start.After(now) {
			if s.Next == nil {
				s.Next = c
			}
			if sameDay(c.Start, now) {
				s.RestToday++
			}
		}
	}
	return s
}

func countdown(d time.Duration) string {
	m := int(d.Round(time.Minute).Minutes())
	switch {
	case m < 1:
		return "now"
	case m < 60:
		return fmt.Sprintf("in %d min", m)
	case m < 24*60:
		if m%60 == 0 {
			return fmt.Sprintf("in %dh", m/60)
		}
		return fmt.Sprintf("in %dh%02d", m/60, m%60)
	}
	days := (m + 12*60) / (24 * 60)
	if days == 1 {
		return "in 1 day"
	}
	return fmt.Sprintf("in %d days", days)
}

func greeting(t time.Time) string {
	h := t.Hour()
	switch {
	case h < 5:
		return "Good night"
	case h < 12:
		return "Good morning"
	case h < 18:
		return "Good afternoon"
	}
	return "Good evening"
}
