package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Alert struct {
	Key   string `json:"-"`
	Icon  string `json:"icon"`
	Title string `json:"title"`
	Text  string `json:"text"`
	At    string `json:"at"`
	URL   string `json:"url,omitempty"`
}

var (
	alertsMu   sync.Mutex
	alertsSent = map[string]time.Time{}
	alertsLog  []Alert
	notify     = func(a Alert) {} // o Windows mostra a notificação
)

// computeAlerts decide quais avisos mostrar agora. Não tem efeitos colaterais.
func computeAlerts(d *Data, c Config, now time.Time) []Alert {
	var out []Alert
	day := now.Format("2006-01-02")
	d.mu.Lock()
	evs := append([]Event(nil), d.Events...)
	w := d.Weather
	d.mu.Unlock()

	// ---- Chuva ----
	if !c.NoAlertRain && w.OK {
		if isRainCode(w.Code) {
			what := "It's raining"
			if isStormCode(w.Code) {
				what = "There's a thunderstorm"
			}
			out = append(out, Alert{Key: "rainnow-" + now.Format("2006-01-02-15"), Icon: "🌧",
				Title: what + " in " + c.City, Text: "Take an umbrella if you go out ☂"})
		} else {
			for _, h := range w.Hourly {
				if h.Time.Before(now) || h.Time.After(now.Add(3*time.Hour)) {
					continue
				}
				if h.Prob >= 60 || isRainCode(h.Code) {
					out = append(out, Alert{Key: "rainsoon-" + day + "-" + h.Time.Format("15"), Icon: "☔",
						Title: fmt.Sprintf("Rain likely around %s", h.Time.Format("15:04")),
						Text:  fmt.Sprintf("%d%% chance in %s. Don't forget an umbrella!", h.Prob, c.City)})
					break
				}
			}
		}
	}

	s := analyse(evs, now)

	// ---- Lição de casa ----
	if !c.NoAlertHomework {
		for _, e := range s.HwToday {
			if e.Done {
				continue
			}
			left := e.Start.Sub(now)
			eod := e.Start.Hour() == 23 && e.Start.Minute() >= 59
			if !eod && left > 0 && left <= time.Hour {
				out = append(out, Alert{Key: "hwsoon-" + e.Title + e.Start.Format(time.RFC3339), Icon: "📚",
					Title: "Homework due at " + e.Start.Format("15:04"), Text: e.Title + courseSuffix(e), URL: e.URL})
			}
			if eod && now.Hour() >= 18 {
				out = append(out, Alert{Key: "hwtonight-" + e.Title + day, Icon: "📚",
					Title: "Due tonight", Text: e.Title + courseSuffix(e), URL: e.URL})
			}
		}
		if now.Hour() >= 17 && now.Hour() < 22 {
			var names []string
			for _, e := range s.HwTomorrow {
				if !e.Done {
					names = append(names, e.Title)
				}
			}
			if len(names) > 0 {
				t := strings.Join(names, ", ")
				if len(names) > 3 {
					t = strings.Join(names[:3], ", ") + fmt.Sprintf(" and %d more", len(names)-3)
				}
				out = append(out, Alert{Key: "hwtomorrow-" + day, Icon: "📝",
					Title: fmt.Sprintf("%d homework due tomorrow", len(names)), Text: t})
			}
		}
	}

	// ---- Reuniões (10 min antes) ----
	if !c.NoAlertMeetings {
		for _, m := range upcomingMeetings(evs, now) {
			left := m.Start.Sub(now)
			if left > 0 && left <= 10*time.Minute+30*time.Second {
				txt := "Starts in " + countdown(left)[3:]
				if m.With != "" {
					txt = "With " + m.With + " · " + txt
				}
				if app := meetingApp(m.Link); app != "" {
					txt += " · click to join on " + app
				}
				out = append(out, Alert{Key: "meet-" + m.Title + m.Start.Format(time.RFC3339), Icon: "📅",
					Title: m.Title + " at " + m.Start.Format("15:04"), Text: txt, URL: m.Link})
			}
		}
	}

	// ---- Próxima aula ----
	if !c.NoAlertClasses && s.Next != nil {
		left := s.Next.Start.Sub(now)
		if left > 0 && left <= 5*time.Minute+30*time.Second {
			where := ""
			if s.Next.Location != "" {
				where = " · " + s.Next.Location
			}
			out = append(out, Alert{Key: "class-" + s.Next.Title + s.Next.Start.Format(time.RFC3339), Icon: "🏫",
				Title: s.Next.Title + " starts at " + s.Next.Start.Format("15:04"), Text: "In " + countdown(left)[3:] + where})
		}
	}
	return out
}

func courseSuffix(e Event) string {
	if e.Course != "" {
		return " (" + e.Course + ")"
	}
	return ""
}

// runAlerts verifica a cada 30 s e manda cada aviso uma vez só.
func runAlerts(d *Data, cfg func() Config) {
	go func() {
		time.Sleep(20 * time.Second) // espera os dados carregarem
		for {
			now := time.Now()
			for _, a := range computeAlerts(d, cfg(), now) {
				alertsMu.Lock()
				_, seen := alertsSent[a.Key]
				if !seen {
					alertsSent[a.Key] = now
					a.At = now.Format(time.RFC3339)
					alertsLog = append(alertsLog, a)
					if len(alertsLog) > 30 {
						alertsLog = alertsLog[len(alertsLog)-30:]
					}
				}
				alertsMu.Unlock()
				if !seen {
					notify(a)
					time.Sleep(4 * time.Second) // não empilhar avisos
				}
			}
			// limpa a memória de avisos antigos
			alertsMu.Lock()
			for k, t := range alertsSent {
				if now.Sub(t) > 48*time.Hour {
					delete(alertsSent, k)
				}
			}
			alertsMu.Unlock()
			time.Sleep(30 * time.Second)
		}
	}()
}

// recentAlerts: avisos das últimas 6 horas, o mais novo primeiro.
func recentAlerts() []Alert {
	alertsMu.Lock()
	defer alertsMu.Unlock()
	out := []Alert{}
	for i := len(alertsLog) - 1; i >= 0; i-- {
		t, _ := time.Parse(time.RFC3339, alertsLog[i].At)
		if time.Since(t) < 6*time.Hour {
			out = append(out, alertsLog[i])
		}
	}
	return out
}
