package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAll(t *testing.T) {
	now := time.Now()
	day := func(d, h, m int) string {
		x := now.AddDate(0, 0, d)
		return time.Date(x.Year(), x.Month(), x.Day(), h, m, 0, 0, time.Local).Format(time.RFC3339)
	}
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nDTSTART:" + now.Add(-7*24*time.Hour).Format("20060102") + "T080000\r\nDTEND:" +
		now.Add(-7*24*time.Hour).Format("20060102") + "T085000\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR\r\nSUMMARY:Portuguese\r\nLOCATION:Room 5\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:" + now.AddDate(0, 0, 1).Format("20060102") + "\r\nSUMMARY:Homework: read chapter 3\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cal.ics" {
			fmt.Fprint(w, ics)
			return
		}
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			return
		}
		fmt.Fprintf(w, `while(1);[
		 {"plannable_type":"assignment","context_name":"English 8B","html_url":"/courses/1/assignments/2","plannable":{"title":"Essay","due_at":%q},"submissions":{"submitted":false}},
		 {"plannable_type":"quiz","context_name":"Maths","plannable":{"title":"Quiz 4","due_at":%q},"submissions":{"submitted":true}},
		 {"plannable_type":"discussion_topic","plannable":{"title":"Discuss","due_at":%q},"submissions":false},
		 {"plannable_type":"calendar_event","context_name":"Science","plannable":{"title":"Science lab","start_at":%q,"end_at":%q}}]`,
			day(0, 23, 59), day(1, 15, 0), day(1, 9, 0), day(1, 10, 0), day(1, 11, 0))
	}))
	defer srv.Close()

	dir := t.TempDir()
	// importa o formato antigo
	os.WriteFile(filepath.Join(dir, "jarvis-settings.txt"), []byte("canvas_url = "+srv.URL+"/\ncanvas_token = good\nics = "+srv.URL+"/cal.ics\n[timetable]\nMon 10:00-10:50 History | B3\nseg 11:00-11:50 Arte\n"), 0o644)
	c, existed := loadSettings(filepath.Join(dir, "settings.json"))
	if existed || c.CanvasToken != "good" || len(c.ICS) != 1 || len(c.Timetable) != 2 || c.Timetable[1].Day != "seg" {
		t.Fatalf("import: %+v", c)
	}
	for _, d := range []string{"Mon", "Tue", "Wed", "Thu", "Fri"} {
		c.Timetable = append(c.Timetable, ClassSlot{Day: d, Start: "16:00", End: "16:50", Subject: "Club", Room: "Gym"})
	}
	if err := saveSettings(filepath.Join(dir, "settings.json"), c); err != nil {
		t.Fatal(err)
	}
	c, existed = loadSettings(filepath.Join(dir, "settings.json"))
	if !existed || len(c.Timetable) != 7 {
		t.Fatal("reload", existed, len(c.Timetable))
	}
	d := &Data{}
	refreshData(d, c)
	if d.CanvasState != "ok" {
		t.Fatal("canvas state", d.CanvasState, d.CanvasErr)
	}
	s := analyse(d.Events, now)
	if len(s.HwToday) != 1 || len(s.HwTomorrow) != 3 || s.Next == nil {
		t.Fatalf("summary wrong: %+v", s)
	}
	fmt.Print(contextSummary(d, now))
	for _, hk := range []string{"Ctrl+Shift+L", "ctrl+alt+j", "Win+Shift+F5"} {
		if _, _, ok := parseHotkey(hk); !ok {
			t.Fatal("hotkey", hk)
		}
	}
	for _, hk := range []string{"L", "Ctrl+Shift", "Ctrl+Banana+L"} {
		if _, _, ok := parseHotkey(hk); ok {
			t.Fatal("hotkey should fail", hk)
		}
	}
}
