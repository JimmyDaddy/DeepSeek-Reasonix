package testutil

import (
	"context"
	"errors"
	"testing"

	"reasonix/internal/provider"
)

func TestMockProviderStreamHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mp := NewMock("mock", Turn{Text: "hello"})
	ch, err := mp.Stream(ctx, provider.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream error = %v, want context.Canceled", err)
	}
	if ch != nil {
		t.Fatal("Stream returned a channel for canceled context")
	}
	if got := mp.CallCount(); got != 0 {
		t.Fatalf("CallCount = %d, want 0", got)
	}
}

func TestMockProviderStreamStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mp := NewMock("mock", Turn{
		Text: "first",
		ToolCalls: []provider.ToolCall{
			{ID: "call-1", Name: "noop", Arguments: `{}`},
		},
	})

	ch, err := mp.Stream(ctx, provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got := (<-ch).Text; got != "first" {
		t.Fatalf("first chunk text = %q, want first", got)
	}
	cancel()
	sawCanceled := false
	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkError:
			if !errors.Is(chunk.Err, context.Canceled) {
				t.Fatalf("chunk after cancellation = %#v, want context.Canceled error", chunk)
			}
			sawCanceled = true
		case provider.ChunkDone:
			t.Fatalf("stream completed normally after cancellation: %#v", chunk)
		}
	}
	if !sawCanceled {
		t.Fatal("stream closed without returning cancellation error")
	}
}
