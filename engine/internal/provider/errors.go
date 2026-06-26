package provider

import "errors"

// Sentinel errors providers return. The gRPC layer maps these to a gRPC status
// plus an ErrorDetail{ErrorCode} so the frontend can localize them (PROMPT §4.1).
var (
	// ErrVersionNotFound means the requested version is not in the upstream list.
	ErrVersionNotFound = errors.New("version not found")
	// ErrChecksumMismatch means a downloaded artifact failed integrity checking.
	ErrChecksumMismatch = errors.New("downloaded file failed checksum verification")
	// ErrInstallerFailed means an installer (Forge/NeoForge) did not complete.
	ErrInstallerFailed = errors.New("server installer failed")
)
