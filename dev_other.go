//go:build !windows && !darwin

package main

// Modo de teste no Linux: só o servidor das janelas (sem a caixa nem a tela).

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

func main() {
	dir := os.Getenv("JARVIS_DIR")
	if dir == "" {
		dir, _ = os.MkdirTemp("", "jarvis")
	}
	path := filepath.Join(dir, "settings.json")
	var mu sync.Mutex
	cfg, _ := loadSettings(path)
	getSettings = func() Config { mu.Lock(); defer mu.Unlock(); return cfg }
	applySettings = func(c Config) error {
		if err := saveSettings(path, c); err != nil {
			return err
		}
		mu.Lock()
		cfg, _ = loadSettings(path)
		mu.Unlock()
		go refreshData(appData, getSettings())
		return nil
	}
	if fake := os.Getenv("JARVIS_FAKE_SCREEN"); fake != "" {
		screen = fakeScreen{path: fake}
	}
	go refreshData(appData, cfg)
	startTimerLoop()
	onTimerDone = func(l string) { fmt.Println("TIMER DONE", l) }
	notify = func(a Alert) { fmt.Println("ALERT", a.Title, "|", a.Text) }
	runAlerts(appData, getSettings)
	s, err := startServer()
	if err != nil {
		panic(err)
	}
	fmt.Println(s.URL("settings"))
	fmt.Println(s.URL("chat"))
	select {}
}

type fakeScreen struct{ path string }

func (f fakeScreen) Capture(int, int) ([]byte, int, int, error) {
	b, err := os.ReadFile(f.path)
	return b, 1280, 800, err
}
func (fakeScreen) Do(a string, in map[string]any) (string, error) { fmt.Println("ACTION", a, in); return "OK", nil }
func (fakeScreen) EscPressed() bool                                { return false }
