package main

import (
	"bufio"
	"io"
	"strings"
)

func scanSSELines(r io.Reader, onData func([]byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if onData != nil {
			if err := onData([]byte(payload)); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
