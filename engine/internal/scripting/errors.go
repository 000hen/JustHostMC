package scripting

import "errors"

// Sentinel errors the scripting layer raises. The gRPC layer maps these (and the
// provider sentinels they may wrap) to localized error codes.
var (
	// ErrPermissionDenied means a script called a host function whose permission
	// the user has not granted.
	ErrPermissionDenied = errors.New("permission not granted")
	// ErrPathEscape means a script tried to touch a path outside the server dir.
	ErrPathEscape = errors.New("path escapes the server directory")
	// ErrScriptInvalid means a script is missing required metadata or functions.
	ErrScriptInvalid = errors.New("invalid script")
)
