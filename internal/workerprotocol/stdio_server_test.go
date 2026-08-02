package workerprotocol

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestServeJSONLProcessesExactlyOneBoundedWireFrame(t *testing.T) {
	t.Parallel()

	request := testWireRequest("worker-a", "placement-a", MessageHeartbeat, HeartbeatPayload{})
	limits := WireLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096}
	raw, err := encodeWireRequest(request, limits)
	if err != nil {
		t.Fatal(err)
	}
	handler := &recordingWireHandler{}
	var output bytes.Buffer
	if err := ServeJSONL(context.Background(), bytes.NewReader(raw), &output, handler, limits); err != nil {
		t.Fatalf("serve JSONL: %v", err)
	}
	response, err := decodeWireResponse(output.Bytes(), request.RequestID, limits)
	if err != nil {
		t.Fatalf("decode served response: %v", err)
	}
	if !response.Accepted || handler.request.RequestID != request.RequestID {
		t.Fatalf("response=%+v request=%+v", response, handler.request)
	}

	if err := ServeJSONL(context.Background(), bytes.NewReader(append(raw, raw...)), &bytes.Buffer{}, handler, limits); !errors.Is(err, ErrWireContract) {
		t.Fatalf("multiple-frame error=%v want ErrWireContract", err)
	}
	oversized := bytes.Repeat([]byte("x"), int(limits.MaxRequestBytes)+1)
	if err := ServeJSONL(context.Background(), bytes.NewReader(oversized), &bytes.Buffer{}, handler, limits); !errors.Is(err, ErrTransportLimit) {
		t.Fatalf("oversized-frame error=%v want ErrTransportLimit", err)
	}
}
