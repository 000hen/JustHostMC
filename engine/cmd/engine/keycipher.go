package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
)

// defaultCurseForgeKeyCipher is an optional build-time CurseForge API key,
// XOR-obfuscated with defaultCurseForgeKeyPad and hex-encoded so it survives
// PowerShell ldflags interpolation (CurseForge keys contain '$' and '/'). It is
// injected with: -ldflags "-X main.defaultCurseForgeKeyCipher=<hex>". The repo
// ships none; a user key set in Settings always wins over this default.
//
// NOTE: this is obfuscation, not encryption. Cipher and pad are BOTH injected at
// build time and neither is committed, so the repo alone cannot decode the key;
// but anyone with the built binary can (both values live in it). It keeps the
// plaintext key out of source, out of the binary's plaintext, and out of
// shell-mangling range.
var defaultCurseForgeKeyCipher string

// defaultCurseForgeKeyPad is the XOR pad, hex-encoded, injected alongside the
// cipher with -ldflags "-X main.defaultCurseForgeKeyPad=<hex>". build.ps1
// generates a fresh random pad per keyed build (or takes JHMC_KEY_CIPHER_PAD),
// so the pad is never committed and must match the pad used to produce the
// cipher. An empty pad (unkeyed dev build) yields no baked key.
var defaultCurseForgeKeyPad string

// decodeKeyCipher reverses the build-time obfuscation: hex-decode both the cipher
// and the pad, then XOR the cipher bytes with the pad (cycling the pad when the
// cipher is longer). It is a pure function of its inputs so it can be tested
// without touching the package-level ldflags vars. It returns an error for
// invalid hex in either argument or a zero-length pad; callers decide how to
// degrade.
func decodeKeyCipher(cipherHex, padHex string) (string, error) {
	cipher, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", fmt.Errorf("invalid cipher hex: %w", err)
	}
	pad, err := hex.DecodeString(padHex)
	if err != nil {
		return "", fmt.Errorf("invalid pad hex: %w", err)
	}
	if len(pad) == 0 {
		return "", errors.New("pad decodes to zero bytes")
	}
	out := make([]byte, len(cipher))
	for i := range cipher {
		out[i] = cipher[i] ^ pad[i%len(pad)]
	}
	return string(out), nil
}

// decodeDefaultCurseForgeKey decodes the ldflags-injected default CurseForge key.
// An empty cipher OR pad yields "" silently (the normal case for an unkeyed dev
// build). Invalid hex or a zero-length pad logs a warning and yields "" rather
// than crashing startup.
func decodeDefaultCurseForgeKey() string {
	if defaultCurseForgeKeyCipher == "" || defaultCurseForgeKeyPad == "" {
		return ""
	}
	key, err := decodeKeyCipher(defaultCurseForgeKeyCipher, defaultCurseForgeKeyPad)
	if err != nil {
		log.Printf("[WARN] baked CurseForge key: %v, ignoring", err)
		return ""
	}
	return key
}
