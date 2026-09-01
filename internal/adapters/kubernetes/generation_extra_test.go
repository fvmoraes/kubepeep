package kubernetes

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGenerationNilGuardsAndIdentity(t *testing.T) {
	if id := (*Generation)(nil).ID(); id != 0 {
		t.Fatalf("nil generation id = %d", id)
	}
	if (*Generation)(nil).Context() == nil {
		t.Fatal("nil generation context is nil")
	}
	(*Generation)(nil).cancelWith(context.Canceled)
	parent := context.Background()
	generation := newGeneration(parent, 7, time.Second)
	if generation.ID() != 7 {
		t.Fatalf("generation id = %d", generation.ID())
	}
	if generation.Context() != generation.ctx {
		t.Fatal("generation context is not the derived context")
	}
	generation.cancelWith(context.Canceled)
	if err := generation.Context().Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("generation after close = %v", err)
	}
}

func TestGenerationUnaryRejectsInvalidInputs(t *testing.T) {
	generation := newGeneration(context.Background(), 1, 0)
	if _, _, err := generation.Unary(context.Background()); err == nil {
		t.Fatal("zero timeout unary was accepted")
	}
	if _, _, err := (*Generation)(nil).Unary(context.Background()); err == nil {
		t.Fatal("nil generation unary was accepted")
	}
	unbounded := newGeneration(context.Background(), 1, time.Second)
	if _, _, err := unbounded.Unary(nil); err == nil {
		t.Fatal("nil parent unary was accepted")
	}
}

func TestGenerationUnaryInheritsParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()
	generation := newGeneration(context.Background(), 1, time.Second)
	unary, cancelUnary, err := generation.Unary(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelUnary()
	select {
	case <-unary.Done():
	case <-time.After(time.Second):
		t.Fatal("canceled parent did not cancel the unary context")
	}
	if cause := context.Cause(unary); !errors.Is(cause, context.Canceled) {
		t.Fatalf("unary cause = %v", cause)
	}
}

func TestGenerationStreamRejectsInvalidInputs(t *testing.T) {
	generation := newGeneration(context.Background(), 1, time.Second)
	if _, err := (*Generation)(nil).Stream(context.Background(), time.Second); err == nil {
		t.Fatal("nil generation stream was accepted")
	}
	if _, err := generation.Stream(nil, time.Second); err == nil {
		t.Fatal("nil parent stream was accepted")
	}
	if _, err := generation.Stream(context.Background(), 0); err == nil {
		t.Fatal("zero idle timeout stream was accepted")
	}
}

func TestStreamContextIdleCloseCarriesTimeoutCause(t *testing.T) {
	generation := newGeneration(context.Background(), 1, time.Second)
	stream, err := generation.Stream(context.Background(), 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Activity() {
		t.Fatal("fresh stream reported no activity")
	}
	select {
	case <-stream.Context().Done():
	case <-time.After(2 * time.Second):
		t.Fatal("idle stream was not closed")
	}
	if cause := context.Cause(stream.Context()); !errors.Is(cause, errStreamIdle) {
		t.Fatalf("stream cause = %v", cause)
	}
	if stream.Activity() {
		t.Fatal("idle-closed stream reported activity")
	}
	stream.Close()
	stream.Close()
}

func TestStreamContextNilAndDoubleCloseGuards(t *testing.T) {
	if (*StreamContext)(nil).Context() == nil {
		t.Fatal("nil stream context is nil")
	}
	if (*StreamContext)(nil).Activity() {
		t.Fatal("nil stream reported activity")
	}
	(*StreamContext)(nil).Close()
	generation := newGeneration(context.Background(), 1, time.Second)
	stream, err := generation.Stream(context.Background(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	if stream.Activity() {
		t.Fatal("closed stream reported activity")
	}
	select {
	case <-stream.Context().Done():
	default:
		t.Fatal("closed stream context is still alive")
	}
}
