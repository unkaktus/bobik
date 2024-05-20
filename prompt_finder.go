package main

import (
	"io"
	"strings"

	"github.com/acarl005/stripansi"
)

type Prompt struct {
	Type string
}

type PromptFinder struct {
	reader        io.Reader
	checkFunction func(string) (bool, string)
	builder       *strings.Builder
	teeReader     io.Reader
	Found         chan Prompt
	disabled      chan struct{}
}

func NewPromptFinder(r io.Reader, check func(string) (bool, string)) *PromptFinder {
	builder := &strings.Builder{}
	return &PromptFinder{
		reader:        r,
		checkFunction: check,
		builder:       builder,
		teeReader:     io.TeeReader(r, builder),
		Found:         make(chan Prompt),
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
			promptPresent, promptType := pf.checkFunction(accumulated)
			if promptPresent {
				prompt := Prompt{
					Type: promptType,
				}
				pf.Found <- prompt
				pf.builder.Reset()
			}
			// Check whether we already got the shell
			if strings.HasSuffix(accumulated, "$") || strings.HasSuffix(accumulated, "#") || strings.HasSuffix(accumulated, ">") {
				// Check if two characters are not the same
				if accumulated[len(accumulated)-1] != accumulated[len(accumulated)-2] {
					pf.Stop()
					pf.builder.Reset()
				}
			}
		}
		return n, err
	}
}

func (pf *PromptFinder) Stop() {
	close(pf.disabled)
	close(pf.Found)
}
