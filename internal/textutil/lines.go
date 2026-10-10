package textutil

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// EachLine calls fn with every line of r, without its line ending, however
// long the line is. bufio.Scanner stops at 64KB by default and fails the
// whole read with "token too long"; a file TARS appends to (a transcript, a
// ledger, a JSONL store) has no such limit on what one record holds, so its
// readers use this instead. A last line with no newline is delivered like
// any other. Empty lines are delivered too — skipping them is the caller's
// choice. fn's error stops the read and is returned as is.
func EachLine(r io.Reader, fn func(line []byte) error) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 || err == nil {
			line = bytes.TrimSuffix(line, []byte("\n"))
			line = bytes.TrimSuffix(line, []byte("\r"))
			if fnErr := fn(line); fnErr != nil {
				return fnErr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
