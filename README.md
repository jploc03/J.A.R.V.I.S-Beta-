# J.A.R.V.I.S. Beta 🤖

A personal school (and work) assistant for **Windows** and **macOS**, inspired by Tony Stark's J.A.R.V.I.S.

## Features
- **Main window** with your next class, today's classes, homework (Canvas), meetings, week timetable, weather and a live animated sky.
- **Mini mode**: a small floating HUD square (Ctrl+Shift+L).
- **Ask J.A.R.V.I.S.**: chat and **voice mode** (arc reactor orb). Works with Claude, the free GitHub AI, Ollama, or offline basics.
- **Look at / control the screen** (with Claude).
- **Tools**: iPhone-style timer, calculator, stopwatch, notes.
- **Alerts** (HUD pop-ups): rain, homework, classes, meetings.
- **Meetings**: your own + Gmail (Google Calendar) and Outlook/Teams calendars, with Join buttons.
- **Sala dos Robôs**: a 3D isometric robot office (realistic or cute style) with 5 robots + a CEO (NEXUS) who hands tasks to the team.
- **Styles**: J.A.R.V.I.S., F.R.I.D.A.Y., Mark III, Emerald, Galaxy.
- **Welcome tutorial**, auto-update from this repository's Releases.

## Install
Download from the [`release/`](release) folder:
- **Windows:** [`release/Jarvis.exe`](release/Jarvis.exe) → double-click (it installs/updates itself).
- **Mac:** [`release/Jarvis-mac.zip`](release/Jarvis-mac.zip) → drag `Jarvis.app` to Applications → right-click → Open.

Installed copies update themselves from this repository.

## Build from source
Requires Go 1.25+.
```bash
# Windows
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o Jarvis.exe .
# Mac (Apple Silicon)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o Jarvis .
```

## Releasing an update
1. Raise `appVersion` in `server.go` and build.
2. Put the new `Jarvis.exe` and `Jarvis-mac.zip` in `release/` and bump `version` in `release/latest.json`.
3. Push to `main`. Installed copies find it automatically (a GitHub Release with tag `vX.Y` and the same file names also works).

## Credits
3D: [three.js](https://threejs.org) (MIT). Fonts: Orbitron & Rajdhani (SIL OFL).

License: MPL 2.0 (see LICENSE).
