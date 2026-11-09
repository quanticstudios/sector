package sample

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
)

// TestGobRoundTrip checks a Snapshot crosses an embedder's gob wire
// unchanged, but for Time's monotonic reading, which gob drops.
func TestGobRoundTrip(t *testing.T) {
	useTree(t, "testdata/devbox")
	stubFS(t, devboxSizes)
	snap, err := New().Sample()
	if err != nil {
		t.Fatal(err)
	}
	snap.Time = snap.Time.Round(0)
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(snap); err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err := gob.NewDecoder(&buf).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, snap) {
		t.Errorf("after gob:\n%+v\nwant\n%+v", got, snap)
	}
}
