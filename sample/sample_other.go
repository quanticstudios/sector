//go:build !linux

package sample

import "errors"

type sampler struct{}

func (s *Sampler) sample() (Snapshot, error) { return Snapshot{}, errors.ErrUnsupported }
