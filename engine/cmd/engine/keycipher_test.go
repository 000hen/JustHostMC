package main

import (
	"encoding/hex"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testPad is an arbitrary 48-byte pad (hex-encoded) used to exercise the
// obfuscation round-trip without depending on any build-injected value.
const testPad = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f"

func repositoryFile(t *testing.T, path ...string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate keycipher_test.go")
	}
	parts := append([]string{filepath.Dir(thisFile), "..", "..", ".."}, path...)
	b, err := os.ReadFile(filepath.Clean(filepath.Join(parts...)))
	if err != nil {
		t.Fatalf("read repository file: %v", err)
	}
	return string(b)
}

// encodeForTest mirrors the build.ps1 Encode-CurseForgeKeyCipher helper: XOR the
// UTF-8 key bytes with the given hex pad (cycling) and lowercase-hex the result.
func encodeForTest(t *testing.T, key, padHex string) string {
	t.Helper()
	pad, err := hex.DecodeString(padHex)
	if err != nil {
		t.Fatalf("pad is not valid hex: %v", err)
	}
	kb := []byte(key)
	out := make([]byte, len(kb))
	for i := range kb {
		out[i] = kb[i] ^ pad[i%len(pad)]
	}
	return hex.EncodeToString(out)
}

func TestDecodeKeyCipher_RoundTrip(t *testing.T) {
	// A realistic bcrypt-style CurseForge key: contains '$', '/', and '.'.
	const key = "$2a$10$AbCdEf/GhIjKl.MnOpQrStUvWxYz0123456789"

	cipher := encodeForTest(t, key, testPad)
	got, err := decodeKeyCipher(cipher, testPad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != key {
		t.Fatalf("round-trip mismatch:\n got=%q\nwant=%q", got, key)
	}
}

func TestDecodeKeyCipher_PadShorterThanKey(t *testing.T) {
	// Force pad cycling: a key longer than the pad. Use a short 4-byte pad.
	const padHex = "deadbeef"
	key := ""
	for i := 0; i < 40; i++ {
		key += "x"
	}

	cipher := encodeForTest(t, key, padHex)
	got, err := decodeKeyCipher(cipher, padHex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != key {
		t.Fatalf("round-trip mismatch on long key:\n got=%q\nwant=%q", got, key)
	}
}

func TestDecodeKeyCipher_EmptyCipher(t *testing.T) {
	got, err := decodeKeyCipher("", testPad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("empty cipher: got %q, want \"\"", got)
	}
}

func TestDecodeKeyCipher_EmptyPad(t *testing.T) {
	// An empty pad string is not valid hex for a pad — it decodes to zero bytes.
	if _, err := decodeKeyCipher("abcd", ""); err == nil {
		t.Fatal("empty pad: expected error, got nil")
	}
}

func TestDecodeKeyCipher_InvalidCipherHex(t *testing.T) {
	for _, bad := range []string{"zzz", "abc", "$2a$10$notes"} {
		if _, err := decodeKeyCipher(bad, testPad); err == nil {
			t.Fatalf("invalid cipher hex %q: expected error, got nil", bad)
		}
	}
}

func TestDecodeKeyCipher_InvalidPadHex(t *testing.T) {
	for _, bad := range []string{"zzz", "abc", "nothex"} {
		if _, err := decodeKeyCipher("abcd", bad); err == nil {
			t.Fatalf("invalid pad hex %q: expected error, got nil", bad)
		}
	}
}

func TestDecodeKeyCipher_ZeroLengthPad(t *testing.T) {
	// "00" is a single zero byte (valid, length 1); a genuinely zero-length pad
	// comes from an empty hex string, which is valid hex decoding to 0 bytes.
	if _, err := decodeKeyCipher("abcd", ""); err == nil {
		t.Fatal("zero-length pad: expected error, got nil")
	}
}

func TestDecodeDefaultCurseForgeKey_EmptyVars(t *testing.T) {
	// Save and restore the ldflags-injected package vars.
	origCipher, origPad := defaultCurseForgeKeyCipher, defaultCurseForgeKeyPad
	defer func() {
		defaultCurseForgeKeyCipher = origCipher
		defaultCurseForgeKeyPad = origPad
	}()

	// Any combination with an empty cipher or pad must silently yield "".
	cases := []struct{ cipher, pad string }{
		{"", ""},
		{"", testPad},
		{encodeForTest(t, "anything", testPad), ""},
	}
	for _, c := range cases {
		defaultCurseForgeKeyCipher = c.cipher
		defaultCurseForgeKeyPad = c.pad
		if got := decodeDefaultCurseForgeKey(); got != "" {
			t.Fatalf("cipher=%q pad=%q: got %q, want \"\"", c.cipher, c.pad, got)
		}
	}
}

func TestDecodeDefaultCurseForgeKey_RoundTrip(t *testing.T) {
	const key = "$2a$10$AbCdEf/GhIjKl.MnOpQrStUvWxYz0123456789"

	origCipher, origPad := defaultCurseForgeKeyCipher, defaultCurseForgeKeyPad
	defer func() {
		defaultCurseForgeKeyCipher = origCipher
		defaultCurseForgeKeyPad = origPad
	}()

	defaultCurseForgeKeyCipher = encodeForTest(t, key, testPad)
	defaultCurseForgeKeyPad = testPad
	if got := decodeDefaultCurseForgeKey(); got != key {
		t.Fatalf("round-trip mismatch:\n got=%q\nwant=%q", got, key)
	}
}

func TestDecodeDefaultCurseForgeKey_InvalidHexDegrades(t *testing.T) {
	origCipher, origPad := defaultCurseForgeKeyCipher, defaultCurseForgeKeyPad
	defer func() {
		defaultCurseForgeKeyCipher = origCipher
		defaultCurseForgeKeyPad = origPad
	}()

	// Invalid cipher hex with a valid pad must degrade to "" (logged), not panic.
	defaultCurseForgeKeyCipher = "zzz"
	defaultCurseForgeKeyPad = testPad
	if got := decodeDefaultCurseForgeKey(); got != "" {
		t.Fatalf("invalid cipher hex: got %q, want \"\"", got)
	}

	// Valid cipher hex with invalid pad hex must also degrade to "".
	defaultCurseForgeKeyCipher = "abcd"
	defaultCurseForgeKeyPad = "zzz"
	if got := decodeDefaultCurseForgeKey(); got != "" {
		t.Fatalf("invalid pad hex: got %q, want \"\"", got)
	}
}

func TestBuildConfigurationDoesNotPersistCurseForgeKey(t *testing.T) {
	launchSettings := repositoryFile(t, "app", "JustHostMC.App", "Properties", "launchSettings.json")
	if strings.Contains(launchSettings, "JHMC_CURSEFORGE_API_KEY") {
		t.Fatal("launchSettings.json must not persist a CurseForge key; set it in the build environment")
	}
}

func TestMSBuildDoesNotEchoReversibleKeyMaterial(t *testing.T) {
	targets := repositoryFile(t, "app", "Engine.targets")
	if strings.Contains(targets, "_CurseForgeKeyLdflags") {
		t.Fatal("MSBuild must not store reversible key material in a property or binlog")
	}
	if !strings.Contains(targets, "-BuildEngine") {
		t.Fatal("MSBuild default builds must delegate key injection to the non-echoing helper")
	}
	decoder := xml.NewDecoder(strings.NewReader(targets))
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("parse Engine.targets: %v", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attrs := make(map[string]string, len(start.Attr))
		for _, attr := range start.Attr {
			attrs[attr.Name.Local] = attr.Value
		}
		if start.Name.Local == "Message" && strings.Contains(attrs["Text"], "EngineGoBuildArgs") {
			t.Fatal("MSBuild message must not print EngineGoBuildArgs containing the cipher and pad")
		}
		if start.Name.Local == "Exec" && strings.Contains(attrs["Command"], "go build $(EngineGoBuildArgs)") && attrs["EchoOff"] != "true" {
			t.Fatal("secret-bearing go build Exec must set EchoOff=true")
		}
	}
}

func TestBuildScriptRedactsEngineLdflags(t *testing.T) {
	buildScript := repositoryFile(t, "build.ps1")
	if !strings.Contains(buildScript, "-DisplayArguments") || !strings.Contains(buildScript, "<redacted>") {
		t.Fatal("build.ps1 must supply redacted display arguments for the key-bearing engine build")
	}
	if !strings.Contains(buildScript, "Remove-Item Env:JHMC_CURSEFORGE_API_KEY") {
		t.Fatal("build.ps1 must not pass the plaintext key to the child go process")
	}
}

func TestReleaseWorkflowUsesObfuscatedKeyFragment(t *testing.T) {
	workflow := repositoryFile(t, ".github", "workflows", "release.yml")
	if strings.Contains(workflow, "main.defaultCurseForgeKey=") {
		t.Fatal("release workflow must not inject the CurseForge key as plaintext")
	}
	if !strings.Contains(workflow, "Get-CurseForgeKeyLdflagsFragment") {
		t.Fatal("release workflow must use the shared XOR key helper")
	}
	if !strings.Contains(workflow, "Remove-Item Env:JHMC_CURSEFORGE_API_KEY") {
		t.Fatal("release workflow must not pass the plaintext key to the child go process")
	}
}
