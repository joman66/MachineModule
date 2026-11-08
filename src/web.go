package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/alsapcm"
	"github.com/federico-pepe/ableton-push-hack/core/httpx"
	"github.com/federico-pepe/ableton-push-hack/core/sse"

	"machinemodule/audioout"
	"machinemodule/engine"
	"machinemodule/livelink"
	"machinemodule/store"
)

//go:embed web/index.html
var webFS embed.FS

type webServer struct {
	a       *app
	broker  *sse.Broker[[]byte]
	clients atomic.Int32 // open /sse/state streams
}

type waveInfo struct {
	Slot int    `json:"slot"`
	Name string `json:"name"`
	Rate int    `json:"rate"`
	Len  int    `json:"len"`
}

func (w *webServer) state() map[string]any {
	a := w.a
	e := a.eng
	pos := e.Position()
	pat := e.PatternCopy(-1)
	ok, msg := a.status.get()
	tracks := make([]map[string]any, engine.NumTracks)
	for t := range tracks {
		var p [engine.NParams]int
		for i := range p {
			p[i] = e.Param(t, i)
		}
		m := e.Machine(t)
		tracks[t] = map[string]any{"machine": m, "name": engine.Machines[m].Name,
			"mute": e.Muted(t), "peak": e.Peak(t), "params": p}
	}
	grid := make([]string, engine.NumTracks)
	for t := range grid {
		b := make([]byte, pat.Len)
		for s := range b {
			b[s] = '.'
			if pat.Tracks[t].Trig[s] {
				b[s] = 'x'
				if pat.Tracks[t].Lock[s] != nil {
					b[s] = 'L'
				}
			}
		}
		grid[t] = string(b)
	}
	var global [engine.NGlobal]int
	for i := range global {
		global[i] = e.Global(i)
	}
	used, total := e.WaveMemory()
	waves := []waveInfo{}
	for i := 0; i < engine.NumWaves; i++ {
		if wv := e.Wave(i); wv != nil {
			waves = append(waves, waveInfo{i, wv.Name, wv.Rate, len(wv.Data)})
		}
	}
	songOn, song := e.SongMode()
	cfg := a.config()
	return map[string]any{
		"playing": pos.Playing, "step": pos.Step, "pattern": pos.Pattern,
		"patternName": engine.PatternName(pos.Pattern), "next": pos.Next,
		"tempo": e.InternalTempo(), "tempoNow": e.Tempo(), "record": e.Recording(), "sync": int(e.Sync()),
		"song": map[string]any{"on": songOn, "n": song, "row": pos.Row},
		"kit":  map[string]any{"n": e.KitNumber(), "name": e.KitName()},
		"len":  pat.Len, "swing": pat.SwingAmt, "accentAmt": pat.AccentAmt,
		"grid": grid, "tracks": tracks, "global": global,
		"master": e.MasterPeak(),
		"audio":  map[string]any{"ok": ok, "message": msg},
		"waves":  waves, "waveUsed": used, "waveTotal": total,
		"imports": append([]string{}, a.st.ImportFiles()...),
		"config": map[string]any{"pcm_device": cfg.PCMDevice, "channel_offset": a.effectivePair(), "bus": a.busMode.Load(), "link": a.linkMode(),
			"midi_channel": cfg.MidiChannel},
	}
}

func machineList() []map[string]any {
	out := make([]map[string]any, len(engine.Machines))
	for i, m := range engine.Machines {
		out[i] = map[string]any{"id": i, "name": m.Name, "group": m.Group, "params": m.Params}
	}
	return out
}

func bad(w http.ResponseWriter, code int, err error) {
	http.Error(w, err.Error(), code)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func ok(w http.ResponseWriter) { httpx.JSON(w, map[string]bool{"ok": true}) }

func slotOf(r *http.Request) (int, error) {
	n, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil || n < 1 || n > engine.NumWaves {
		return 0, errors.New("slot must be 1-64")
	}
	return n - 1, nil
}

func (w *webServer) routes() http.Handler {
	a := w.a
	e := a.eng
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", func(rw http.ResponseWriter, r *http.Request) { httpx.JSON(rw, w.state()) })
	mux.HandleFunc("GET /api/machines", func(rw http.ResponseWriter, r *http.Request) {
		httpx.JSON(rw, map[string]any{"machines": machineList(), "fx": engine.FXNames, "global": engine.GlobalNames})
	})
	mux.HandleFunc("GET /sse/state", func(rw http.ResponseWriter, r *http.Request) {
		ch := w.broker.Register()
		w.clients.Add(1)
		defer func() { w.clients.Add(-1); w.broker.Unregister(ch) }()
		_ = sse.Serve(rw, r, ch, func(b []byte) ([]byte, error) { return b, nil })
	})
	mux.HandleFunc("POST /api/transport", func(rw http.ResponseWriter, r *http.Request) {
		var q struct {
			Action string  `json:"action"`
			Value  float64 `json:"value"`
		}
		if err := readJSON(r, &q); err != nil {
			bad(rw, 400, err)
			return
		}
		switch q.Action {
		case "play":
			e.Play()
		case "stop":
			e.Stop()
		case "toggle":
			e.TogglePlay()
		case "record":
			e.SetRecord(q.Value != 0)
		case "tempo":
			e.SetTempo(q.Value)
		case "sync":
			m := engine.SyncInternal
			if q.Value != 0 {
				m = engine.SyncMIDI
			}
			e.SetSync(m)
		default:
			bad(rw, 400, errors.New("unknown action"))
			return
		}
		ok(rw)
	})
	mux.HandleFunc("POST /api/hit", func(rw http.ResponseWriter, r *http.Request) {
		var q struct {
			Track int     `json:"track"`
			Vel   float64 `json:"vel"`
		}
		if err := readJSON(r, &q); err != nil || q.Track < 0 || q.Track >= engine.NumTracks {
			bad(rw, 400, errors.New("bad track"))
			return
		}
		if q.Vel <= 0 || q.Vel > 1 {
			q.Vel = 0.8
		}
		e.Hit(q.Track, float32(q.Vel))
		ok(rw)
	})
	mux.HandleFunc("POST /api/param", func(rw http.ResponseWriter, r *http.Request) {
		var q struct{ Track, Param, Value int }
		if err := readJSON(r, &q); err != nil || q.Track < 0 || q.Track >= engine.NumTracks || q.Param < 0 || q.Param >= engine.NParams {
			bad(rw, 400, errors.New("bad parameter"))
			return
		}
		e.SetParam(q.Track, q.Param, q.Value)
		ok(rw)
	})
	mux.HandleFunc("POST /api/global", func(rw http.ResponseWriter, r *http.Request) {
		var q struct{ Index, Value int }
		if err := readJSON(r, &q); err != nil || q.Index < 0 || q.Index >= engine.NGlobal {
			bad(rw, 400, errors.New("bad parameter"))
			return
		}
		e.SetGlobal(q.Index, q.Value)
		ok(rw)
	})
	mux.HandleFunc("POST /api/machine", func(rw http.ResponseWriter, r *http.Request) {
		var q struct{ Track, Machine int }
		if err := readJSON(r, &q); err != nil || q.Track < 0 || q.Track >= engine.NumTracks || q.Machine < 0 || q.Machine >= len(engine.Machines) {
			bad(rw, 400, errors.New("bad machine"))
			return
		}
		e.SetMachine(q.Track, q.Machine)
		ok(rw)
	})
	mux.HandleFunc("POST /api/mute", func(rw http.ResponseWriter, r *http.Request) {
		var q struct {
			Track int
			Mute  bool
		}
		if err := readJSON(r, &q); err != nil || q.Track < 0 || q.Track >= engine.NumTracks {
			bad(rw, 400, errors.New("bad track"))
			return
		}
		e.SetMute(q.Track, q.Mute)
		ok(rw)
	})
	mux.HandleFunc("POST /api/trig", func(rw http.ResponseWriter, r *http.Request) {
		var q struct {
			Track, Step int
			On          bool
		}
		if err := readJSON(r, &q); err != nil || q.Track < 0 || q.Track >= engine.NumTracks || q.Step < 0 || q.Step >= engine.MaxSteps {
			bad(rw, 400, errors.New("bad step"))
			return
		}
		e.SetTrig(-1, q.Track, q.Step, q.On)
		ok(rw)
	})
	mux.HandleFunc("POST /api/pattern", func(rw http.ResponseWriter, r *http.Request) {
		var q struct {
			N   int
			Now bool
		}
		if err := readJSON(r, &q); err != nil {
			bad(rw, 400, err)
			return
		}
		if q.N < 0 || q.N >= engine.NumPatterns {
			bad(rw, 400, errors.New("pattern 0-127"))
			return
		}
		e.SelectPattern(q.N, q.Now)
		ok(rw)
	})
	mux.HandleFunc("POST /api/kit", func(rw http.ResponseWriter, r *http.Request) {
		var q struct {
			Action string
			N      int
			Name   string
		}
		if err := readJSON(r, &q); err != nil || q.N < 0 || q.N >= engine.NumKits {
			bad(rw, 400, errors.New("kit 0-63"))
			return
		}
		switch q.Action {
		case "load":
			e.LoadKit(q.N)
		case "save":
			e.SaveKit(q.N, q.Name)
		case "reset":
			e.ResetKit()
		default:
			bad(rw, 400, errors.New("unknown action"))
			return
		}
		ok(rw)
	})
	// UserWave: PUT a WAV body (?name=file.wav) or choose one from imports.
	mux.HandleFunc("PUT /api/wave/{slot}", func(rw http.ResponseWriter, r *http.Request) {
		slot, err := slotOf(r)
		if err != nil {
			bad(rw, 400, err)
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			name = fmt.Sprintf("UW%02d.wav", slot+1)
		}
		if err := a.st.UploadWave(slot, name, r.Body); err != nil {
			bad(rw, 422, err)
			return
		}
		ok(rw)
	})
	mux.HandleFunc("POST /api/wave/{slot}/import", func(rw http.ResponseWriter, r *http.Request) {
		slot, err := slotOf(r)
		if err != nil {
			bad(rw, 400, err)
			return
		}
		if err := a.st.ImportWave(slot, r.URL.Query().Get("file")); err != nil {
			bad(rw, 422, err)
			return
		}
		ok(rw)
	})
	mux.HandleFunc("DELETE /api/wave/{slot}", func(rw http.ResponseWriter, r *http.Request) {
		slot, err := slotOf(r)
		if err != nil {
			bad(rw, 400, err)
			return
		}
		e.ClearWave(slot)
		ok(rw)
	})
	mux.HandleFunc("GET /api/wave/{slot}", func(rw http.ResponseWriter, r *http.Request) {
		slot, err := slotOf(r)
		if err != nil {
			bad(rw, 400, err)
			return
		}
		wv := e.Wave(slot)
		if wv == nil {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "audio/wav")
		_, _ = rw.Write(engine.EncodeWAV16(wv.Data, wv.Rate, 1))
	})
	mux.HandleFunc("GET /api/project", func(rw http.ResponseWriter, r *http.Request) {
		b, err := a.st.ProjectJSON()
		if err != nil {
			bad(rw, 500, err)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Content-Disposition", `attachment; filename="machinemodule-project.json"`)
		_, _ = rw.Write(b)
	})
	mux.HandleFunc("POST /api/project", func(rw http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			bad(rw, 400, err)
			return
		}
		if err := a.st.ReplaceProject(b); err != nil {
			bad(rw, 422, err)
			return
		}
		ok(rw)
	})
	mux.HandleFunc("POST /api/config", func(rw http.ResponseWriter, r *http.Request) {
		var q struct {
			ChannelOffset *int    `json:"channel_offset"`
			MidiChannel   *int    `json:"midi_channel"`
			PCMDevice     *string `json:"pcm_device"`
		}
		if err := readJSON(r, &q); err != nil {
			bad(rw, 400, err)
			return
		}
		devices, _ := alsapcm.EnumPlaybackDevices()
		if q.PCMDevice != nil && !audioout.HasDevice(devices, *q.PCMDevice) {
			bad(rw, 400, errors.New("unknown audio device"))
			return
		}
		if q.ChannelOffset != nil && !audioout.ValidPair(int(a.channels.Load()), *q.ChannelOffset) {
			bad(rw, 400, errors.New("channel pair not on this card"))
			return
		}
		err := a.setConfig(func(c *store.Config) {
			if q.ChannelOffset != nil {
				c.ChannelOffset = *q.ChannelOffset
				c.BusPair = *q.ChannelOffset
			}
			if q.MidiChannel != nil && *q.MidiChannel >= -1 && *q.MidiChannel <= 15 {
				c.MidiChannel = *q.MidiChannel
			}
			if q.PCMDevice != nil {
				c.PCMDevice = *q.PCMDevice
			}
		})
		if err != nil {
			bad(rw, 500, err)
			return
		}
		if c := a.busClient.Load(); c != nil && q.ChannelOffset != nil {
			c.SetPair(*q.ChannelOffset)
		}
		ok(rw)
	})
	// The audio outputs on offer. Read on demand: listing devices reads /proc,
	// too much for the 100 ms state broadcast.
	mux.HandleFunc("GET /api/audio", func(rw http.ResponseWriter, r *http.Request) {
		devices, _ := alsapcm.EnumPlaybackDevices()
		cfg := a.config()
		httpx.JSON(rw, map[string]any{
			"devices": audioout.Devices(devices, cfg.PCMDevice),
			"pairs":   audioout.Pairs(int(a.channels.Load()), a.effectivePair()),
			"bus":     a.busMode.Load(),
			"link":    a.linkMode(),
		})
	})
	mux.HandleFunc("POST /api/live/tracks", func(rw http.ResponseWriter, r *http.Request) {
		res, err := livelink.EnsureTracks(livelink.Call, a.effectivePair())
		if err != nil {
			bad(rw, 502, err)
			return
		}
		httpx.JSON(rw, res)
	})
	mux.HandleFunc("/", httpx.ServeEmbedded(webFS, "web/index.html"))
	return httpx.WithLogging(httpx.WithCORS("GET, POST, PUT, DELETE, OPTIONS", mux))
}

func (a *app) runWeb(port int, shutdown <-chan struct{}) {
	w := &webServer{a: a, broker: sse.NewBroker[[]byte](8, false)}
	srv := httpx.NewServer(fmt.Sprintf(":%d", port), w.routes())
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-shutdown:
				return
			case <-t.C:
				if w.clients.Load() == 0 {
					continue
				}
				if b, err := json.Marshal(w.state()); err == nil {
					w.broker.Broadcast(b)
				}
			}
		}
	}()
	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	log.Printf("web UI on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("web server: %v", err)
	}
}
