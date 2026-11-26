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
Download the latest **Release**:
- Windows: `Jarvis.exe` → double-click (it installs/updates itself).
- Mac: `Jarvis-mac.zip` → drag `Jarvis.app` to Applications → right-click → Open.

## Build from source
Requires Go 1.25+.
```bash
# Windows
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o Jarvis.exe .
# Mac (Apple Silicon)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o Jarvis .
```

## Releasing an update
Create a Release with tag `vX.Y` (higher than the current version) and attach `Jarvis.exe` and `Jarvis-mac.zip`. Installed copies find it automatically.

## Credits
3D: [three.js](https://threejs.org) (MIT). Fonts: Orbitron & Rajdhani (SIL OFL).

License: MPL 2.0 (see LICENSE).
