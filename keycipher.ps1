<#
.SYNOPSIS
    Single source of truth for the optional baked-in CurseForge API key ldflags.

.DESCRIPTION
    Produces the Go `-ldflags` fragment that injects an XOR-obfuscated,
    hex-encoded CurseForge API key into the engine binary. Used by BOTH build
    paths so every build (build.ps1 AND the dotnet/VS/MSIX build via
    app/Engine.targets) bakes the same kind of key:

      - build.ps1 dot-sources this file and calls Get-CurseForgeKeyLdflagsFragment.
      - app/Engine.targets executes this file with -BuildEngine so reversible
        key material never enters an MSBuild property or binlog.

    When executed directly without -BuildEngine this script prints EXACTLY one
    line to stdout:
      * the fragment  -X main.defaultCurseForgeKeyCipher=<hex> -X main.defaultCurseForgeKeyPad=<hex>
        when $env:JHMC_CURSEFORGE_API_KEY is set, or
      * an empty line when it is not.
    On a validation failure it writes the reason to stderr and exits 1.

    The key is XOR-obfuscated with a pad and hex-encoded so it survives ldflags
    interpolation (CF keys contain '$' and '/') and does not sit plaintext in the
    binary. BOTH the cipher and the pad are injected via ldflags; neither is
    committed. The pad is generated fresh per build (48 cryptographically random
    bytes) unless JHMC_KEY_CIPHER_PAD supplies one (hex, >= 32 bytes) for a
    reproducible build. The engine reverses this in decodeDefaultCurseForgeKey
    (engine/cmd/engine/keycipher.go). This is obfuscation, not encryption: anyone
    with the built binary has both halves.

    COMPATIBILITY: must run under Windows PowerShell 5.1 AND pwsh 7 (MSBuild boxes
    may only have 5.1). Uses RandomNumberGenerator::Create().GetBytes() (not the
    PS7 static ::Fill) and avoids PS7-only syntax (no '&&', no ternary).
#>

[CmdletBinding()]
param(
    [switch]$BuildEngine,
    [string]$EngineSourceDir,
    [string]$EngineOutput
)

function Get-KeyCipherPadBytes {
    # Returns the XOR pad as a byte[]. Uses JHMC_KEY_CIPHER_PAD when set
    # (validated: hex only, even length, >= 32 bytes decoded; invalid -> throw),
    # otherwise generates 48 cryptographically random bytes fresh per call.
    if ($env:JHMC_KEY_CIPHER_PAD) {
        $hex = $env:JHMC_KEY_CIPHER_PAD
        if ($hex.Length % 2 -ne 0) {
            throw "JHMC_KEY_CIPHER_PAD is invalid: hex string must have an even length (got $($hex.Length) chars)."
        }
        if ($hex -notmatch '^[0-9a-fA-F]+$') {
            throw 'JHMC_KEY_CIPHER_PAD is invalid: must contain only hex characters [0-9a-fA-F].'
        }
        if (($hex.Length / 2) -lt 32) {
            throw "JHMC_KEY_CIPHER_PAD is invalid: pad must be at least 32 bytes (got $($hex.Length / 2))."
        }
        $bytes = for ($i = 0; $i -lt $hex.Length; $i += 2) {
            [Convert]::ToByte($hex.Substring($i, 2), 16)
        }
        return [byte[]]$bytes
    }
    $bytes = [byte[]]::new(48)
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    return $bytes
}

function ConvertTo-HexString {
    param([byte[]]$Bytes)
    $sb = [System.Text.StringBuilder]::new()
    foreach ($b in $Bytes) { [void]$sb.Append($b.ToString('x2')) }
    $sb.ToString()
}

function Encode-CurseForgeKeyCipher {
    param([string]$Key, [byte[]]$Pad)
    $keyBytes = [System.Text.Encoding]::UTF8.GetBytes($Key)
    $sb = [System.Text.StringBuilder]::new()
    for ($i = 0; $i -lt $keyBytes.Length; $i++) {
        $b = $keyBytes[$i] -bxor $Pad[$i % $Pad.Count]
        [void]$sb.Append($b.ToString('x2'))
    }
    $sb.ToString()
}

function Get-CurseForgeKeyLdflagsFragment {
    # Returns the ldflags fragment (two -X flags) when JHMC_CURSEFORGE_API_KEY is
    # set, or an empty string when it is not. Throws on an invalid
    # JHMC_KEY_CIPHER_PAD (via Get-KeyCipherPadBytes).
    if (-not $env:JHMC_CURSEFORGE_API_KEY) {
        return ''
    }
    $padBytes = Get-KeyCipherPadBytes
    $padHex = ConvertTo-HexString $padBytes
    $cipher = Encode-CurseForgeKeyCipher $env:JHMC_CURSEFORGE_API_KEY $padBytes
    return "-X main.defaultCurseForgeKeyCipher=$cipher -X main.defaultCurseForgeKeyPad=$padHex"
}

function Invoke-DefaultEngineBuild {
    param(
        [Parameter(Mandatory = $true)][string]$SourceDir,
        [Parameter(Mandatory = $true)][string]$OutputPath
    )

    $ldflags = '-s -w -buildid='
    $keyFragment = Get-CurseForgeKeyLdflagsFragment
    # The linker needs only the derived cipher/pad. Do not pass the plaintext
    # source secret through to the child go process environment.
    Remove-Item Env:JHMC_CURSEFORGE_API_KEY -ErrorAction SilentlyContinue
    if ($keyFragment) {
        $ldflags += " $keyFragment"
    }

    Push-Location $SourceDir
    try {
        & go build -trimpath -buildvcs=false -mod=readonly `
            "-ldflags=$ldflags" -o $OutputPath ./cmd/engine
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE."
        }
    } finally {
        Pop-Location
    }
}

# When executed directly (not dot-sourced), emit the fragment on one stdout line
# and translate a validation failure into stderr + exit 1. Dot-sourcing (build.ps1)
# sets InvocationName to '.' and skips this block, importing only the functions.
if ($MyInvocation.InvocationName -ne '.') {
    try {
        if ($BuildEngine) {
            if (-not $EngineSourceDir -or -not $EngineOutput) {
                throw '-BuildEngine requires -EngineSourceDir and -EngineOutput.'
            }
            Invoke-DefaultEngineBuild -SourceDir $EngineSourceDir `
                                      -OutputPath $EngineOutput
        } else {
            $fragment = Get-CurseForgeKeyLdflagsFragment
            [Console]::Out.WriteLine($fragment)
        }
    } catch {
        [Console]::Error.WriteLine($_.Exception.Message)
        exit 1
    }
}
