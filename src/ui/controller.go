// Package ui is MachineModule's front panel: it turns Push 3 pads, buttons
// and encoders into Machinedrum operations, draws the screen and decides
// what the LEDs show. It does no I/O of its own (the host moves the events
// and the frames), so all of it can be tested on any computer.
package ui

import (
	"fmt"
	"sync"
	"time"

	"machinemodule/engine"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

// Pages, in the order of the eight buttons above the screen.
const (
	PgSynth = iota
	PgFX
	PgRoute
	PgMaster
	PgSeq
	PgSong
	PgKit
	PgSystem
	numPages
)

var pageNames = [numPages]string{"SYNTH", "EFFECT", "ROUTE", "MASTER", "PATTERN", "SONG", "KIT", "SYSTEM"}

// PageName is the label of a page.
func PageName(p int) string {
	if p < 0 || p >= numPages {
		return ""
	}
	return pageNames[p]
}

type layerKind int

const (
	layerNone layerKind = iota
	layerAccent
	layerSwing
	layerSlide
)

// Host is what the front panel needs from the program around it.
type Host interface {
	// ImportFiles lists the WAV files that can be loaded into a slot.
	ImportFiles() []string
	// ImportWave loads one of them.
	ImportWave(slot int, file string) error
	// SaveProject writes the project to disk.
	SaveProject() error
	// AudioStatus says whether sound is going out, and if not why.
	AudioStatus() (ok bool, msg string)
	// SyncChanged is told when the user picks the clock source.
	SyncChanged(m engine.SyncMode)
}

type stepHold struct{ existed, edited bool }

type overlayKind int

const (
	ovNone overlayKind = iota
	ovMachine
)

// Controller is the state of the front panel.
type Controller struct {
	mu   sync.Mutex
	eng  *engine.Engine
	host Host
	now  func() time.Time

	page, track, sub  int
	stepPage, bank    int
	octave            int
	layer             layerKind
	muteMode          bool
	soloHeld          bool
	shift, cpy, clear bool
	held              map[int]*stepHold
	ov                overlayKind
	ovCursor          int
	toast             string
	toastUntil        time.Time
	taps              []time.Time
	songRow, songNum  int
	kitCursor         int
	waveSlot, fileCur int
	clipPat           int
	acc               [8]int
	lastTouch         int // encoder last turned, for highlighting
	lastTouchAt       time.Time
}

// New makes the panel for an engine.
func New(e *engine.Engine, h Host) *Controller {
	return &Controller{eng: e, host: h, now: time.Now, held: map[int]*stepHold{},
		octave: 0, clipPat: -1, lastTouch: -1, kitCursor: e.KitNumber()}
}

// SetClock replaces the time source (for tests).
func (c *Controller) SetClock(f func() time.Time) { c.now = f }

// Page is the page showing now.
func (c *Controller) Page() int { c.mu.Lock(); defer c.mu.Unlock(); return c.page }

// Track is the selected track (0..15).
func (c *Controller) Track() int { c.mu.Lock(); defer c.mu.Unlock(); return c.track }

func (c *Controller) say(format string, a ...any) {
	c.toast = fmt.Sprintf(format, a...)
	c.toastUntil = c.now().Add(1500 * time.Millisecond)
}

// ------------------------------------------------------------ helpers

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ticks turns a stream of encoder deltas into whole steps: one step every n
// units, remembering the remainder per encoder.
func (c *Controller) ticks(i, delta, n int) int {
	c.acc[i] += delta
	s := c.acc[i] / n
	c.acc[i] -= s * n
	return s
}

// paramIndex is the kit parameter that encoder i controls on the page, or -1.
func paramIndex(page, i int) int {
	switch page {
	case PgSynth:
		return i
	case PgFX:
		return engine.NSynth + i
	case PgRoute:
		return engine.NSynth + engine.NFX + i
	}
	return -1
}

func (c *Controller) pattern() engine.Pattern { return c.eng.PatternCopy(-1) }

// stepAbs converts the pad position of a step key to a step number.
func (c *Controller) stepAbs(note byte) int {
	col, row := push3.PadCoord(note)
	k := (1-row)*8 + col // row 1 holds steps 1-8, row 0 holds 9-16
	return c.stepPage*16 + k
}

// ------------------------------------------------------------- pads

// Pad handles a pad press (vel > 0) or release (vel == 0).
func (c *Controller) Pad(note byte, vel byte) {
	if !push3.IsPadNote(note) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	col, row := push3.PadCoord(note)
	down := vel > 0
	v := float32(vel) / 127
	if v < 0.35 {
		v = 0.35
	}
	switch {
	case row <= 1: // step keys
		pat := c.pattern()
		s := c.stepAbs(note)
		if s >= pat.Len {
			return
		}
		if down {
			c.stepDown(s, pat)
		} else {
			c.stepUp(s)
		}
	case !down:
		return
	case row <= 3: // track keys
		t := (3-row)*8 + col
		c.trackKey(t, v)
	case row <= 5: // pattern keys
		p := c.bank*16 + (5-row)*8 + col
		c.patternKey(p)
	default: // pitch keys
		k := (7-row)*8 + col
		semis := k - 4 + 12*c.octave
		base := c.eng.Param(c.track, 0)
		per := engine.PitchPerSemitone(c.eng.Machine(c.track))
		c.eng.HitPitched(c.track, v, base+int(float64(semis)*per+0.5*sign(semis)))
	}
}

func sign(v int) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func (c *Controller) stepDown(s int, pat engine.Pattern) {
	switch {
	case c.shift:
		c.eng.SetPatternLen(-1, s+1)
		c.say("LENGTH %d", s+1)
	case c.layer == layerAccent:
		c.eng.ToggleLayer(-1, engine.LayerAccent, s)
	case c.layer == layerSwing:
		c.eng.ToggleLayer(-1, engine.LayerSwing, s)
	case c.layer == layerSlide:
		c.eng.ToggleLayer(-1, engine.LayerSlide, s)
	case c.clear:
		c.eng.SetTrig(-1, c.track, s, false)
	default:
		existed := pat.Tracks[c.track].Trig[s]
		if !existed {
			c.eng.SetTrig(-1, c.track, s, true)
		}
		c.held[s] = &stepHold{existed: existed}
	}
}

func (c *Controller) stepUp(s int) {
	h := c.held[s]
	if h == nil {
		return
	}
	delete(c.held, s)
	if h.existed && !h.edited {
		c.eng.SetTrig(-1, c.track, s, false) // a second press takes the trig away
	}
}

func (c *Controller) trackKey(t int, vel float32) {
	switch {
	case c.muteMode:
		c.eng.SetMute(t, !c.eng.Muted(t))
	case c.soloHeld:
		solo := true
		for i := 0; i < engine.NumTracks; i++ {
			if i != t && !c.eng.Muted(i) {
				solo = false
			}
		}
		for i := 0; i < engine.NumTracks; i++ {
			c.eng.SetMute(i, !solo && i != t)
		}
	case c.clear:
		c.eng.ClearTrack(-1, t)
		c.say("%s CLEARED", engine.TrackNames[t])
	case c.cpy:
		c.eng.CopyTrack(-1, c.track, -1, t)
		c.say("%s TO %s", engine.TrackNames[c.track], engine.TrackNames[t])
	default:
		c.track = t
		c.eng.Hit(t, vel)
	}
}

func (c *Controller) patternKey(p int) {
	switch {
	case c.clear:
		c.eng.ClearPattern(p)
		c.say("%s CLEARED", engine.PatternName(p))
	case c.cpy:
		cur := c.eng.Position().Pattern
		c.eng.CopyPattern(cur, p)
		c.say("%s TO %s", engine.PatternName(cur), engine.PatternName(p))
	default:
		c.eng.SelectPattern(p, c.shift)
	}
}

// ---------------------------------------------------------- buttons

// Button handles a button press (down) or release.
func (c *Controller) Button(cc byte, down bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// hold buttons
	switch cc {
	case push3.CCShift:
		c.shift = down
		return
	case push3.CCDuplicate:
		c.cpy = down
		if down {
			c.clipPat = c.eng.Position().Pattern
			c.say("COPY: PRESS A TARGET")
		}
		return
	case push3.CCDelete:
		c.clear = down
		return
	case push3.CCSolo:
		c.soloHeld = down
		return
	}
	if !down {
		return
	}
	switch {
	case cc >= push3.CCScreenTop1 && cc <= push3.CCScreenTop8:
		c.setPage(int(cc - push3.CCScreenTop1))
	case cc >= push3.CCScreenBot1 && cc <= push3.CCScreenBot8:
		c.softKey(int(cc - push3.CCScreenBot1))
	case cc >= push3.CCScene14 && cc <= push3.CCScene132t:
		c.bank = 7 - int(cc-push3.CCScene14) // top button is bank A
	case cc == push3.CCPlay:
		if !c.eng.LocalTransport() {
			c.say("PLAY FOLLOWS LIVE (SYNC MIDI)")
		} else if c.shift {
			c.eng.Stop()
			c.eng.Play()
		} else {
			c.eng.TogglePlay()
		}
	case cc == push3.CCRecord:
		c.eng.SetRecord(!c.eng.Recording())
		c.say("REC %s", onOff(c.eng.Recording()))
	case cc == push3.CCTapTempo:
		c.tap()
	case cc == push3.CCMute:
		c.muteMode = !c.muteMode
	case cc == push3.CCAccent:
		c.toggleLayer(layerAccent)
	case cc == push3.CCQuantize:
		c.toggleLayer(layerSwing)
	case cc == push3.CCRepeat:
		c.toggleLayer(layerSlide)
	case cc == push3.CCConvert: // paste
		if c.clipPat >= 0 {
			cur := c.eng.Position().Pattern
			c.eng.CopyPattern(c.clipPat, cur)
			c.say("PASTED %s", engine.PatternName(c.clipPat))
		}
	case cc == push3.CCSave:
		c.save()
	case cc == push3.CCSettings:
		c.setPage(PgSystem)
	case cc == push3.CCUndo: // EXIT / NO
		c.ov = ovNone
	case cc == push3.CCJogPress || cc == push3.CCDPadCenter: // ENTER / YES
		c.enter()
	case cc == push3.CCDPadLeft:
		c.track = (c.track + engine.NumTracks - 1) % engine.NumTracks
	case cc == push3.CCDPadRight:
		c.track = (c.track + 1) % engine.NumTracks
	case cc == push3.CCDPadUp:
		c.eng.SelectPattern(c.eng.Position().Pattern+1, c.shift)
	case cc == push3.CCDPadDown:
		c.eng.SelectPattern(c.eng.Position().Pattern-1, c.shift)
	case cc == push3.CCPageLeft:
		c.stepPage = clampi(c.stepPage-1, 0, 3)
	case cc == push3.CCPageRight:
		c.stepPage = clampi(c.stepPage+1, 0, (c.pattern().Len-1)/16)
	case cc == push3.CCOctaveUp:
		c.octave = clampi(c.octave+1, -3, 3)
	case cc == push3.CCOctaveDown:
		c.octave = clampi(c.octave-1, -3, 3)
	}
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

func (c *Controller) setPage(p int) {
	if p == c.page && p == PgMaster {
		c.sub ^= 1
	}
	c.page = p
	c.ov = ovNone
	c.acc = [8]int{}
}

func (c *Controller) toggleLayer(l layerKind) {
	if c.layer == l {
		c.layer = layerNone
	} else {
		c.layer = l
	}
}

func (c *Controller) save() {
	if err := c.host.SaveProject(); err != nil {
		c.say("SAVE FAILED")
		return
	}
	c.say("PROJECT SAVED")
}

func (c *Controller) tap() {
	now := c.now()
	if n := len(c.taps); n > 0 && now.Sub(c.taps[n-1]) > 2*time.Second {
		c.taps = nil
	}
	c.taps = append(c.taps, now)
	if len(c.taps) > 5 {
		c.taps = c.taps[1:]
	}
	if len(c.taps) >= 2 {
		span := c.taps[len(c.taps)-1].Sub(c.taps[0]).Seconds() / float64(len(c.taps)-1)
		if span > 0 {
			c.eng.SetTempo(60 / span)
			c.say("TEMPO %.1f", c.eng.Tempo())
		}
	}
}

func (c *Controller) enter() {
	if c.ov == ovMachine {
		c.eng.SetMachine(c.track, c.ovCursor)
		c.say("%s", engine.Machines[c.ovCursor].Name)
		c.ov = ovNone
	}
}

// softKey runs bottom button i of the current page.
func (c *Controller) softKey(i int) {
	switch c.page {
	case PgSynth:
		switch i {
		case 0:
			if c.ov == ovMachine {
				c.enter()
			} else {
				c.ov = ovMachine
				c.ovCursor = c.eng.Machine(c.track)
			}
		case 1:
			c.track = (c.track + engine.NumTracks - 1) % engine.NumTracks
		case 2:
			c.track = (c.track + 1) % engine.NumTracks
		}
	case PgFX, PgRoute:
		switch i {
		case 1:
			c.track = (c.track + engine.NumTracks - 1) % engine.NumTracks
		case 2:
			c.track = (c.track + 1) % engine.NumTracks
		}
	case PgMaster:
		if i < 2 {
			c.sub = i
		}
	case PgSeq:
		pos := c.eng.Position()
		switch i {
		case 0:
			if c.shift {
				c.eng.ClearPattern(-1)
				c.say("PATTERN CLEARED")
			}
		case 1:
			c.clipPat = pos.Pattern
			c.say("COPIED %s", engine.PatternName(pos.Pattern))
		case 2:
			if c.clipPat >= 0 {
				c.eng.CopyPattern(c.clipPat, pos.Pattern)
				c.say("PASTED %s", engine.PatternName(c.clipPat))
			}
		case 3:
			c.eng.SetRecord(!c.eng.Recording())
		case 4:
			c.eng.ShiftTrack(-1, c.track, -1)
		case 5:
			c.eng.ShiftTrack(-1, c.track, 1)
		}
	case PgSong:
		s := c.eng.SongCopy(c.songNum)
		switch i {
		case 0:
			c.eng.SongSetRow(c.songNum, len(s.Rows), c.eng.Position().Pattern, 1)
			c.songRow = len(s.Rows)
		case 1:
			c.eng.SongDeleteRow(c.songNum, c.songRow)
			c.songRow = clampi(c.songRow, 0, len(s.Rows))
		case 2:
			on, _ := c.eng.SongMode()
			c.eng.SetSong(!on, c.songNum)
			c.say("SONG MODE %s", onOff(!on))
		}
	case PgKit:
		switch i {
		case 0:
			c.eng.LoadKit(c.kitCursor)
			c.say("KIT %02d LOADED", c.kitCursor+1)
		case 1:
			c.eng.SaveKit(c.kitCursor, "")
			c.say("KIT %02d SAVED", c.kitCursor+1)
		case 2:
			c.eng.ResetKit()
			c.say("KIT INIT")
		}
	case PgSystem:
		files := c.host.ImportFiles()
		switch i {
		case 0:
			if len(files) == 0 {
				c.say("NO WAV FILES")
				return
			}
			c.fileCur = clampi(c.fileCur, 0, len(files)-1)
			if err := c.host.ImportWave(c.waveSlot, files[c.fileCur]); err != nil {
				c.say("%s", upperASCII(err.Error(), 28))
			} else {
				c.say("UW-%02d LOADED", c.waveSlot+1)
			}
		case 1:
			c.eng.ClearWave(c.waveSlot)
			c.say("UW-%02d CLEARED", c.waveSlot+1)
		case 2:
			c.save()
		}
	}
}

func upperASCII(s string, n int) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b = append(b, byte(r-32))
		case r >= 32 && r < 127:
			b = append(b, byte(r))
		}
	}
	if len(b) > n {
		b = b[:n]
	}
	return string(b)
}

// ---------------------------------------------------------- encoders

// Encoder handles one encoder turn. cc is the encoder CC (71..78 for the
// eight knobs, 70 jog, 79 volume, 14 tempo); delta is the signed amount.
func (c *Controller) Encoder(cc byte, delta int) {
	if delta == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case cc >= push3.CCEncoder1 && cc <= push3.CCEncoder8:
		c.knob(int(cc-push3.CCEncoder1), delta)
	case cc == push3.CCJogWheel:
		c.jog(delta)
	case cc == push3.CCVolume:
		c.eng.SetGlobal(engine.GMVOL, c.eng.Global(engine.GMVOL)+delta)
		c.say("MASTER %d", c.eng.Global(engine.GMVOL))
	case cc == push3.CCTempo:
		d := float64(delta)
		if c.shift {
			d /= 10
		}
		c.eng.SetTempo(c.eng.InternalTempo() + d)
	}
}

func (c *Controller) jog(delta int) {
	switch {
	case c.ov == ovMachine:
		c.ovCursor = clampi(c.ovCursor+delta, 0, len(engine.Machines)-1)
	case c.page == PgKit:
		c.kitCursor = clampi(c.kitCursor+delta, 0, engine.NumKits-1)
	case c.page == PgSong:
		c.songRow = clampi(c.songRow+delta, 0, len(c.eng.SongCopy(c.songNum).Rows))
	case c.page == PgSystem:
		c.fileCur += delta
		if n := len(c.host.ImportFiles()); n > 0 {
			c.fileCur = clampi(c.fileCur, 0, n-1)
		} else {
			c.fileCur = 0
		}
	default:
		c.eng.SetParam(c.track, engine.VOL, c.eng.Param(c.track, engine.VOL)+delta)
	}
}

func (c *Controller) knob(i, delta int) {
	c.lastTouch, c.lastTouchAt = i, c.now()
	if c.ov == ovMachine {
		if i == 0 {
			c.ovCursor = clampi(c.ovCursor+delta, 0, len(engine.Machines)-1)
		}
		return
	}
	switch c.page {
	case PgSynth, PgFX, PgRoute:
		p := paramIndex(c.page, i)
		edited := false
		for s, h := range c.held {
			pat := c.pattern()
			if s >= pat.Len || !pat.Tracks[c.track].Trig[s] {
				continue
			}
			cur := c.eng.Param(c.track, p)
			if l, ok := c.eng.LockOf(-1, c.track, s); ok && l.Has(p) {
				cur = int(l.Val[p])
			}
			c.eng.SetLock(-1, c.track, s, p, cur+delta)
			h.edited = true
			edited = true
		}
		if !edited {
			c.eng.SetParam(c.track, p, c.eng.Param(c.track, p)+delta)
		}
	case PgMaster:
		g := c.sub*8 + i
		c.eng.SetGlobal(g, c.eng.Global(g)+delta)
	case PgSeq:
		c.seqKnob(i, delta)
	case PgSong:
		c.songKnob(i, delta)
	case PgKit:
		if i == 0 {
			c.kitCursor = clampi(c.kitCursor+delta, 0, engine.NumKits-1)
		}
	case PgSystem:
		switch i {
		case 0:
			c.waveSlot = clampi(c.waveSlot+c.ticks(i, delta, 1), 0, engine.NumWaves-1)
		case 1:
			c.jog(delta)
		case 6:
			if s := c.ticks(i, delta, 6); s != 0 {
				m := engine.SyncInternal
				if s > 0 {
					m = engine.SyncMIDI
				}
				c.eng.SetSync(m)
				c.host.SyncChanged(m)
			}
		}
	}
}

// SeqScaleNames etc. are used by the screen too.
func (c *Controller) seqKnob(i, delta int) {
	p := c.pattern()
	switch i {
	case 0:
		c.eng.SetPatternLen(-1, p.Len+delta)
	case 1:
		c.eng.SetPatternScale(-1, p.Scale+c.ticks(i, delta, 4))
	case 2:
		c.eng.SetSwingAmount(-1, p.SwingAmt+delta)
	case 3:
		c.eng.SetAccentAmount(-1, int(p.AccentAmt)+delta)
	case 4:
		c.eng.SetPatternKit(-1, p.Kit+c.ticks(i, delta, 3))
	case 5:
		d := float64(delta)
		if c.shift {
			d /= 10
		}
		c.eng.SetTempo(c.eng.InternalTempo() + d)
	case 6:
		if s := c.ticks(i, delta, 6); s != 0 {
			m := engine.SyncInternal
			if s > 0 {
				m = engine.SyncMIDI
			}
			c.eng.SetSync(m)
			c.host.SyncChanged(m)
		}
	case 7:
		if s := c.ticks(i, delta, 6); s != 0 {
			c.eng.SetSong(s > 0, c.songNum)
		}
	}
}

func (c *Controller) songKnob(i, delta int) {
	s := c.eng.SongCopy(c.songNum)
	switch i {
	case 0:
		c.songRow = clampi(c.songRow+c.ticks(i, delta, 2), 0, len(s.Rows))
	case 1:
		cur := c.eng.Position().Pattern
		if c.songRow < len(s.Rows) {
			cur = s.Rows[c.songRow].Pattern
		}
		rep := 1
		if c.songRow < len(s.Rows) {
			rep = s.Rows[c.songRow].Repeats
		}
		c.eng.SongSetRow(c.songNum, c.songRow, cur+c.ticks(i, delta, 2), rep)
	case 2:
		if c.songRow < len(s.Rows) {
			r := s.Rows[c.songRow]
			c.eng.SongSetRow(c.songNum, c.songRow, r.Pattern, r.Repeats+c.ticks(i, delta, 2))
		}
	case 3:
		c.eng.SongSetLoop(c.songNum, clampi(s.LoopTo+c.ticks(i, delta, 3), -1, len(s.Rows)-1))
	case 4:
		on, _ := c.eng.SongMode()
		c.songNum = clampi(c.songNum+c.ticks(i, delta, 4), 0, engine.NumSongs-1)
		if on {
			c.eng.SetSong(true, c.songNum)
		}
		c.songRow = 0
	}
}
