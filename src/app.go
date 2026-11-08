package main

import (
	"sync"
	"sync/atomic"

	"github.com/federico-pepe/ableton-push-hack/core/alsaseq"
	"github.com/federico-pepe/ableton-push-hack/core/audiobus"

	"machinemodule/engine"
	"machinemodule/store"
	"machinemodule/ui"
)

// app is everything the pieces share. The engine has its own lock; cfg has
// cfgMu; the rest is set once.
type app struct {
	eng     *engine.Engine
	ui      *ui.Controller
	st      *store.Store
	hackDir string
	dataDir string
	pmURL   string

	cfgMu sync.Mutex
	cfg   store.Config

	status audioStatus
	// channels is what Live negotiated on the card; 0 until it has.
	channels atomic.Int32
	// offset mirrors cfg.ChannelOffset for the render loop, which must not lock.
	offset atomic.Int32
	// busClient talks to the audio mixer (audiobus); busMode says it is in use.
	busClient atomic.Pointer[audiobus.Client]
	busMode   atomic.Bool
	// push3 is where Push 3's Live Port is; its client number moves.
	push3 alsaseq.Push3
	focus atomic.Bool // MachineModule owns the Push surface

	linkOnce sync.Once
	linkP    *audiobus.LinkProbe
}

// linkMode says the mixer sends audio to Live over Link. Then the card input
// the Create tracks button sets is wrong, so the page hides the audio setup.
func (a *app) linkMode() bool {
	a.linkOnce.Do(func() { a.linkP = audiobus.NewLinkProbe() })
	return a.busMode.Load() && a.linkP.Link()
}

func (a *app) config() store.Config {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	return a.cfg
}

func (a *app) setConfig(f func(*store.Config)) error {
	a.cfgMu.Lock()
	f(&a.cfg)
	c := a.cfg
	a.offset.Store(int32(c.ChannelOffset))
	a.cfgMu.Unlock()
	return store.SaveConfig(a.hackDir, c)
}

// ---- ui.Host ----

func (a *app) ImportFiles() []string                  { return a.st.ImportFiles() }
func (a *app) ImportWave(slot int, file string) error { return a.st.ImportWave(slot, file) }
func (a *app) SaveProject() error                     { return a.st.Save() }
func (a *app) AudioStatus() (bool, string)            { return a.status.get() }
func (a *app) SyncChanged(m engine.SyncMode)          { a.eng.SetSync(m) }

// audioStatus says whether sound is going out.
type audioStatus struct {
	mu  sync.Mutex
	ok  bool
	msg string
}

func (s *audioStatus) set(ok bool, msg string) {
	s.mu.Lock()
	s.ok, s.msg = ok, msg
	s.mu.Unlock()
}

func (s *audioStatus) get() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ok, s.msg
}
