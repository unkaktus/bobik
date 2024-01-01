package main

import (
	"io"
	"strings"

	"github.com/acarl005/stripansi"
)

type PromptFinder struct {
	reader        io.Reader
	checkFunction func(string) bool
	builder       *strings.Builder
	teeReader     io.Reader
	Found         chan struct{}
	disabled      chan struct{}
}

func NewPromptFinder(r io.Reader, check func(string) bool) *PromptFinder {
	builder := &strings.Builder{}
	return &PromptFinder{
		reader:        r,
		checkFunction: check,
		builder:       builder,
		teeReader:     io.TeeReader(r, builder),
		Found:         make(chan struct{}),
		disabled:      make(chan struct{}),
	}
}

func (pf *PromptFinder) Read(p []byte) (int, error) {
	select {
	case <-pf.disabled:
		if pf.builder.Len() != 0 {
			return pf.teeReader.Read(p)
		}
		return pf.reader.Read(p)
	default:
		n, err := pf.teeReader.Read(p)
		accumulated := strings.TrimSpace(
			pf.builder.String(),
		)
		accumulated = stripansi.Strip(accumulated)
		if accumulated != "" {
			if pf.checkFunction(accumulated) {
				pf.Found <- struct{}{}
				pf.builder.Reset()
			}
		}
		return n, err
	}
}

func (pf *PromptFinder) Stop() {
	close(pf.disabled)
}
