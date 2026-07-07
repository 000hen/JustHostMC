<#
.SYNOPSIS
    Formats code in the JustHostMC repository.

.DESCRIPTION
    Runs gofmt on Go files, clang-format on C# files, and xstyler on XAML files.
    By default, it modifies the files in place.

.PARAMETER Check
    When specified, checks if the files are formatted correctly without modifying them.
    Exits with code 1 if any files need formatting, which is useful for CI environments.

.EXAMPLE
    .\format.ps1
    .\format.ps1 -Check
#>
[CmdletBinding()]
param(
    [switch]$Check
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# -- Helpers -------------------------------------------------------------------

function Write-Status {
    param(
        [string]$Message,
        [ValidateSet('INFO','WARN','FAIL','OK')]
        [string]$Level = 'INFO'
    )
    $color = switch ($Level) {
        'INFO' { 'Cyan'   }
        'OK'   { 'Green'  }
        'WARN' { 'Yellow' }
        'FAIL' { 'Red'    }
    }
    Write-Host $Message -ForegroundColor $color
}

# -- Main ----------------------------------------------------------------------

Write-Host ''
Write-Host '====================================================' -ForegroundColor Cyan
if ($Check) {
    Write-Host '   JustHostMC - Code Format Check' -ForegroundColor Cyan
} else {
    Write-Host '   JustHostMC - Code Formatting' -ForegroundColor Cyan
}
Write-Host '====================================================' -ForegroundColor Cyan
Write-Host ''

$script:hasError = $false

# 1. Go Files
$goFiles = Get-ChildItem -Recurse -File -Filter *.go | Where-Object { $_.FullName -notmatch '\\(\.git|vendor)\\' }

if ($goFiles.Count -gt 0) {
    if ($Check) {
        Write-Status "Checking Go files formatting..." 'INFO'
        $unformattedGo = gofmt -l $goFiles.FullName
        if ($unformattedGo) {
            Write-Status "The following Go files are not formatted properly:" 'FAIL'
            $unformattedGo | ForEach-Object { Write-Host "  $_" }
            $script:hasError = $true
        } else {
            Write-Status "Go files are formatted correctly." 'OK'
        }
    } else {
        Write-Status "Formatting Go files..." 'INFO'
        gofmt -w $goFiles.FullName
    }
}

# 2. C# Files
$csFiles = Get-ChildItem -Recurse -File -Filter *.cs |
    Where-Object {
        $_.FullName -notmatch '\\(\.git|bin|obj)\\' `
        -and $_.Name -notmatch '\.g\.cs$' `
        -and $_.Name -notmatch '\.Designer\.cs$'
    }

if ($csFiles.Count -gt 0) {
    if ($Check) {
        Write-Status "Checking C# files formatting with clang-format..." 'INFO'
        try {
            clang-format --dry-run --Werror --style=file $csFiles.FullName
            if ($LASTEXITCODE -ne 0) {
                Write-Status "Some C# files are not formatted properly. Please run .\format.ps1 locally." 'FAIL'
                $script:hasError = $true
            } else {
                Write-Status "C# files are formatted correctly." 'OK'
            }
        } catch {
            Write-Status "Some C# files are not formatted properly. Please run .\format.ps1 locally." 'FAIL'
            $script:hasError = $true
        }
    } else {
        Write-Status "Formatting C# files with clang-format..." 'INFO'
        clang-format -i --style=file $csFiles.FullName
    }
}

# 3. XAML Files
dotnet tool restore | Out-Null
$xamlFiles = Get-ChildItem -Recurse -File -Filter *.xaml | Where-Object { $_.FullName -notmatch '\\(\.git|bin|obj)\\' }

if ($xamlFiles.Count -gt 0) {
    if ($Check) {
        Write-Status "Checking XAML files formatting with xstyler..." 'INFO'
        try {
            # xstyler with --passive checks files and returns non-zero if issues exist
            dotnet tool run xstyler -c Settings.XamlStyler -f $xamlFiles.FullName --passive
            if ($LASTEXITCODE -ne 0) {
                Write-Status "Some XAML files are not formatted properly. Please run .\format.ps1 locally." 'FAIL'
                $script:hasError = $true
            } else {
                Write-Status "XAML files are formatted correctly." 'OK'
            }
        } catch {
            Write-Status "Some XAML files are not formatted properly. Please run .\format.ps1 locally." 'FAIL'
            $script:hasError = $true
        }
    } else {
        Write-Status "Formatting XAML files with xstyler..." 'INFO'
        dotnet tool run xstyler -c Settings.XamlStyler -f $xamlFiles.FullName
    }
}

Write-Host ''
Write-Host '----------------------------------------------------' -ForegroundColor DarkGray

if ($Check) {
    if ($script:hasError) {
        Write-Host ''
        Write-Host '  Code formatting check failed!' -ForegroundColor Red
        Write-Host '  Please run .\format.ps1 locally and push the changes.' -ForegroundColor Yellow
        Write-Host ''
        exit 1
    } else {
        Write-Host ''
        Write-Host '  All files are formatted correctly!' -ForegroundColor Green
        Write-Host ''
        exit 0
    }
} else {
    Write-Host ''
    Write-Host '  Formatting complete!' -ForegroundColor Green
    Write-Host ''
}
