package grpcsvc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// incomingTokenCtx builds a context carrying the session token the way an
// incoming gRPC call would, so interceptors can be unit-tested in isolation.
func incomingTokenCtx(token string) context.Context {
	md := metadata.New(map[string]string{tokenMetadataKey: token})
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestUnaryAuthInterceptor(t *testing.T) {
	const expected = "s3cr3t"
	interceptor := NewUnaryAuthInterceptor(expected)
	info := &grpc.UnaryServerInfo{FullMethod: "/mcmanager.v1.EngineService/Health"}

	tests := []struct {
		name      string
		ctx       context.Context
		wantCode  codes.Code
		wantCalls bool
	}{
		{"valid token", incomingTokenCtx(expected), codes.OK, true},
		{"missing metadata", context.Background(), codes.Unauthenticated, false},
		{"wrong token", incomingTokenCtx("nope"), codes.Unauthenticated, false},
		{"empty token", incomingTokenCtx(""), codes.Unauthenticated, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := func(ctx context.Context, req any) (any, error) {
				called = true
				return "ok", nil
			}

			_, err := interceptor(tt.ctx, nil, info, handler)

			if got := status.Code(err); got != tt.wantCode {
				t.Errorf("status code = %v, want %v (err = %v)", got, tt.wantCode, err)
			}
			if called != tt.wantCalls {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalls)
			}
		})
	}
}

// fakeServerStream lets us inject a context into the stream interceptor.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeServerStream) Context() context.Context { return f.ctx }

func TestStreamAuthInterceptor(t *testing.T) {
	const expected = "s3cr3t"
	interceptor := NewStreamAuthInterceptor(expected)
	info := &grpc.StreamServerInfo{FullMethod: "/mcmanager.v1.ConsoleService/Attach"}

	tests := []struct {
		name      string
		ctx       context.Context
		wantCode  codes.Code
		wantCalls bool
	}{
		{"valid token", incomingTokenCtx(expected), codes.OK, true},
		{"missing metadata", context.Background(), codes.Unauthenticated, false},
		{"wrong token", incomingTokenCtx("nope"), codes.Unauthenticated, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := func(srv any, ss grpc.ServerStream) error {
				called = true
				return nil
			}

			err := interceptor(nil, fakeServerStream{ctx: tt.ctx}, info, handler)

			if got := status.Code(err); got != tt.wantCode {
				t.Errorf("status code = %v, want %v (err = %v)", got, tt.wantCode, err)
			}
			if called != tt.wantCalls {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalls)
			}
		})
	}
}
