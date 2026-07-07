package grpcsvc

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryRPCLoggingIncludesSuccessfulLifecycle(t *testing.T) {
	var output bytes.Buffer
	interceptor := logUnaryRPCs(log.New(&output, "", 0))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Healthy"}

	response, err := interceptor(
		context.Background(),
		struct{}{},
		info,
		func(context.Context, any) (any, error) { return "ok", nil },
	)
	if err != nil || response != "ok" {
		t.Fatalf("interceptor result = (%v, %v), want (ok, nil)", response, err)
	}

	logs := output.String()
	for _, want := range []string{
		"[DEBUG] grpc unary started method=/test.Service/Healthy",
		"[INFO] grpc unary completed method=/test.Service/Healthy code=OK duration=",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs do not contain %q:\n%s", want, logs)
		}
	}
}

func TestUnaryRPCLoggingIncludesStatusErrorFromGoHandler(t *testing.T) {
	var output bytes.Buffer
	interceptor := logUnaryRPCs(log.New(&output, "", 0))
	wantErr := status.Error(codes.Internal, "registry write failed")

	_, gotErr := interceptor(
		context.Background(),
		struct{}{},
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/Save"},
		func(context.Context, any) (any, error) { return nil, wantErr },
	)
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("interceptor error = %v, want %v", gotErr, wantErr)
	}

	logs := output.String()
	for _, want := range []string{
		"[ERROR] grpc unary failed method=/test.Service/Save",
		"code=Internal",
		`error="registry write failed"`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs do not contain %q:\n%s", want, logs)
		}
	}
}

func TestRPCLoggingClassifiesExpectedStatusAsWarning(t *testing.T) {
	var output bytes.Buffer
	interceptor := logUnaryRPCs(log.New(&output, "", 0))

	_, _ = interceptor(
		context.Background(),
		struct{}{},
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/Lookup"},
		func(context.Context, any) (any, error) {
			return nil, status.Error(codes.NotFound, "missing")
		},
	)

	if logs := output.String(); !strings.Contains(logs, "[WARN] grpc unary failed") {
		t.Fatalf("logs do not classify NotFound as warning:\n%s", logs)
	}
}

func TestStreamRPCLoggingIncludesStatusErrorFromGoHandler(t *testing.T) {
	var output bytes.Buffer
	interceptor := logStreamRPCs(log.New(&output, "", 0))
	wantErr := status.Error(codes.Unavailable, "console transport closed")

	gotErr := interceptor(
		nil,
		nil,
		&grpc.StreamServerInfo{
			FullMethod:     "/test.Service/Attach",
			IsClientStream: true,
			IsServerStream: true,
		},
		func(any, grpc.ServerStream) error { return wantErr },
	)
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("interceptor error = %v, want %v", gotErr, wantErr)
	}

	logs := output.String()
	for _, want := range []string{
		"[DEBUG] grpc stream started method=/test.Service/Attach client_stream=true server_stream=true",
		"[ERROR] grpc stream failed method=/test.Service/Attach",
		"code=Unavailable",
		`error="console transport closed"`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs do not contain %q:\n%s", want, logs)
		}
	}
}
