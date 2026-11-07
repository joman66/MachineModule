# MachineModule manual

## The Push

Open it from Push Hack Home (Shift + Help). The screen shows the page tabs on top, the current track and pattern in a header, eight parameter cells in the middle and the soft-key labels at the bottom. The eight encoders edit the eight cells; the eight buttons above the screen choose the page, the eight below it are soft keys that change with the page.

### Pads

| Pad rows (bottom to top) | Use |
|---|---|
| 1-2 (bottom) | **Trig keys**: steps 1-8 on row 2, 9-16 on row 1 (page with Page Left/Right for steps 17-64). Press an empty step to place a trig, press a trig again to remove it. Hold a step and turn an encoder to **lock** that parameter on the step (locked steps turn blue on the web page). |
| 3-4 | **Track keys** 1-16 (BD SD HT MT LT CP RS CB CH OH RC CC M1-M4). Press to select and hit the track. |
| 5-6 | **Pattern keys**: 16 patterns of the current bank. Pressing one while playing queues it. |
| 7-8 (top) | **Pitch keys**: play the selected track chromatically (Octave Up/Down moves the range). |

The Scene buttons choose the bank A-H.

### Buttons

| Button | Action |
|---|---|
| Play | Start / stop. With Shift: restart from step 1. |
| Record | Real-time recording on/off: pad hits are written into the pattern while it plays. |
| Tap Tempo | Tap the tempo. |
| Mute | Track keys now mute / unmute. Solo (hold) + track key: solo. |
| Accent / Quantize (swing) / Repeat (slide) | Edit the accent, swing or slide layer: the step keys now set those flags. |
| Shift + step key | Set the pattern length to that step. |
| Delete + step / track key | Clear a trig / clear the whole track. |
| Duplicate + track key | Copy the selected track's pattern into that track. Duplicate alone remembers the pattern for **Convert** (paste). |
| Left / Right (D-pad) | Previous / next track. Up / Down: next / previous pattern (Shift: switch at once). |
| Undo | Cancel / No. Jog press or D-pad centre: Enter / Yes. |
| Save | Save the project now (it also saves by itself a moment after every change). |
| Settings | SYSTEM page. |
| Volume knob | Master volume. Tempo knob: tempo (Shift: fine). Jog wheel: browse lists, otherwise the selected track's volume. |

### Pages

1. **SYNTH** - the machine's eight parameters. Soft key 1 opens the machine list (jog or encoder 1 to browse, Enter to choose); soft keys 2 and 3 select the previous / next track.
2. **EFFECT** - AMD, AMF (amplitude modulation depth and frequency), EQF, EQG (EQ), FLTF, FLTW, FLTQ (high-pass, width, resonance), SRR (sample-rate reduction).
3. **ROUTE** - DEL, REV (sends), PAN, VOL, and the LFO: LFOP (which parameter), LFOS (speed), LFOD (depth), LFOM (shape).
4. **MASTER** - global effects. Soft keys 1 and 2 switch between delay and reverb (DTIM DSYN DFB DLP DLEV RDEC RDMP RLEV) and compressor, EQ and volume (CTHR CRAT CREL CMKP CMIX ELG EHG MVOL).
5. **PATTERN** - length, scale, swing amount, accent amount, the kit the pattern loads, tempo, sync (INT / MIDI) and song mode. Soft keys: clear (with Shift), copy, paste, record, shift the track left or right.
6. **SONG** - rows of pattern + repeats, a loop-to row, song select. Soft keys: add row, delete row, song mode on/off.
7. **KIT** - 64 kits. Soft keys: load, save, init. A kit holds all 16 machines, their parameters and the global effects.
8. **SYSTEM** - UserWave: encoder 1 selects the slot, encoder 2 / jog the file from the imports folder; soft keys **LOAD UW**, **CLEAR UW**, **SAVE**. Encoder 7 selects the clock source.

Parameter edits made while no step is held change the kit. Edits made while holding a trig change only that step (a *parameter lock*).

### Sync

*Sync* is MIDI by default: MachineModule follows Live's clock, start and stop. Set it to INT on the PATTERN or SYSTEM page (or the web page) for the sequencer's own tempo. The tempo is not saved: it starts as MIDI each time.

MIDI Start only reaches MachineModule at the moment Live starts. So that it also follows when Live is already playing (you opened MachineModule, or switched Sync to MIDI, in the middle of a song), MachineModule asks the PushHack Remote Script in Live whether it plays and where. When Live changes from stopped to playing, the sequencer joins at Live's position (the pattern lines up with Live's beats as if it had started with Live at beat 0). When Live stops, it stops. The tempo follows Live too: from the MIDI clock while it arrives, and otherwise from the Remote Script (Live sends no clock unless you turn on Sync for MachineModule's MIDI port, and none while it is stopped), so a tempo change in Live reaches the sequencer and the tempo-synced delay either way. It only reacts to a change of Live's transport, so a sequencer you started by hand stays on until Live stops after playing. Without the Remote Script, only MIDI Start, Stop and the clock work.

## UserWave samples

Put `.wav` files (8, 16, 24, 32-bit PCM or 32-bit float, mono or stereo, up to 60 seconds; all 64 slots share about 64 MB of memory) in the module's `data/imports/` folder, or use the web page's *Load into slot* upload. Then load a file on the Push's SYSTEM page. The UW-SMP machine plays a slot: its **WAVE** parameter picks the slot (two steps of the encoder per slot: value 0 = slot 1, 2 = slot 2, ... 126 = slot 64), **STRT** and **END** trim, **LOOP** loops, **REV** reverses, **PTCH** transposes, **DEC** and **HOLD** shape the envelope.

## The web page

Open it from the Push Hack menu. It has the step grid, every parameter of every track, the global effects, kit load/save, the sample manager, project download/upload and the Live setup. It is a second view of the same instrument: changes show up on the Push and vice versa.

## Audio output

By default push-audio-loopback sends MachineModule to Live over Ableton Link Audio. In Live, turn on Link and Link Audio, give an audio track the input type *PushHack*, pick the channel *MachineModule* and set Monitor to In. The channel pair below applies to the virtual card mode only. The *Create tracks in Live* button makes its track on the virtual card, so in Link mode set the input by hand. After a restart of push-audio-loopback, press play in Live to bring the sequencer back in time.

Virtual card mode: MachineModule sends its audio to the audio mixer of push-audio-loopback when it runs, so it can play together with other audio hacks such as Braids. Choose the channel pair on the web page (*Channel pair*). In Live, enable that pair on the input of the Push Hack Virtual Audio card and give an audio track that input (the *Create tracks in Live* button does this). Without the mixer, MachineModule writes to the card directly, the *Audio output* list is active, and no other audio hack can play at the same time.

## MIDI

The module offers one MIDI input, **MachineModule MIDI In**. Route a Live MIDI track or external gear to it.

| Message | Action |
|---|---|
| Notes 36-51 | Trigger tracks 1-16 (velocity = level) |
| CC 16-111 | Track parameters: 24 per track in the order SYNTH 1-8, EFFECT 1-8, ROUTE 1-8; tracks 1-4 on the base channel, 5-8 on the next channel, 9-12 on the third, 13-16 on the fourth |
| CC 112 / 113 / 114 | Master volume / tempo (30-157 BPM) / mute all (value 64 or more) |
| Program change | Select pattern 1-128 |
| Clock, Start, Continue, Stop | Followed when Sync is MIDI |

The base channel is set on the web page (*External MIDI channel*; "any" means channel 1 for CCs, and notes on any channel).

## Files

On the device, in `/data/push-hack/hacks/machinemodule/`: `machinemodule-config.json` (settings, kept across updates) and `data/` with `project.json`, `waves/` (one WAV per slot) and `imports/`.
