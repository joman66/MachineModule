package engine

import (
	"math"
	"time"
)

// ---------------------------------------------------------- the clock

// fireTime is when step k of pat fires, in ticks from the start of the
// pattern. Step Len is the end of the pattern. A swung step is late by a
// fraction of a step: 50% is straight, 66% is triplet swing, 75% is half a
// step late.
func (e *Engine) fireTime(pat *Pattern, k int) float64 {
	st := float64(StepTicks(pat.Scale))
	if k >= pat.Len {
		return st * float64(pat.Len)
	}
	t := st * float64(k)
	if pat.Swing[k] && pat.SwingAmt > 50 {
		t += (2*float64(pat.SwingAmt)/100 - 1) * st
	}
	return t
}

func (e *Engine) rewindLocked() {
	e.tick, e.nextStep, e.playStep = 0, 0, 0
	e.extTicks = 0
}

// stepEventLocked fires whatever is due at the next step, or moves to the
// next pattern at the end of this one.
func (e *Engine) stepEventLocked() {
	pat := e.proj.Pattern(e.curPat)
	if e.nextStep >= pat.Len {
		e.tick -= e.fireTime(pat, pat.Len)
		if e.tick < 0 {
			e.tick = 0
		}
		e.advancePatternLocked()
		e.nextStep = 0
		return
	}
	k := e.nextStep
	e.playStep = k
	e.nextStep++
	vel := float32(velNormal)
	if pat.Accent[k] {
		vel = velNormal + (velFull-velNormal)*clampf(float32(pat.AccentAmt)/127*1.4, 0, 1)
	}
	stepSamples := int(float64(StepTicks(pat.Scale)) / e.ticksPerSample())
	for t := 0; t < NumTracks; t++ {
		tp := &pat.Tracks[t]
		if !tp.Trig[k] {
			continue
		}
		tr := &e.tracks[t]
		tr.lockMask = 0
		if l := tp.Lock[k]; l != nil {
			tr.lockMask = l.Mask
			for i := 0; i < NParams; i++ {
				if l.Has(i) {
					tr.lockVal[i] = float32(l.Val[i]) / 127
				}
			}
		}
		if pat.Slide[k] {
			tr.slideRemain = stepSamples
			if tr.active {
				continue // slide: the sound keeps going, the parameters glide
			}
		} else {
			tr.slideRemain = 0
		}
		e.fireLocked(t, vel, 0)
	}
}

// advancePatternLocked picks the pattern that follows the one that just
// ended: the next song row, a queued pattern, or the same one again.
func (e *Engine) advancePatternLocked() {
	next := e.curPat
	if e.songMode {
		s := e.proj.Song(e.song)
		if len(s.Rows) == 0 {
			e.playing = false
			return
		}
		e.songRep++
		if e.songRow < 0 || e.songRow >= len(s.Rows) {
			e.songRow, e.songRep = 0, 0
		} else if e.songRep >= s.Rows[e.songRow].Repeats {
			e.songRow++
			e.songRep = 0
			if e.songRow >= len(s.Rows) {
				if s.LoopTo >= 0 && s.LoopTo < len(s.Rows) {
					e.songRow = s.LoopTo
				} else {
					e.songRow = -1
					e.playing = false
					return
				}
			}
		}
		next = s.Rows[e.songRow].Pattern
	} else if e.queued >= 0 {
		next = e.queued
		e.queued = -1
	}
	if next != e.curPat {
		e.setPatternLocked(next)
	}
}

func (e *Engine) setPatternLocked(n int) {
	e.curPat = clampi(n, 0, NumPatterns-1)
	if k := e.proj.Pattern(e.curPat).Kit; k >= 0 && k != e.kitN {
		e.loadKitLocked(k)
	}
}

// ------------------------------------------------------------ transport

// Play starts the sequencer from the top of the pattern (or of the song).
func (e *Engine) Play() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sync == SyncMIDI {
		return
	}
	e.startLocked()
}

func (e *Engine) startLocked() {
	if e.songMode {
		s := e.proj.Song(e.song)
		if len(s.Rows) == 0 {
			return
		}
		e.songRow, e.songRep = 0, 0
		e.setPatternLocked(s.Rows[0].Pattern)
	}
	e.rewindLocked()
	e.extTarget = 0
	e.playing = true
}

// Stop halts the sequencer. Sounds already playing ring out.
func (e *Engine) Stop() {
	e.mu.Lock()
	if e.sync == SyncMIDI {
		e.mu.Unlock()
		return
	}
	e.playing = false
	e.mu.Unlock()
}

// TogglePlay is Play when stopped and Stop when playing.
func (e *Engine) TogglePlay() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sync == SyncMIDI {
		return
	}
	if e.playing {
		e.playing = false
	} else {
		e.startLocked()
	}
}

// LocalTransport reports whether Play and Stop work from the module. With MIDI
// sync Live is the only transport: the module starts, stops and finds its place
// with Live (Follower, MIDI Start/Stop). A Play or Stop pressed here would only
// put it out of step with Live and with other modules, so Play, Stop and
// TogglePlay do nothing then.
func (e *Engine) LocalTransport() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.sync != SyncMIDI }

// Playing reports whether the sequencer runs.
func (e *Engine) Playing() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.playing }

// SetTempo sets the internal tempo, 30..300 BPM.
func (e *Engine) SetTempo(bpm float64) {
	e.mu.Lock()
	e.bpm = math.Min(300, math.Max(30, bpm))
	e.proj.Tempo = e.bpm
	e.dirty = true
	e.mu.Unlock()
}

// Tempo is the tempo in use: the internal one, or the one measured from the
// incoming MIDI clock.
func (e *Engine) Tempo() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tempoLocked()
}

// tempoLocked is the tempo the sequencer and the tempo-synced delay run at:
// the internal one, or with MIDI sync the measured MIDI clock, else Live's
// tempo from the Remote Script, else the internal one.
func (e *Engine) tempoLocked() float64 {
	if e.sync == SyncMIDI {
		if e.extBPM > 0 {
			return e.extBPM
		}
		if e.liveBPM > 0 {
			return e.liveBPM
		}
	}
	return e.bpm
}

// LiveTempo tells the engine Live's tempo, read from the PushHack Remote
// Script. It is used while MIDI sync is on and no MIDI clock arrives (Live
// sends none unless sync is enabled for the port, and none while stopped); a
// clock that is still arriving wins. now is the time of the reading.
func (e *Engine) LiveTempo(bpm float64, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if bpm < 20 || bpm > 999 || math.IsNaN(bpm) {
		return
	}
	if e.extBPM > 0 && !e.extLast.IsZero() && now.Sub(e.extLast) > time.Second {
		e.extBPM = 0 // the clock stopped; do not hold its last tempo
	}
	e.liveBPM = math.Min(300, math.Max(30, bpm))
}

// SetRecord turns real-time recording of hits on or off.
func (e *Engine) SetRecord(on bool) { e.mu.Lock(); e.rec = on; e.mu.Unlock() }

// Recording reports record mode.
func (e *Engine) Recording() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.rec }

// SelectPattern picks a pattern. While playing it is queued and starts when
// the current pattern ends, unless now is true. When stopped it is selected
// at once.
func (e *Engine) SelectPattern(n int, now bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n = clampi(n, 0, NumPatterns-1)
	if e.songMode {
		return
	}
	if !e.playing || now {
		e.queued = -1
		e.setPatternLocked(n)
		if e.playing {
			e.rewindLocked()
		}
		return
	}
	if n == e.curPat {
		e.queued = -1
	} else {
		e.queued = n
	}
}

// SetSong turns song mode on (playing song n) or off.
func (e *Engine) SetSong(on bool, n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.songMode = on
	e.song = clampi(n, 0, NumSongs-1)
	e.queued = -1
	if !on {
		e.songRow = -1
	}
}

// SongMode reports song mode and the song in use.
func (e *Engine) SongMode() (bool, int) { e.mu.Lock(); defer e.mu.Unlock(); return e.songMode, e.song }

// Position is the sequencer position, for the screens.
func (e *Engine) Position() Position {
	e.mu.Lock()
	defer e.mu.Unlock()
	row := -1
	if e.songMode {
		row = e.songRow
	}
	return Position{Playing: e.playing, Step: e.playStep, Pattern: e.curPat,
		Next: e.queued, Row: row, Repeat: e.songRep, Tick: e.tick}
}

// ---------------------------------------------------------- MIDI clock

// SetSync chooses the clock source.
func (e *Engine) SetSync(m SyncMode) { e.mu.Lock(); e.sync = m; e.mu.Unlock() }

// Sync is the clock source.
func (e *Engine) Sync() SyncMode { e.mu.Lock(); defer e.mu.Unlock(); return e.sync }

// ExtStart is MIDI Start: play from the top. Ignored unless sync is MIDI.
func (e *Engine) ExtStart() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sync != SyncMIDI {
		return
	}
	e.startLocked()
	e.extTarget = 0
}

// ExtJoin starts the sequencer in the middle of a song: Live was already
// playing when MachineModule came up, so no MIDI Start arrived. beat is Live's
// song position in quarter notes. The pattern starts where it would be had it
// been started with Live at beat 0, so the grids line up. The MIDI clock
// pulses that follow keep it there. Ignored unless sync is MIDI, or when the
// sequencer is already playing.
func (e *Engine) ExtJoin(beat float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sync != SyncMIDI || e.playing || beat < 0 || math.IsNaN(beat) || math.IsInf(beat, 0) {
		return
	}
	e.startLocked()
	if !e.playing { // an empty song: nothing to play
		return
	}
	pat := e.proj.Pattern(e.curPat)
	total := e.fireTime(pat, pat.Len)
	pos := math.Mod(beat*TicksPerBeat, total)
	k := 0
	for k < pat.Len && e.fireTime(pat, k) < pos-1e-6 {
		k++
	}
	e.tick, e.nextStep = pos, k
	e.playStep = clampi(k-1, 0, pat.Len-1)
	e.extTicks, e.extTarget = pos, pos
}

// ExtContinue is MIDI Continue.
func (e *Engine) ExtContinue() {
	e.mu.Lock()
	if e.sync == SyncMIDI {
		e.playing = true
	}
	e.mu.Unlock()
}

// ExtStop is MIDI Stop.
func (e *Engine) ExtStop() {
	e.mu.Lock()
	if e.sync == SyncMIDI {
		e.playing = false
	}
	e.mu.Unlock()
}

// ExtPulse is one MIDI clock pulse (24 per quarter note). now is when it
// arrived. It measures the tempo and tells the sequencer where the clock is.
func (e *Engine) ExtPulse(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sync != SyncMIDI {
		return
	}
	if !e.extLast.IsZero() {
		dt := now.Sub(e.extLast).Seconds()
		if dt > 0.0005 && dt < 0.5 {
			inst := 60 / (dt * 24)
			if e.extBPM == 0 || math.Abs(inst-e.extBPM) > e.extBPM*0.5 {
				e.extBPM = inst
			} else {
				e.extBPM += (inst - e.extBPM) * 0.05
			}
			e.extBPM = math.Min(300, math.Max(30, e.extBPM))
		} else if dt >= 0.5 {
			e.extBPM = 0
		}
	}
	e.extLast = now
	e.extTarget += TicksPerBeat / 24
}
