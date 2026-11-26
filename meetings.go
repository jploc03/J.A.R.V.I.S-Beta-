package main

import (
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Reunião adicionada à mão (ex.: "Mãe — chamada no Teams").
type Meeting struct {
	Title  string `json:"title"`
	With   string `json:"with"`
	Repeat string `json:"repeat"` // once | weekly
	Date   string `json:"date"`   // AAAA-MM-DD (once)
	Day    string `json:"day"`    // Mon..Sun (weekly)
	Start  string `json:"start"`
	End    string `json:"end"`
	Link   string `json:"link"`
}

// Calendário conectado (link ICS do Google/Gmail ou Outlook/Teams).
type CalLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

var meetLinkRe = regexp.MustCompile(`https://(?:teams\.microsoft\.com/l/meetup-join/[^\s"<>\\]+|teams\.live\.com/meet/[^\s"<>\\]+|meet\.google\.com/[a-z0-9-]+|[a-z0-9.]*zoom\.us/j/[^\s"<>\\]+)`)

func findMeetLink(fields ...string) string {
	for _, f := range fields {
		f = strings.ReplaceAll(f, `\n`, " ")
		if m := meetLinkRe.FindString(f); m != "" {
			return strings.TrimRight(m, ".,;)>")
		}
	}
	return ""
}

func meetingApp(link string) string {
	switch {
	case strings.Contains(link, "teams."):
		return "Teams"
	case strings.Contains(link, "meet.google"):
		return "Google Meet"
	case strings.Contains(link, "zoom.us"):
		return "Zoom"
	case link != "":
		return "link"
	}
	return ""
}

func fetchMeetingCalendars(c Config) []Event {
	var out []Event
	for _, cal := range c.Calendars {
		u := strings.TrimSpace(cal.URL)
		if u == "" {
			continue
		}
		if strings.HasPrefix(u, "webcal://") {
			u = "https://" + strings.TrimPrefix(u, "webcal://")
		}
		resp, err := httpClient.Get(u)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		resp.Body.Close()
		if resp.StatusCode != 200 {
			continue
		}
		for _, e := range parseICS(string(b)) {
			e.Kind = "meeting"
			e.Source = strings.TrimSpace(cal.Name)
			if e.Source == "" {
				e.Source = "Calendar"
			}
			out = append(out, e)
		}
	}
	return out
}

func manualMeetings(ms []Meeting, now time.Time) []Event {
	var out []Event
	for _, m := range ms {
		sh, sm, ok := parseHM(m.Start)
		if !ok || strings.TrimSpace(m.Title) == "" {
			continue
		}
		eh, em, okEnd := parseHM(m.End)
		mk := func(d time.Time) {
			ev := Event{Kind: "meeting", Source: "My meetings", Title: strings.TrimSpace(m.Title), With: strings.TrimSpace(m.With),
				Start: time.Date(d.Year(), d.Month(), d.Day(), sh, sm, 0, 0, time.Local), Link: strings.TrimSpace(m.Link)}
			if okEnd {
				ev.End = time.Date(d.Year(), d.Month(), d.Day(), eh, em, 0, 0, time.Local)
			}
			out = append(out, ev)
		}
		if m.Repeat == "weekly" {
			wd, ok := weekdayOf(m.Day)
			if !ok {
				continue
			}
			for k := -1; k < 8; k++ {
				d := now.AddDate(0, 0, k)
				if d.Weekday() == wd {
					mk(d)
				}
			}
		} else if d, err := time.ParseInLocation("2006-01-02", m.Date, time.Local); err == nil {
			mk(d)
		}
	}
	return out
}

// upcomingMeetings: reuniões de agora (em andamento) até daqui a 7 dias.
func upcomingMeetings(evs []Event, now time.Time) []Event {
	var out []Event
	seen := map[string]bool{}
	for _, e := range evs {
		if e.Kind != "meeting" {
			continue
		}
		end := e.End
		if end.IsZero() {
			end = e.Start.Add(time.Hour)
		}
		if end.Before(now) || e.Start.After(now.AddDate(0, 0, 7)) {
			continue
		}
		k := strings.ToLower(e.Title) + e.Start.Format(time.RFC3339)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}
