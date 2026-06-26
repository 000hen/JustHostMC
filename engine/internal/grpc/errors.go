package grpcsvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/provider"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errorStatus builds a gRPC error carrying an ErrorDetail in the status details
// so the frontend can map ErrorCode to a localized string (PROMPT §4.1, §14).
// The status message itself is diagnostic only and never shown to users.
func errorStatus(code codes.Code, ec mcmanagerv1.ErrorCode, msg string, meta map[string]string) error {
	st := status.New(code, msg)
	if withDetails, err := st.WithDetails(&mcmanagerv1.ErrorDetail{Code: ec, Metadata: meta}); err == nil {
		return withDetails.Err()
	}
	return st.Err()
}

// mapInstallError converts provider/JRE failures into typed gRPC statuses.
func mapInstallError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return status.FromContextError(context.Canceled).Err()
	case errors.Is(err, provider.ErrVersionNotFound):
		return errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_VERSION_NOT_FOUND, err.Error(), nil)
	case errors.Is(err, jre.ErrJREUnavailable):
		return errorStatus(codes.Unavailable, mcmanagerv1.ErrorCode_JRE_DOWNLOAD_FAILED, err.Error(), nil)
	case errors.Is(err, provider.ErrChecksumMismatch):
		return errorStatus(codes.Internal, mcmanagerv1.ErrorCode_INSTALL_FAILED, err.Error(), nil)
	default:
		return errorStatus(codes.Internal, mcmanagerv1.ErrorCode_INSTALL_FAILED, err.Error(), nil)
	}
}

// genID returns a short random hex id for a new server.
func genID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
