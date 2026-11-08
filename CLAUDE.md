# CLAUDE.md

Guidance for Claude Code in this repository.

## What this is

MachineModule: a [push-hack](https://github.com/federico-pepe/ableton-push-hack) module for Ableton Push 3 that recreates a Machinedrum-style drum machine. A Go host renders audio (pure-Go engine) into push-audio-loopback's virtual card, draws a front panel on Push's screen, and serves a web page. Not affiliated with Elektron.

## Layout (`src/`)

| Package | Role | Needs hardware? |
|---|---|---|
| `engine/` | DSP and sequencer. Machines, per-track FX, global FX, patterns, kits, songs, player (96 PPQN), project JSON. One mutex guards `Engine`; `Render` holds it for one block. No allocations in `Render`. | no |
| `ui/` | Front panel: pads/buttons/encoders in, 960x160 frame and LED map out. Talks to the host through the `Host` interface. | no |
| `store/` | config, project.json, waves as WAV files, imports folder, autosave | no |
| `midimap/` | what notes/CCs mean | no |
| `livelink/` | PushHack Remote Script client (`track.ensure`, `transport.get`) | no (Caller is injectable) |
| `livesync/` | polls Live's transport (Remote Script) and joins/stops the sequencer on a change while Sync is MIDI | no |
| `cmd/mmrender/` | renders a pattern to WAV | no |
| main package | `main.go` (supervisor re-exec), `audio.go` (supervisor, direct card path), `bus.go` (audio bus client), `pcm.go`/`bridge.c` (cgo ALSA playback only, direct path), `midi.go` (port "MachineModule MIDI In"), `surface.go` (RunSurface, frames, LEDs), `web.go` + `web/index.html`, `app.go` | yes (cgo + libasound) |

## Rules (from the Push Hack framework)

- Never write the display shm directly: frames go through `pmclient.Push` while the app owns the surface; LEDs go through `POST /api/midi/led`.
- On-screen text is ASCII only. Never hardcode ports (`hack.json` "port" is only a default; the catalog allocates).
- Push controls count only while focused (`app.focus`), and only from Push 3's Live Port (port 0, channel 0 for CCs). Its client number is looked up by name with `alsaseq.Push3` before each subscribe: it is 16 only while Push 3 is ALSA card 0, and 20 when the loopback card is card 0. Never pin `Push3ClientDefault`.
- The render goroutine is pinned (`LockOSThread`, SCHED_FIFO); keep it allocation-free.
- Audio goes through the push-audio-loopback mixer when one answers on the bus socket (`bus.go`, `core/audiobus`): float blocks to the mixer, which owns the card and the channel pair (`bus_pair` in the config, -1 = the mixer chooses). Without a mixer: open `hw:PHVAudio,1,0` with the params Live negotiated (`hwparams`), S16_LE interleaved, channel pair at `channel_offset`. `superviseAudio` switches by itself. Design: `docs/audio-bus.md` in ableton-push-hack.
- With MIDI sync Live is the only transport. `engine.Play`, `Stop` and `TogglePlay` do nothing then (`LocalTransport()` is false; the Push Play button says "PLAY FOLLOWS LIVE"). Live sends no MIDI Start/Stop/Continue to this port in practice: the module follows through `livesync` (PushHack Remote Script poll) and the MIDI clock. A local Play/Stop restarted the module at its own position and made two modules drift (seen on a Push 3, 2026-11-04). Do not add a local transport under MIDI sync.
- `core/audiobus` and `alsaseq.Push3` come from `core/v0.5.6` (pinned in `src/go.mod`). It needs push-audio-loopback 0.2.0-alpha or later for bus mode.
- push-audio-loopback 0.3.0-alpha sends the bus audio to Live over Ableton Link Audio by default (a Link channel named `MachineModule`); the code here does not change. *Create tracks in Live* (`livelink`) makes the track on the virtual card, which is wrong in Link mode. So the Setup section hides the audio settings (class `aud`) when `audiobus.LinkProbe` (core v0.5.6) says Link (`app.linkMode`, state `config.link`), and the page tells the user to set the `PushHack` input by hand. Known issue: `renderBus` ends when the bus disconnects, so the engine stops while the MIDI clock goes on, and the sequencer drifts after a restart of push-audio-loopback. Fix, not done: keep calling `Render` while disconnected (`Write` already drops audio and sleeps one block).

## Build and test

- `cd src && go test -race ./...` runs everything except the main package's hardware paths. `go vet ./...` too.
- `make` builds the cgo binary in Docker (golang:1.25-bookworm + libasound2-dev). `go mod tidy` runs there and keeps go.sum current (committed).
- `make test` runs vet and tests in the same container.
- `scripts/deploy.sh` copies the binary to a Push for a manual test (service `push-hack-machinemodule`, log `/data/push-hack/logs/machinemodule.log`).

## Release

Bump `hack.json` "version", tag `v<version>`, push the tag. The workflow checks they match, tests, builds, packages `machinemodule.tar.gz` (top directory `machinemodule/`) and commits `release.json`. `.github/workflows/release.yml` also tests and builds every push and pull request (Docker, cgo) and keeps the package as a workflow artifact, so a build can be tried before tagging; only a `v*` tag publishes the Release. The catalog entry is in `catalog-entry.json` (add it to the framework's `catalog/catalog.json`).

## Hardware status

Checked on a Push 3: audio through the mixer and direct to the card, the channel pair dropdown, Create tracks in Live, pad, button and encoder input (Push 3 as ALSA client 20), following Live's transport and tempo (Sync MIDI). Not yet checked: Home focus/blur; LED updates (one HTTP request per change).
