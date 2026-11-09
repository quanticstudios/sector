//go:build !linux

package procs

import "errors"

type scanner struct{}

func (s *Scanner) scan() (Table, error) { return Table{}, errors.ErrUnsupported }
