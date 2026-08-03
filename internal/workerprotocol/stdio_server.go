package workerprotocol

import (
	"context"
	"fmt"
	"io"
)

func ServeJSONL(ctx context.Context, input io.Reader, output io.Writer, handler WireHandler, limits WireLimits) error {
	if input == nil || output == nil || handler == nil {
		return ErrTransportConfig
	}
	if limits == (WireLimits{}) {
		limits = DefaultWireLimits()
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(input, limits.MaxRequestBytes+1))
	if err != nil {
		return fmt.Errorf("workerprotocol: read worker wire frame: %w", err)
	}
	if int64(len(raw)) > limits.MaxRequestBytes {
		return ErrTransportLimit
	}
	request, err := decodeWireRequest(raw, limits)
	if err != nil {
		return err
	}
	response, err := handler.Handle(ctx, request)
	if err != nil {
		return err
	}
	encoded, err := encodeWireResponse(response, request.RequestID, limits)
	if err != nil {
		return err
	}
	for len(encoded) > 0 {
		written, writeErr := output.Write(encoded)
		if writeErr != nil {
			return fmt.Errorf("workerprotocol: write worker wire frame: %w", writeErr)
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		encoded = encoded[written:]
	}
	return nil
}
