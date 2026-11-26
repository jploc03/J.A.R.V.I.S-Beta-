package main

import (
	"fmt"
	"testing"
	"time"
)

func TestQuick(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.Local)
	d := &Data{Weather: Weather{OK: true, Temp: 17, Code: 2, TodayMin: 12, TodayMax: 19,
		Hourly: []HourW{{Time: now.Add(2 * time.Hour), Prob: 70, Code: 61}}}}
	cfg := defaultSettings()
	cfg.Meetings = []Meeting{{Title: "Call with Mom", With: "Mom", Repeat: "weekly", Day: now.Weekday().String()[:3], Start: "19:00", End: "19:30", Link: "https://teams.microsoft.com/l/meetup-join/abc"}}
	d.Events = append(timetableEvents([]ClassSlot{{Day: "Tue", Start: "11:00", End: "12:00", Subject: "Spanish", Room: "N304"}}),
		Event{Kind: "homework", Title: "Essay", Start: now.Add(5 * time.Hour)})
	d.Events = append(d.Events, manualMeetings(cfg.Meetings, now)...)
	for _, q := range []string{"what's my next class?", "qual a minha próxima aula?", "do I have homework", "vai chover hoje?", "any meetings?", "oi", "explain photosynthesis"} {
		a, ok := quickAnswer(q, d, cfg, now)
		fmt.Printf("Q: %-28s → %v %s\n", q, ok, a)
	}
	for _, q := range []string{"set a timer for 10 minutes", "põe um timer de 1 hora e 30 minutos", "stop the timer"} {
		a, _ := timerCommand(q)
		fmt.Printf("Q: %-36s → %s  [%s]\n", q, a, timer.View().State)
	}
	if a, _ := timerCommand("timer 2 min 30 s"); timer.View().TotalMs != 150000 {
		t.Fatal("duration", a, timer.View().TotalMs)
	}
	if l := findMeetLink(`Join: https://meet.google.com/abc-defg-hij\nThanks`); l != "https://meet.google.com/abc-defg-hij" {
		t.Fatal("meet link", l)
	}
	ms := upcomingMeetings(d.Events, now)
	if len(ms) == 0 || ms[0].Link == "" {
		t.Fatal("meetings", ms)
	}
}
