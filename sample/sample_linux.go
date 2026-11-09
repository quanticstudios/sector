package sample

import "time"

type sampler struct{}

// sample is a placeholder until the /proc reader lands.
func (s *Sampler) sample() (Snapshot, error) { return Snapshot{Time: time.Now()}, nil }
