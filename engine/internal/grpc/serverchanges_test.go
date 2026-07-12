package grpcsvc

import (
	"context"
	"io"
	"testing"
	"time"

	mb "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestWatchChangesStreamsReadyUpsertAndDelete(t *testing.T) {
	observable := store.NewObservable(store.NewMemory(), 4)
	service := NewServerService(ServerServiceConfig{Store: observable, Changes: observable})
	stream := newServerChangeTestStream(t.Context())
	done := make(chan error, 1)
	go func() { done <- service.WatchChanges(&mb.Empty{}, stream) }()

	if ready := receiveServerChange(t, stream.sent); ready.GetReady() == nil {
		t.Fatalf("first event = %#v, want ready", ready)
	}
	if err := observable.Put(&store.Server{ID: "one", Name: "First"}); err != nil {
		t.Fatal(err)
	}
	if upsert := receiveServerChange(t, stream.sent).GetUpsert(); upsert == nil || upsert.Id != "one" {
		t.Fatalf("upsert = %#v", upsert)
	}
	if err := observable.Delete("one"); err != nil {
		t.Fatal(err)
	}
	if deleted := receiveServerChange(t, stream.sent).GetDeleted(); deleted == nil || deleted.Id != "one" {
		t.Fatalf("deleted = %#v", deleted)
	}

	stream.cancel()
	if err := <-done; !isCanceled(err) {
		t.Fatalf("WatchChanges error = %v, want cancellation", err)
	}
}

func TestWatchChangesCancellationStopsHandler(t *testing.T) {
	observable := store.NewObservable(store.NewMemory(), 1)
	service := NewServerService(ServerServiceConfig{Store: observable, Changes: observable})
	stream := newServerChangeTestStream(t.Context())
	done := make(chan error, 1)
	go func() { done <- service.WatchChanges(&mb.Empty{}, stream) }()
	_ = receiveServerChange(t, stream.sent)

	stream.cancel()
	select {
	case err := <-done:
		if !isCanceled(err) {
			t.Fatalf("WatchChanges error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WatchChanges did not stop after cancellation")
	}
}

func TestWatchChangesReturnsResourceExhaustedForSlowSubscriber(t *testing.T) {
	observable := store.NewObservable(store.NewMemory(), 1)
	service := NewServerService(ServerServiceConfig{Store: observable, Changes: observable})
	stream := newServerChangeTestStream(t.Context())
	done := make(chan error, 1)
	go func() { done <- service.WatchChanges(&mb.Empty{}, stream) }()
	_ = receiveServerChange(t, stream.sent)
	_ = receiveServerChange(t, stream.sendStarted)

	if err := observable.Put(&store.Server{ID: "one"}); err != nil {
		t.Fatal(err)
	}
	started := receiveServerChange(t, stream.sendStarted)
	if started.GetUpsert().GetId() != "one" {
		t.Fatalf("blocked send = %#v", started)
	}
	if err := observable.Put(&store.Server{ID: "two"}); err != nil {
		t.Fatal(err)
	}
	if err := observable.Put(&store.Server{ID: "three"}); err != nil {
		t.Fatal(err)
	}

	_ = receiveServerChange(t, stream.sent)
	_ = receiveServerChange(t, stream.sent)
	select {
	case err := <-done:
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("WatchChanges code = %v, want ResourceExhausted", status.Code(err))
		}
	case <-time.After(time.Second):
		t.Fatal("WatchChanges did not stop after subscriber overflow")
	}
}

func receiveServerChange(t *testing.T, events <-chan *mb.ServerChangeEvent) *mb.ServerChangeEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server change")
		return nil
	}
}

func isCanceled(err error) bool {
	return err == context.Canceled || status.Code(err) == codes.Canceled
}

type serverChangeTestStream struct {
	ctx         context.Context
	cancel      context.CancelFunc
	sent        chan *mb.ServerChangeEvent
	sendStarted chan *mb.ServerChangeEvent
}

func newServerChangeTestStream(parent context.Context) *serverChangeTestStream {
	ctx, cancel := context.WithCancel(parent)
	return &serverChangeTestStream{
		ctx:         ctx,
		cancel:      cancel,
		sent:        make(chan *mb.ServerChangeEvent),
		sendStarted: make(chan *mb.ServerChangeEvent, 8),
	}
}

func (s *serverChangeTestStream) Send(event *mb.ServerChangeEvent) error {
	s.sendStarted <- event
	select {
	case s.sent <- event:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *serverChangeTestStream) SetHeader(metadata.MD) error  { return nil }
func (s *serverChangeTestStream) SendHeader(metadata.MD) error { return nil }
func (s *serverChangeTestStream) SetTrailer(metadata.MD)       {}
func (s *serverChangeTestStream) Context() context.Context     { return s.ctx }
func (s *serverChangeTestStream) SendMsg(message any) error {
	event, ok := message.(*mb.ServerChangeEvent)
	if !ok {
		return status.Error(codes.Internal, "unexpected stream message")
	}
	return s.Send(event)
}
func (s *serverChangeTestStream) RecvMsg(any) error { return io.EOF }
