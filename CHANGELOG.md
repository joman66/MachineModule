# Changelog

## Unreleased

- With MIDI sync, Live is the only transport: Play, Stop and Shift+Play do nothing in the module (the screen says "PLAY FOLLOWS LIVE"), on the Push and on the web page. Pressing Play in a module used to stop it alone and restart it at its own position, so two modules drifted apart. Start and stop Live instead; the module follows. With internal sync Play and Stop work as before.

## 0.2.0-alpha

- Release workflow (`.github/workflows/release.yml`): a `v*` tag tests, builds (Docker, cgo, linux/amd64), packages `machinemodule.tar.gz` and publishes a GitHub Release, then updates `release.json`. Every push and pull request is also tested and built, and the package is kept as a workflow artifact.
- Sync is MIDI by default (it was INT). The tempo follows Live: the MIDI clock while one arrives, otherwise the tempo from the PushHack Remote Script. Before, a tempo change in Live did nothing unless Live sent MIDI clock. The web page shows the tempo in use while Sync is MIDI. `Engine.LiveTempo`.
- Follows Live's transport when Sync is MIDI: if Live is already playing (MachineModule opened, or Sync switched to MIDI, mid-song) the sequencer joins at Live's position, and it stops when Live stops. Uses the PushHack Remote Script (`transport.get`), so it works even when Live sends no MIDI Start. New package `livesync`, `Engine.ExtJoin`.
- Finds Push 3 by name instead of assuming ALSA client 16. With the loopback card as ALSA card 0, Push 3 is client 20, and the pads, buttons and encoders did nothing (the log repeated `SUBSCRIBE_PORT ioctl: invalid argument`).
- Plays through the audio mixer of push-audio-loopback (audio bus) when it runs, so other audio hacks can play at the same time. Without the mixer it writes to the card directly, as before. Needs push-audio-loopback 0.2.0-alpha or later for this mode.

- Web page: dropdowns for the audio output device and channel pair.
- Create tracks in Live no longer makes a MIDI track.
- The audio session reopens when the device disappears (it spun at full CPU before).
- The web page stops building state when no browser is connected.

## 0.1.0-alpha

First version. 33 machines, 64 UserWave slots, 16-track sequencer with parameter locks, kits, songs, global effects, Push 3 front panel, web panel, MIDI input, project autosave.
