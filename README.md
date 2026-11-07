# MachineModule

A [Push Hack](https://github.com/federico-pepe/ableton-push-hack) module for Ableton Push 3: a drum machine in the style of the Elektron Machinedrum SPS-1UW MKII. It runs on the Push, draws on Push's screen, plays from its pads and buttons, and sends its audio into Live through the Push Hack virtual audio card.

Not affiliated with or endorsed by Elektron. The sound engine is original; only the model (names, track layout, parameter pages, sequencer features) follows the Machinedrum.

## What it does

- **16 tracks**, each with a "machine" chosen from 33 synth models: TRX (kick, snare, clap, rim, cowbell, hats, cymbal, maracas, claves), EFM (FM drums), PI (physical-model toms and percussion), GND (sine, noise, square) and UW (a UserWave sample player).
- **UserWave samples**: 64 slots, loaded from WAV files (Push SYSTEM page or the web page). Start, end, loop, reverse, pitch and decay per hit.
- **Sound shaping**: 8 synth parameters per machine, an effects page (amplitude modulation, EQ, filter, sample-rate reduction), a routing page (delay and reverb sends, pan, volume) and one LFO per track that can modulate any parameter.
- **Global effects** in every kit: delay, reverb, compressor, master EQ, master volume.
- **Sequencer**: 128 patterns (banks A-H, 16 each), 2-64 steps, pattern scale (1/8x to 2x), swing, accent, slide, **parameter locks** per step, real-time recording, copy/paste/clear, tap tempo, internal clock or MIDI clock (with MIDI sync Live is the only transport: Play and Stop in the module do nothing).
- **Kits** (64) and **songs** (16 songs of pattern rows with repeats and a loop point).
- **Project saving**: patterns, kits, songs, tempo and samples are saved automatically.

See [MANUAL.md](MANUAL.md) for the controls.

## Requirements

- Push 3 with Push Hack, including **push-manager** and **push-display** (for the Push screen).
- **push-audio-loopback** 0.3.0-alpha or later (it sends the audio to Live over Ableton Link Audio, or through a virtual audio card). The catalog installs it for you.
- The **PushHack Remote Script** in Live, only for the "Create tracks in Live" button.

## Install and first run

1. Install **MachineModule** from the Push Hack catalog (or `make`, then `scripts/deploy.sh`).
2. In Live: turn on **Link** and **Link Audio** (see "Link Audio" below). With the virtual card instead: Push audio preferences, Devices, enable an input pair of the *Push Hack Virtual Audio* card.
3. Open MachineModule's web page from the Push Hack menu, press **Create tracks in Live** (this makes an audio track that listens to the card; the sequencer is inside MachineModule, so no MIDI track is needed).
   Without the Remote Script: make an audio track with that input and Monitor *In*. In Link mode the button still makes the track on the virtual card: give the track the input type `PushHack` and pick `MachineModule` yourself.
   The web page's Setup section has a dropdown for the channel pair, and one for the audio output device when no audio mixer runs.
4. On Push, open **Push Hack Home** (Shift + Help) and choose MachineModule.

## Link Audio (default)

push-audio-loopback 0.3.0-alpha sends the audio of MachineModule to Live over Ableton Link Audio. This is its default.
MachineModule is then its own Link channel, named `MachineModule`, and Live keeps the sound card of the Push as its output.

1. In Live, turn on **Link** and **Link Audio**. Live switches them off after a restart.
2. Give an audio track the input type **PushHack**, pick the channel `MachineModule`, and set Monitor to In.
3. The Audio page of push-audio-loopback (Push Hack menu, *Audio*) switches between Link and the virtual card.

- The channel pair on the web page does not matter in Link mode.
- The *Create tracks in Live* button still makes the audio track on the virtual card. In Link mode, set the input of that track by hand as in step 2.
- After a restart of push-audio-loopback, MachineModule can drift against the clock of Live. Press play in Live and it is back in time. This is a known issue.

## Audio mixer

This section is about the virtual card mode.

MachineModule plays through the audio mixer of **push-audio-loopback** when it runs. The mixer adds the audio of all audio hacks, so MachineModule and Braids can play at the same time. The mixer owns the card, and the channel pair is chosen with the mixer (the web page's *Channel pair* list; the mixer remembers it). Without the mixer, MachineModule writes to the card itself, and only one audio hack can do that at a time. The Setup section of the web page shows which mode is active. In both modes Live needs an audio track that listens to the pair.

The design is in [docs/audio-bus.md](https://github.com/federico-pepe/ableton-push-hack/blob/main/docs/audio-bus.md) in the main Push Hack repository.

## Build

```
make          # builds ./machinemodule (needs Docker: cgo + libasound for linux/amd64)
make test     # go vet + go test -race inside the same container
```

The engine, sequencer, front panel, storage and MIDI mapping are plain Go and are covered by tests that run anywhere:

```
cd src && go test -race ./...
```

`src/cmd/mmrender` renders a pattern to a WAV file with no hardware, which is handy for sound design.

## Status

Beta. The audio engine, sequencer, and front panel has been tested on Push 3 Standalone and run with no apparent issues. Some small UI issues remain and will need sorting; please open issues if you encounter any. Playing through the audio mixer, the audio output and channel pair choice, and the Create tracks button have also been run on a Push 3.

## Not (yet) implemented

RAM-R / RAM-P live sampling, the CTR, INP and MID machine families, the ROM bank, per-track multi-output routing, per-track pattern length and scale ("multi" mode), sysex dump/load of the original hardware.
