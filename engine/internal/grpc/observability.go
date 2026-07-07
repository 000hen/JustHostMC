package grpcsvc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// logUnaryRPCs records both sides of every unary call. This deliberately logs
// method metadata rather than request values: the latter can contain API keys,
// console commands, and other user data that must not leak into diagnostics.
func logUnaryRPCs(logger *log.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		started := time.Now()
		logger.Printf("[DEBUG] grpc unary started method=%s", info.FullMethod)

		response, err := handler(ctx, req)
		logRPCCompletion(logger, "unary", info.FullMethod, started, err)
		return response, err
	}
}

// logStreamRPCs provides the same engine-side visibility for server, client,
// and bidirectional streams. Completion is emitted when the stream handler
// exits, including cancellation and transport errors.
func logStreamRPCs(logger *log.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		started := time.Now()
		logger.Printf(
			"[DEBUG] grpc stream started method=%s client_stream=%t server_stream=%t",
			info.FullMethod,
			info.IsClientStream,
			info.IsServerStream,
		)

		err := handler(srv, stream)
		logRPCCompletion(logger, "stream", info.FullMethod, started, err)
		return err
	}
}

func logRPCCompletion(logger *log.Logger, kind, method string, started time.Time, err error) {
	duration := time.Since(started).Round(time.Microsecond)
	if err == nil {
		logger.Printf(
			"[INFO] grpc %s completed method=%s code=%s duration=%s",
			kind,
			method,
			codes.OK,
			duration,
		)
		return
	}

	st := status.Convert(err)
	logger.Printf(
		"[%s] grpc %s failed method=%s code=%s duration=%s error=%q",
		grpcErrorSeverity(st.Code()),
		kind,
		method,
		st.Code(),
		duration,
		st.Message(),
	)
}

// Client mistakes and lifecycle cancellations are warnings; engine, transport,
// capacity, and data-integrity failures are errors that deserve attention.
func grpcErrorSeverity(code codes.Code) string {
	switch code {
	case codes.Canceled,
		codes.InvalidArgument,
		codes.NotFound,
		codes.AlreadyExists,
		codes.PermissionDenied,
		codes.FailedPrecondition,
		codes.Aborted,
		codes.OutOfRange,
		codes.Unauthenticated:
		return "WARN"
	default:
		return "ERROR"
	}
}
