//go:build darwin

package main

// Jarvis para macOS: mesmo cérebro e mesmas janelas (Chrome/Edge em modo app, ou o navegador padrão).
// Sem o quadrado mini e sem atalho global (precisariam de código nativo do Mac).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const macAddr = "127.0.0.1:47391"

var (
	macDir   string
	macCfgMu sync.Mutex
	macCfg   Config
	macSrv   *Server
)

func main() {
	home, _ := os.UserHomeDir()
	macDir = filepath.Join(home, "Library", "Application Support", "Jarvis")
	_ = os.MkdirAll(macDir, 0o755)
	cfgPath := filepath.Join(macDir, "settings.json")
	urlFile := filepath.Join(macDir, "url.txt")

	// segredo fixo (para reabrir a mesma janela)
	secretFile := filepath.Join(macDir, "secret.txt")
	secret := ""
	if b, err := os.ReadFile(secretFile); err == nil {
		secret = strings.TrimSpace(string(b))
	}

	srv, err := startServerAt(macAddr, secret)
	if err != nil {
		// já está aberto: só mostra a janela
		if b, err := os.ReadFile(urlFile); err == nil {
			openApp(strings.TrimSpace(string(b)), 1220, 820)
		}
		return
	}
	macSrv = srv
	_ = os.WriteFile(secretFile, []byte(srv.Secret), 0o600)
	_ = os.WriteFile(urlFile, []byte(srv.URL("home")), 0o600)

	var existed bool
	macCfg, existed = loadSettings(cfgPath)
	if !existed {
		_ = saveSettings(cfgPath, macCfg)
		setLoginItem(true)
	}

	platform = "mac"
	appData = &Data{}
	getSettings = func() Config { macCfgMu.Lock(); defer macCfgMu.Unlock(); return macCfg }
	applySettings = func(c Config) error {
		if err := saveSettings(cfgPath, c); err != nil {
			return err
		}
		n, _ := loadSettings(cfgPath)
		macCfgMu.Lock()
		macCfg = n
		macCfgMu.Unlock()
		go refreshData(appData, getSettings())
		return nil
	}
	startupGet = loginItemEnabled
	startupSet = setLoginItem
	openURL = func(u string) { _ = exec.Command("open", u).Start() }
	openSettingsWindow = func(hash string) { openApp(srv.URL("settings")+hash, 900, 860) }
	goMini = func() {}
	openRobotsWindow = func() { openApp(srv.URL("robots"), 1320, 860) }
	quitApp = func() { os.Exit(0) }
	refreshNow = func() { go refreshData(appData, getSettings()) }

	onTimerDone = func(label string) {
		t := "Your timer finished."
		if label != "" {
			t = label + " — time's up!"
		}
		macNotify("⏱ Time's up!", t)
		go func() {
			for i := 0; i < 4 && timer.View().State == "done"; i++ {
				_ = exec.Command("afplay", "/System/Library/Sounds/Glass.aiff").Run()
				time.Sleep(700 * time.Millisecond)
			}
		}()
	}
	notify = func(a Alert) { macNotify(a.Icon+" "+a.Title, a.Text) }
	startTimerLoop()
	runAlerts(appData, getSettings)
	updateAssetName = "Jarvis-mac.zip"
	applyUpdate = func(file string) error {
		app := appBundlePath()
		if !strings.HasSuffix(app, ".app") {
			return fmt.Errorf("Jarvis isn't running from Jarvis.app")
		}
		tmp := filepath.Join(os.TempDir(), "jarvis-new")
		script := fmt.Sprintf(`rm -rf %[1]q && mkdir -p %[1]q && ditto -x -k %[2]q %[1]q && sleep 1 && rm -rf %[3]q && ditto %[1]q/Jarvis.app %[3]q && xattr -dr com.apple.quarantine %[3]q; open -a %[3]q`, tmp, file, app)
		if err := exec.Command("/bin/sh", "-c", "("+script+") >/dev/null 2>&1 &").Start(); err != nil {
			return err
		}
		go func() { time.Sleep(500 * time.Millisecond); os.Exit(0) }()
		return nil
	}
	startUpdateLoop()

	go func() {
		refreshData(appData, getSettings())
		for range time.Tick(15 * time.Minute) {
			refreshData(appData, getSettings())
		}
	}()

	openApp(srv.URL("home"), 1220, 820)
	select {}
}

// macNotify mostra uma notificação do macOS.
func macNotify(title, text string) {
	q := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) }
	script := fmt.Sprintf(`display notification "%s" with title "J.A.R.V.I.S." subtitle "%s" sound name "Glass"`, q(text), q(title))
	_ = exec.Command("osascript", "-e", script).Start()
}

// openApp abre a página numa janela própria (Chrome/Edge modo app) ou no navegador padrão.
func openApp(url string, w, h int) {
	for _, app := range []string{"Google Chrome", "Microsoft Edge", "Brave Browser"} {
		if _, err := os.Stat("/Applications/" + app + ".app"); err == nil {
			args := []string{"-na", app, "--args", "--app=" + url, "--user-data-dir=" + filepath.Join(macDir, "window"),
				"--no-first-run", "--no-default-browser-check", fmt.Sprintf("--window-size=%d,%d", w, h)}
			if exec.Command("open", args...).Start() == nil {
				return
			}
		}
	}
	_ = exec.Command("open", url).Start()
}

// ---- abrir ao iniciar a sessão (LaunchAgent) ----

func agentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.jarvis.school.plist")
}

func appBundlePath() string {
	exe, _ := os.Executable()
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		return exe[:i+4]
	}
	return exe
}

func loginItemEnabled() bool { _, err := os.Stat(agentPath()); return err == nil }

func setLoginItem(on bool) {
	if !on {
		_ = os.Remove(agentPath())
		return
	}
	app := appBundlePath()
	args := `<string>` + app + `</string>`
	if strings.HasSuffix(app, ".app") {
		args = `<string>/usr/bin/open</string><string>-a</string><string>` + app + `</string>`
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.jarvis.school</string>
<key>ProgramArguments</key><array>` + args + `</array>
<key>RunAtLoad</key><true/>
</dict></plist>
`
	_ = os.MkdirAll(filepath.Dir(agentPath()), 0o755)
	_ = os.WriteFile(agentPath(), []byte(plist), 0o644)
}
