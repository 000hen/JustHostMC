<#
.SYNOPSIS
    JustHostMC - Full project build.
    Runs every step needed to produce a working build from a fresh clone.

.DESCRIPTION
    .\build.ps1                         # default: Debug, x64, all steps
    .\build.ps1 -Configuration Release  # release build
    .\build.ps1 -Platform ARM64         # target ARM64
    .\build.ps1 -SkipTests              # skip go test + dotnet test
    .\build.ps1 -SkipEngine             # skip Go build (reuse existing engine.exe)
    .\build.ps1 -SkipProto              # skip buf generate (reuse existing stubs)

.PARAMETER Configuration
    Build configuration: Debug (default) or Release.

.PARAMETER Platform
    Target platform: x64 (default), x86, or ARM64.

.PARAMETER SkipTests
    Skip both Go and .NET test steps.

.PARAMETER SkipEngine
    Skip Go engine compilation (assumes build/engine.exe already exists).

.PARAMETER SkipProto
    Skip protobuf code generation (assumes engine/gen/ already exists).
#>
[CmdletBinding()]
param(
    [ValidateSet('Debug','Release')]
    [string]$Configuration = 'Debug',

    [ValidateSet('x64','x86','ARM64')]
    [string]$Platform = 'x64',

    [switch]$SkipTests,
    [switch]$SkipEngine,
    [switch]$SkipProto
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = $PSScriptRoot
$ProtoDir = Join-Path $RepoRoot 'proto'
$EngineDir = Join-Path $RepoRoot 'engine'
$BuildDir = Join-Path $RepoRoot 'build'
$AppCsproj = Join-Path (Join-Path (Join-Path $RepoRoot 'app') 'JustHostMC.App') 'JustHostMC.App.csproj'
$TestCsproj = Join-Path (Join-Path (Join-Path $RepoRoot 'app') 'JustHostMC.Core.Tests') 'JustHostMC.Core.Tests.csproj'
$EngineExe = Join-Path $BuildDir 'engine.exe'

$script:stepNum = 0
$script:totalTime = [System.Diagnostics.Stopwatch]::StartNew()

# -- Helpers -------------------------------------------------------------------

function Invoke-Step {
    param(
        [string]$Name,
        [scriptblock]$Action
    )
    $script:stepNum++
    Write-Host ''
    Write-Host "=== Step $($script:stepNum): $Name " -ForegroundColor Cyan
    $sw = [System.Diagnostics.Stopwatch]::StartNew()

    try {
        & $Action
        $sw.Stop()
        Write-Host "  [OK] $Name completed ($([math]::Round($sw.Elapsed.TotalSeconds, 1))s)" -ForegroundColor Green
    } catch {
        $sw.Stop()
        Write-Host "  [FAIL] $Name FAILED ($([math]::Round($sw.Elapsed.TotalSeconds, 1))s)" -ForegroundColor Red
        Write-Host "    $($_.Exception.Message)" -ForegroundColor Red
        Write-Host ''
        Write-Host 'Build failed. Fix the error above and re-run.' -ForegroundColor Red
        exit 1
    }
}

function Invoke-External {
    param(
        [string]$Description,
        [string]$Command,
        [string[]]$Arguments,
        [string[]]$DisplayArguments,
        [string]$WorkingDirectory = $RepoRoot,
        [hashtable]$Environment = @{}
    )

    if (-not $PSBoundParameters.ContainsKey('DisplayArguments')) {
        $DisplayArguments = $Arguments
    }
    Write-Host "  > $Command $($DisplayArguments -join ' ')" -ForegroundColor DarkGray

    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $Command
    $psi.Arguments = $Arguments -join ' '
    $psi.WorkingDirectory = $WorkingDirectory
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true

    foreach ($kv in $Environment.GetEnumerator()) {
        $psi.EnvironmentVariables[$kv.Key] = $kv.Value
    }

    $proc = [System.Diagnostics.Process]::Start($psi)
    $stdout = $proc.StandardOutput.ReadToEnd()
    $stderr = $proc.StandardError.ReadToEnd()
    $proc.WaitForExit()

    if ($stdout) { Write-Host $stdout }
    if ($stderr) { Write-Host $stderr -ForegroundColor DarkYellow }

    if ($proc.ExitCode -ne 0) {
        throw "$Description failed with exit code $($proc.ExitCode)."
    }
}

# -- Banner --------------------------------------------------------------------

Write-Host ''
Write-Host '====================================================' -ForegroundColor Cyan
Write-Host '            JustHostMC - Full Build' -ForegroundColor Cyan
Write-Host '====================================================' -ForegroundColor Cyan
Write-Host "  Configuration : $Configuration" -ForegroundColor White
Write-Host "  Platform      : $Platform" -ForegroundColor White
Write-Host "  SkipTests     : $SkipTests" -ForegroundColor White
Write-Host "  SkipEngine    : $SkipEngine" -ForegroundColor White
Write-Host "  SkipProto     : $SkipProto" -ForegroundColor White

# -- Step 1: Generate Go gRPC stubs -------------------------------------------

if (-not $SkipProto) {
    Invoke-Step 'Generate Go gRPC stubs (buf generate)' {
        Invoke-External `
            -Description 'buf generate' `
            -Command 'buf' `
            -Arguments @('generate') `
            -WorkingDirectory $ProtoDir
    }
} else {
    Write-Host ''
    Write-Host '  >> Skipping protobuf generation (-SkipProto)' -ForegroundColor Yellow
}

# -- Step 2: Build Go engine --------------------------------------------------

if (-not $SkipEngine) {
    Invoke-Step 'Build Go engine (go build)' {
        # Ensure build output directory exists
        if (-not (Test-Path $BuildDir)) {
            New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null
        }

        Invoke-External `
            -Description 'go build (verify)' `
            -Command 'go' `
            -Arguments @('build', './...') `
            -WorkingDirectory $EngineDir

        # Optional baked-in CurseForge API key (never committed): set
        # JHMC_CURSEFORGE_API_KEY in the environment before building. The XOR/pad
        # obfuscation and the ldflags fragment are produced by the shared
        # keycipher.ps1 (the single source of truth used by every build path,
        # including the dotnet/VS/MSIX build via app/Engine.targets). Dot-sourcing
        # imports its functions without running its stdout/exit main block; an
        # invalid JHMC_KEY_CIPHER_PAD throws and is surfaced by Invoke-Step. See
        # keycipher.ps1 and engine/cmd/engine/keycipher.go for the full contract.
        . (Join-Path $RepoRoot 'keycipher.ps1')

        $ldflags = '-s -w -buildid='
        $keyFragment = Get-CurseForgeKeyLdflagsFragment
        Remove-Item Env:JHMC_CURSEFORGE_API_KEY -ErrorAction SilentlyContinue
        if ($keyFragment) {
            $ldflags += " $keyFragment"
        }

        Invoke-External `
            -Description 'go build (engine.exe)' `
            -Command 'go' `
            -Arguments @('build', '-trimpath', '-buildvcs=false', '-mod=readonly',
                         "-ldflags=`"$ldflags`"",
                         '-o', "`"$EngineExe`"", './cmd/engine') `
            -DisplayArguments @('build', '-trimpath', '-buildvcs=false', '-mod=readonly',
                                '-ldflags="<redacted>"',
                                '-o', "`"$EngineExe`"", './cmd/engine') `
            -WorkingDirectory $EngineDir `
            -Environment @{ CGO_ENABLED = '0' }
    }

    if (-not $SkipTests) {
        Invoke-Step 'Test Go engine (go test)' {
            Invoke-External `
                -Description 'go test' `
                -Command 'go' `
                -Arguments @('test', './...') `
                -WorkingDirectory $EngineDir
        }
    }
} else {
    Write-Host ''
    Write-Host '  >> Skipping Go engine build (-SkipEngine)' -ForegroundColor Yellow

    if (-not (Test-Path $EngineExe)) {
        Write-Host '  [WARN] build/engine.exe does not exist!' -ForegroundColor Red
        Write-Host '    The C# build will fail. Remove -SkipEngine or provide a prebuilt engine.' -ForegroundColor Red
    }
}

# -- Step 3: Restore .NET packages --------------------------------------------

Invoke-Step 'Restore .NET packages (dotnet restore)' {
    Invoke-External `
        -Description 'dotnet restore' `
        -Command 'dotnet' `
        -Arguments @('restore', $AppCsproj, "-p:Platform=$Platform") `
        -WorkingDirectory $RepoRoot
}

# -- Step 4: Build C# app -----------------------------------------------------

Invoke-Step "Build C# app (dotnet build - $Configuration|$Platform)" {
    Invoke-External `
        -Description 'dotnet build' `
        -Command 'dotnet' `
        -Arguments @('build', $AppCsproj,
                     '--configuration', $Configuration,
                     "-p:Platform=$Platform",
                     '-p:SkipEngineBuild=true',
                     '--no-restore') `
        -WorkingDirectory $RepoRoot
}

# -- Step 5: Run C# tests -----------------------------------------------------

if (-not $SkipTests) {
    Invoke-Step 'Run C# tests (dotnet test)' {
        Invoke-External `
            -Description 'dotnet test' `
            -Command 'dotnet' `
            -Arguments @('test', $TestCsproj,
                         '-p:SkipEngineBuild=true',
                         '--no-build') `
            -WorkingDirectory $RepoRoot
    }
} else {
    Write-Host ''
    Write-Host '  >> Skipping tests (-SkipTests)' -ForegroundColor Yellow
}

# -- Summary -------------------------------------------------------------------

$script:totalTime.Stop()
Write-Host ''
Write-Host '====================================================' -ForegroundColor Green
Write-Host "  Build completed successfully! ($([math]::Round($script:totalTime.Elapsed.TotalSeconds, 1))s)" -ForegroundColor Green
Write-Host "  Engine : $EngineExe" -ForegroundColor White
Write-Host '====================================================' -ForegroundColor Green
Write-Host ''
