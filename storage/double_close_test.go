package storage

import (
	"context"
	"fmt"
	"testing"
)

func TestWriterDoubleClose(t *testing.T) {
	ctx := context.Background()
	w := &Writer{ctx: ctx, donec: make(chan struct{})}
	w.closed = true // Simulate it being closed
	w.err = fmt.Errorf("some previous error")

	err := w.Close()
	if err == nil || err.Error() != "some previous error" {
		t.Fatalf("expected some previous error, got: %v", err)
	}
}

func TestPCUWriterWriteAfterClose(t *testing.T) {
	ctx := context.Background()
	w := &Writer{ctx: ctx, donec: make(chan struct{})}
	w.closed = true // Simulate it being closed

	_, err := w.Write([]byte("hello"))
	if err == nil || err.Error() != "storage: Writer is closed" {
		t.Fatalf("expected Writer is closed error, got: %v", err)
	}
}
