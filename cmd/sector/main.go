// Command sector is a system monitor that groups CPU and memory by the
// coding agent, repo or app using them.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/sample"
	"github.com/quanticstudios/sector/theme"
	"github.com/quanticstudios/sector/view"
)

// version is set at build time with -ldflags "-X main.version=v0.1.0".
var version = "dev"

var (
	versionFlag = flag.Bool("version", false, "print the version and exit")
	demoFlag    = flag.Bool("demo", false, "show a made-up busy machine instead of this one")
	sizeFlag    = flag.String("size", "1440x900", "window size in dp, WxH")
)

func main() {
	flag.Parse()
	if *versionFlag {
		fmt.Println("sector", version)
		return
	}
	go func() {
		if err := run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

type reading struct {
	s sample.Snapshot
	t procs.Table
}

// backlog holds the readings taken since the window last drew. A window
// on another workspace gets no frames, so the sampler keeps reading into
// it and the next frame pushes them all; it keeps the newest
// view.History, which covers the graphs.
type backlog struct {
	mu sync.Mutex
	rs []reading
}

func (b *backlog) add(r reading) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rs = append(b.rs, r)
	if n := len(b.rs) - view.History; n > 0 {
		b.rs = append(b.rs[:0], b.rs[n:]...)
	}
}

// take empties the backlog and returns what it held, oldest first.
func (b *backlog) take() []reading {
	b.mu.Lock()
	defer b.mu.Unlock()
	rs := b.rs
	b.rs = nil
	return rs
}

func run() error {
	w := new(app.Window)
	var ww, wh float32
	if _, err := fmt.Sscanf(*sizeFlag, "%gx%g", &ww, &wh); err != nil || ww <= 0 || wh <= 0 {
		return fmt.Errorf("-size %q: want WxH, like 1440x900", *sizeFlag)
	}
	w.Option(app.Title("sector"), app.Size(unit.Dp(ww), unit.Dp(wh)))
	v := view.New(theme.Dark())
	readings := new(backlog)
	read := machine()
	if *demoFlag {
		d := newDemo()
		// Fill the 2 minute window so the graphs start full.
		now := time.Now()
		for i := 120; i > 0; i-- {
			s, t := d.next()
			s.Time = now.Add(-time.Duration(i) * time.Second)
			v.Push(s, t)
		}
		read = func() reading { s, t := d.next(); return reading{s, t} }
	}
	go sampleLoop(w, v, read, readings)
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			for _, r := range readings.take() {
				v.Push(r.s, r.t)
			}
			gtx := app.NewContext(&ops, e)
			v.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// machine reads this machine's counters and processes.
func machine() func() reading {
	smp, scn := sample.New(), procs.NewScanner()
	return func() reading {
		s, err := smp.Sample()
		if err != nil {
			log.Printf("sample: %v", err)
		}
		t, err := scn.Scan()
		if err != nil {
			log.Printf("procs: %v", err)
		}
		return reading{s, t}
	}
}

// sampleLoop calls read every interval, adds the reading to out and wakes
// the window. The interval and pause are read from v between samples; a
// stale read only delays the change by one tick.
func sampleLoop(w *app.Window, v *view.View, read func() reading, out *backlog) {
	for {
		if !v.Paused() {
			out.add(read())
			w.Invalidate()
		}
		time.Sleep(v.Interval())
	}
}
