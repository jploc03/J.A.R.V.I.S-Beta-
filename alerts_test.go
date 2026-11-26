package main

import (
	"fmt"
	"testing"
	"time"
)

func TestAlerts(t *testing.T) {
	now := time.Date(2026, 11, 24, 17, 30, 0, 0, time.Local)
	d := &Data{}
	d.Weather = Weather{OK: true, Code: 1, Hourly: []HourW{{Time: now.Add(90 * time.Minute).Truncate(time.Hour), Prob: 75, Code: 61}}}
	d.Events = []Event{
		{Kind: "homework", Title: "Essay", Course: "English", Start: now.Add(40 * time.Minute)},
		{Kind: "homework", Title: "Worksheet", Start: now.AddDate(0, 0, 1).Add(-3 * time.Hour)},
		{Kind: "homework", Title: "Quiz", Start: now.AddDate(0, 0, 1), Done: true},
		{Kind: "class", Title: "Maths", Location: "N310", Start: now.Add(4 * time.Minute), End: now.Add(64 * time.Minute)},
	}
	got := computeAlerts(d, defaultSettings(), now)
	for _, a := range got {
		fmt.Println(a.Icon, a.Title, "|", a.Text)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 alerts (rain soon, homework soon, tomorrow, class), got %d", len(got))
	}
	c := defaultSettings()
	c.NoAlertRain, c.NoAlertHomework, c.NoAlertClasses = true, true, true
	if n := len(computeAlerts(d, c, now)); n != 0 {
		t.Fatal("alerts should be off", n)
	}
	d.Weather.Code = 63
	if a := computeAlerts(d, defaultSettings(), now); a[0].Title != "It's raining in Cascais" {
		t.Fatal("raining now", a[0].Title)
	}
	sky, cond := sceneFor(Weather{OK: true, Code: 63, Sunrise: now.Add(-10 * time.Hour), Sunset: now.Add(-time.Hour)}, now)
	if sky != "night" || cond != "rain" {
		t.Fatal("scene", sky, cond)
	}
	if sky, _ := sceneFor(Weather{OK: true, Sunrise: now.Add(-9 * time.Hour), Sunset: now.Add(10 * time.Minute)}, now); sky != "sunset" {
		t.Fatal("sunset", sky)
	}
}
