package main

// Atualização automática pelo GitHub (Releases do repositório jploc03/jarvis).

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const updateRepo = "jploc03/J.A.R.V.I.S-Beta-"

type UpdateInfo struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	Ready   bool   `json:"ready"` // já baixado
	Busy    bool   `json:"busy"`
	Error   string `json:"error,omitempty"`
	url     string
	file    string
}

var (
	updMu   sync.Mutex
	updInfo *UpdateInfo
	// por plataforma
	updateAssetName = "Jarvis.exe"
	applyUpdate     = func(file string) error { return errors.New("not supported") }
)

func currentUpdate() *UpdateInfo {
	updMu.Lock()
	defer updMu.Unlock()
	if updInfo == nil {
		return nil
	}
	c := *updInfo
	return &c
}

// checkUpdate pergunta ao GitHub se há versão nova (Releases ou a pasta release/ do repositório).
func checkUpdate() {
	if checkReleases() {
		return
	}
	checkRepoFolder()
}

// checkRepoFolder lê release/latest.json do repositório (funciona sem "Releases").
func checkRepoFolder() {
	base := "https://raw.githubusercontent.com/" + updateRepo + "/main/release/"
	req, _ := http.NewRequest("GET", base+"latest.json?t="+strconv.FormatInt(time.Now().Unix(), 10), nil)
	req.Header.Set("User-Agent", "Jarvis-School-Assistant")
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	var m struct {
		Version string `json:"version"`
		Notes   string `json:"notes"`
		Windows string `json:"windows"`
		Mac     string `json:"mac"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&m) != nil {
		return
	}
	v := strings.TrimPrefix(m.Version, "v")
	if !newerVersion(v, appVersion) {
		return
	}
	file := m.Mac
	if strings.HasSuffix(strings.ToLower(updateAssetName), ".exe") {
		file = m.Windows
	}
	if file == "" || strings.Contains(file, "..") || strings.Contains(file, "/") {
		return
	}
	updMu.Lock()
	if updInfo == nil || updInfo.Version != v {
		updInfo = &UpdateInfo{Version: v, Notes: m.Notes, url: base + file}
		updMu.Unlock()
		go downloadUpdate()
		return
	}
	updMu.Unlock()
}

// checkReleases usa a API de Releases do GitHub; devolve true se achou algo para tratar.
func checkReleases() bool {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+updateRepo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Jarvis-School-Assistant")
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	var r struct {
		Tag    string `json:"tag_name"`
		Body   string `json:"body"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if json.NewDecoder(resp.Body).Decode(&r) != nil {
		return false
	}
	v := strings.TrimPrefix(strings.TrimPrefix(r.Tag, "v"), "V")
	if !newerVersion(v, appVersion) {
		return false
	}
	for _, a := range r.Assets {
		if strings.EqualFold(a.Name, updateAssetName) {
			updMu.Lock()
			if updInfo == nil || updInfo.Version != v {
				notes := r.Body
				if len(notes) > 400 {
					notes = notes[:400] + "…"
				}
				updInfo = &UpdateInfo{Version: v, Notes: notes, url: a.URL}
				updMu.Unlock()
				go downloadUpdate()
				return true
			}
			updMu.Unlock()
			return true
		}
	}
	return false
}

func downloadUpdate() {
	updMu.Lock()
	if updInfo == nil || updInfo.Busy || updInfo.Ready {
		updMu.Unlock()
		return
	}
	updInfo.Busy = true
	u, v := updInfo.url, updInfo.Version
	updMu.Unlock()
	file := filepath.Join(os.TempDir(), "jarvis-update-"+v+"-"+updateAssetName)
	err := func() error {
		cl := &http.Client{Timeout: 10 * time.Minute}
		resp, err := cl.Get(u)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return errors.New(resp.Status)
		}
		f, err := os.Create(file)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, io.LimitReader(resp.Body, 200<<20))
		f.Close()
		return err
	}()
	updMu.Lock()
	defer updMu.Unlock()
	updInfo.Busy = false
	if err != nil {
		updInfo.Error = "Download failed: " + err.Error()
		return
	}
	updInfo.Ready, updInfo.file, updInfo.Error = true, file, ""
	go notify(Alert{Key: "update-" + v, Icon: "⬆", Title: "J.A.R.V.I.S. " + v + " is ready", Text: "Open Jarvis and click \"Update now\"."})
}

func installUpdate() error {
	updMu.Lock()
	info := updInfo
	updMu.Unlock()
	if info == nil || !info.Ready {
		return errors.New("no update ready")
	}
	return applyUpdate(info.file)
}

func startUpdateLoop() {
	go func() {
		time.Sleep(45 * time.Second)
		for {
			checkUpdate()
			time.Sleep(6 * time.Hour)
		}
	}()
}

// newerVersion diz se a é mais nova que b ("3.1" > "3.0").
func newerVersion(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}
