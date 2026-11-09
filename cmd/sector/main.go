// Command sector is a system monitor that groups CPU and memory by the
// coding agent, repo or app using them.
package main

import (
	"log"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/sample"
	"github.com/quanticstudios/sector/theme"
	"github.com/quanticstudios/sector/view"
)

func main() {
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

func run() error {
	w := new(app.Window)
	w.Option(app.Title("sector"), app.Size(unit.Dp(1440), unit.Dp(900)))
	v := view.New(theme.Dark())
	readings := make(chan reading, 1)
	go sampleLoop(w, v, readings)
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			select {
			case r := <-readings:
				v.Push(r.s, r.t)
			default:
			}
			gtx := app.NewContext(&ops, e)
			v.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// sampleLoop reads the machine every interval and wakes the window. The
// interval and pause are read from v between samples; a stale read only
// delays the change by one tick.
func sampleLoop(w *app.Window, v *view.View, out chan reading) {
	smp, scn := sample.New(), procs.NewScanner()
	for {
		if !v.Paused() {
			s, err := smp.Sample()
			if err != nil {
				log.Printf("sample: %v", err)
			}
			t, err := scn.Scan()
			if err != nil {
				log.Printf("procs: %v", err)
			}
			select {
			case <-out:
			default:
			}
			out <- reading{s, t}
			w.Invalidate()
		}
		time.Sleep(v.Interval())
	}
}
