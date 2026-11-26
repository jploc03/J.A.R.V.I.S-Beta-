package main

import (
	"sync"
	"time"
)

// Timer roda dentro do Jarvis (continua contando com a janela fechada).
type Timer struct {
	mu     sync.Mutex
	Total  time.Duration
	EndsAt time.Time
	Left   time.Duration // quando pausado
	State  string        // idle | running | paused | done
	Label  string
	DoneAt time.Time
}

var timer = &Timer{State: "idle"}

// ganchos: aviso e som quando acaba (o Windows preenche)
var (
	onTimerDone  = func(label string) {}
	onTimerClear = func() {}
)

type TimerView struct {
	State   string `json:"state"`
	TotalMs int64  `json:"totalMs"`
	LeftMs  int64  `json:"leftMs"`
	EndsAt  string `json:"endsAt,omitempty"`
	Label   string `json:"label"`
}

func (t *Timer) View() TimerView {
	t.mu.Lock()
	defer t.mu.Unlock()
	v := TimerView{State: t.State, TotalMs: t.Total.Milliseconds(), Label: t.Label}
	switch t.State {
	case "running":
		v.LeftMs = max(0, time.Until(t.EndsAt).Milliseconds())
		v.EndsAt = t.EndsAt.Format(time.RFC3339)
	case "paused":
		v.LeftMs = t.Left.Milliseconds()
	}
	return v
}

func (t *Timer) Start(d time.Duration, label string) {
	if d < time.Second {
		return
	}
	if d > 24*time.Hour {
		d = 24 * time.Hour
	}
	t.mu.Lock()
	t.Total, t.EndsAt, t.State, t.Label = d, time.Now().Add(d), "running", label
	t.mu.Unlock()
	onTimerClear()
}

func (t *Timer) Pause() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State == "running" {
		t.Left = time.Until(t.EndsAt)
		t.State = "paused"
	}
}

func (t *Timer) Resume() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State == "paused" {
		t.EndsAt = time.Now().Add(t.Left)
		t.State = "running"
	}
}

func (t *Timer) Cancel() {
	t.mu.Lock()
	t.State = "idle"
	t.mu.Unlock()
	onTimerClear()
}

// Tick é chamado a cada segundo.
func (t *Timer) Tick() {
	t.mu.Lock()
	fire := t.State == "running" && !time.Now().Before(t.EndsAt)
	label := t.Label
	if fire {
		t.State, t.DoneAt = "done", time.Now()
	}
	// some o aviso de "acabou" depois de 10 minutos
	if t.State == "done" && time.Since(t.DoneAt) > 10*time.Minute {
		t.State = "idle"
	}
	t.mu.Unlock()
	if fire {
		onTimerDone(label)
	}
}

func startTimerLoop() {
	go func() {
		for range time.Tick(time.Second) {
			timer.Tick()
		}
	}()
}
