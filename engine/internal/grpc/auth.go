// Package grpcsvc implements the engine's gRPC server: session-token
// authentication and the service handlers exposed to the WinUI frontend.
package grpcsvc

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// tokenMetadataKey is the gRPC metadata header carrying the per-launch session
// token. The app generates a random token, passes it to the engine at launch,
// and attaches it to every call so other local processes cannot hijack the
// loopback channel.
const tokenMetadataKey = "x-mcmanager-token"

// tokenFromContext extracts the session token from incoming call metadata.
func tokenFromContext(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	values := md.Get(tokenMetadataKey)
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

// tokenMatches compares tokens in constant time to avoid leaking the expected
// value through timing differences.
func tokenMatches(expected, got string) bool {
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}

// NewUnaryAuthInterceptor rejects unary calls whose metadata does not carry the
// expected session token with codes.Unauthenticated.
func NewUnaryAuthInterceptor(expected string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		got, ok := tokenFromContext(ctx)
		if !ok || !tokenMatches(expected, got) {
			return nil, status.Error(codes.Unauthenticated, "invalid or missing session token")
		}
		return handler(ctx, req)
	}
}

// NewStreamAuthInterceptor is the streaming counterpart of
// NewUnaryAuthInterceptor.
func NewStreamAuthInterceptor(expected string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		got, ok := tokenFromContext(ss.Context())
		if !ok || !tokenMatches(expected, got) {
			return status.Error(codes.Unauthenticated, "invalid or missing session token")
		}
		return handler(srv, ss)
	}
}
