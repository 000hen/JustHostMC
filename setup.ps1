<#
.SYNOPSIS
    JustHostMC - Development environment setup.
    Checks that every prerequisite tool is installed and offers to install
    missing Go-based tools (buf, protoc-gen-go, protoc-gen-go-grpc) via
    'go install'.

.DESCRIPTION
    Run this once after cloning the repository:
        .\setup.ps1

    The script never installs Go or .NET SDK itself - those require platform
    installers - but prints actionable download links when they are missing.

.PARAMETER InstallMissing
    When set, automatically installs missing Go-based tools without prompting.

.EXAMPLE
    .\setup.ps1
    .\setup.ps1 -InstallMissing
#>
[CmdletBinding()]
param(
    [switch]$InstallMissing
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# -- Helpers -------------------------------------------------------------------

function Write-Status {
    param(
        [string]$Tool,
        [string]$Status,
        [string]$Detail,
        [ValidateSet('OK','WARN','FAIL')]
        [string]$Level = 'OK'
    )
    $color = switch ($Level) {
        'OK'   { 'Green'  }
        'WARN' { 'Yellow' }
        'FAIL' { 'Red'    }
    }
    $symbol = switch ($Level) {
        'OK'   { '[OK]  ' }
        'WARN' { '[WARN]' }
        'FAIL' { '[FAIL]' }
    }
    Write-Host "$symbol " -ForegroundColor $color -NoNewline
    Write-Host "$Tool" -ForegroundColor White -NoNewline
    if ($Detail) {
        Write-Host " - $Detail" -ForegroundColor DarkGray
    } else {
        Write-Host ''
    }
}

function Test-Command {
    param([string]$Name)
    $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Get-ToolVersion {
    param([string]$Name, [string]$VersionArg = '--version')
    try {
        $output = & $Name $VersionArg 2>&1 | Out-String
        return $output.Trim()
    } catch {
        return $null
    }
}

function Install-GoTool {
    param(
        [string]$DisplayName,
        [string]$ImportPath
    )

    if (-not $script:hasGo) {
        Write-Status $DisplayName 'FAIL' 'Cannot install - Go is not available' -Level FAIL
        $script:failures++
        return $false
    }

    $doInstall = $false
    if ($InstallMissing) {
        $doInstall = $true
    } else {
        $answer = Read-Host "    Install $DisplayName via 'go install $ImportPath'? [Y/n]"
        $doInstall = ($answer -eq '' -or $answer -match '^[Yy]')
    }

    if ($doInstall) {
        Write-Host "    Installing $DisplayName..." -ForegroundColor Cyan
        try {
            & go install $ImportPath 2>&1 | Out-String | Write-Host
            if ($LASTEXITCODE -eq 0) {
                Write-Status $DisplayName 'OK' 'installed via go install' -Level OK
                return $true
            } else {
                Write-Status $DisplayName 'FAIL' "go install exited with code $LASTEXITCODE" -Level FAIL
                $script:failures++
                return $false
            }
        } catch {
            Write-Status $DisplayName 'FAIL' $_.Exception.Message -Level FAIL
            $script:failures++
            return $false
        }
    } else {
        Write-Status $DisplayName 'FAIL' 'skipped by user' -Level FAIL
        $script:failures++
        return $false
    }
}

# -- Main ----------------------------------------------------------------------

Write-Host ''
Write-Host '====================================================' -ForegroundColor Cyan
Write-Host '   JustHostMC - Development Environment Setup' -ForegroundColor Cyan
Write-Host '====================================================' -ForegroundColor Cyan
Write-Host ''

$script:failures = 0
$script:hasGo = $false

# -- 1. Go ---------------------------------------------------------------------
Write-Host 'Checking prerequisites...' -ForegroundColor White
Write-Host ''

if (Test-Command 'go') {
    $goVer = Get-ToolVersion 'go' 'version'
    Write-Status 'Go' 'OK' $goVer -Level OK
    $script:hasGo = $true
} else {
    Write-Status 'Go' 'FAIL' 'Not found - download from https://go.dev/dl/' -Level FAIL
    $script:failures++
}

# -- 2. .NET SDK ---------------------------------------------------------------
if (Test-Command 'dotnet') {
    $dotnetVer = Get-ToolVersion 'dotnet' '--version'
    Write-Status '.NET SDK' 'OK' "v$dotnetVer" -Level OK
} else {
    Write-Status '.NET SDK' 'FAIL' 'Not found - download from https://dotnet.microsoft.com/download' -Level FAIL
    $script:failures++
}

# -- 3. buf --------------------------------------------------------------------
if (Test-Command 'buf') {
    $bufVer = Get-ToolVersion 'buf' '--version'
    Write-Status 'buf' 'OK' "v$bufVer" -Level OK
} else {
    Write-Host ''
    Write-Status 'buf' 'FAIL' 'Not found' -Level FAIL
    Write-Host '    buf is required for protobuf code generation.' -ForegroundColor DarkGray
    Write-Host '    Install options:' -ForegroundColor DarkGray
    Write-Host '      * winget:     winget install Bufbuild.Buf' -ForegroundColor Yellow
    Write-Host '      * scoop:      scoop install buf' -ForegroundColor Yellow
    Write-Host '      * go install: go install github.com/bufbuild/buf/cmd/buf@latest' -ForegroundColor Yellow
    Write-Host '      * manual:     https://buf.build/docs/installation' -ForegroundColor Yellow
    Write-Host ''

    # Offer go install as a fallback
    if ($script:hasGo) {
        $installed = Install-GoTool 'buf' 'github.com/bufbuild/buf/cmd/buf@latest'
        if (-not $installed) { $script:failures++ }
    } else {
        $script:failures++
    }
}

# -- 4. protoc-gen-go ----------------------------------------------------------
if (Test-Command 'protoc-gen-go') {
    $pgVer = Get-ToolVersion 'protoc-gen-go' '--version'
    Write-Status 'protoc-gen-go' 'OK' $pgVer -Level OK
} else {
    Write-Host ''
    Write-Status 'protoc-gen-go' 'FAIL' 'Not found' -Level FAIL
    Install-GoTool 'protoc-gen-go' 'google.golang.org/protobuf/cmd/protoc-gen-go@latest'
}

# -- 5. protoc-gen-go-grpc ----------------------------------------------------
if (Test-Command 'protoc-gen-go-grpc') {
    $pggVer = Get-ToolVersion 'protoc-gen-go-grpc' '--version'
    Write-Status 'protoc-gen-go-grpc' 'OK' $pggVer -Level OK
} else {
    Write-Host ''
    Write-Status 'protoc-gen-go-grpc' 'FAIL' 'Not found' -Level FAIL
    Install-GoTool 'protoc-gen-go-grpc' 'google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest'
}

# -- Summary -------------------------------------------------------------------
Write-Host ''
Write-Host '----------------------------------------------------' -ForegroundColor DarkGray

if ($script:failures -eq 0) {
    Write-Host ''
    Write-Host '  All prerequisites satisfied!' -ForegroundColor Green
    Write-Host '  Run .\build.ps1 to build the project.' -ForegroundColor White
    Write-Host ''
    exit 0
} else {
    Write-Host ''
    Write-Host "  $($script:failures) prerequisite(s) missing or failed." -ForegroundColor Red
    Write-Host '  Please install the missing tools and re-run .\setup.ps1' -ForegroundColor Yellow
    Write-Host ''
    exit 1
}
