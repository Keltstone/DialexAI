# ==============================================================================
# DialexAI Ecosystem Windows Installer (PowerShell)
# Installs Artix AI, Dialex AI, Kritix AI, and Socratix Deliberation Engine
# Supports:
#   - Standalone CLI binaries (socratix.exe, artix.exe, kritix.exe)
#   - Desktop Installers (MSI / EXE for Dialex AI, Artix AI, Kritix AI)
#   - With Engine & Without Engine variants
#   - Express Engine Permission check before updating
# ==============================================================================

[CmdletBinding()]
param (
    [switch]$All,
    [switch]$Artix,
    [switch]$Dialex,
    [switch]$Kritix,
    [switch]$EngineOnly,
    [switch]$CliOnly,
    [switch]$DesktopOnly,
    [switch]$WithEngine,
    [switch]$WithoutEngine,
    [switch]$UpdateEngine,
    [switch]$SkipEngine,
    [string]$InstallDir = "$env:LOCALAPPDATA\DialexAI",
    [switch]$NonInteractive,
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$Version = "1.0.0"

Write-Host "==============================================================================" -ForegroundColor Cyan
Write-Host "   🌟 DialexAI Ecosystem Windows Installer (v$Version)" -ForegroundColor Cyan
Write-Host "==============================================================================" -ForegroundColor Cyan
Write-Host "Install Directory: $InstallDir"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}
$BinDir = "$InstallDir\bin"
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
}

$InstallTarget = "all"
if ($EngineOnly) { $InstallTarget = "engine" }
elseif ($Artix) { $InstallTarget = "artix" }
elseif ($Dialex) { $InstallTarget = "dialex" }
elseif ($Kritix) { $InstallTarget = "kritix" }

$InstallMode = "both"
if ($CliOnly) { $InstallMode = "cli" }
elseif ($DesktopOnly) { $InstallMode = "desktop" }

$EngineChoice = "with"
if ($WithoutEngine) { $EngineChoice = "without" }

# ------------------------------------------------------------------------------
# Engine Detection and Express Permission Check
# ------------------------------------------------------------------------------
$ExistingEngine = Get-Command "socratix.exe" -ErrorAction SilentlyContinue
if (-not $ExistingEngine) {
    if (Test-Path "$BinDir\socratix.exe") {
        $ExistingEngine = "$BinDir\socratix.exe"
    } elseif (Test-Path "C:\Program Files\DialexAI\resources\engine\socratix.exe") {
        $ExistingEngine = "C:\Program Files\DialexAI\resources\engine\socratix.exe"
    }
}

$EngineAction = "install"

if ($ExistingEngine) {
    Write-Host "`n[!] Existing Socratix engine detected at: $ExistingEngine" -ForegroundColor Yellow
    if ($UpdateEngine) {
        Write-Host "[v] Express permission to update granted via -UpdateEngine parameter." -ForegroundColor Green
        $EngineAction = "update"
    } elseif ($SkipEngine) {
        Write-Host "[i] Skipping engine update via -SkipEngine parameter." -ForegroundColor Cyan
        $EngineAction = "skip"
    } elseif ($NonInteractive) {
        Write-Host "[i] Non-interactive mode: preserving existing engine (skipping update)." -ForegroundColor Cyan
        $EngineAction = "skip"
    } else {
        $Choice = Read-Host "Do you give express permission to UPDATE the existing Socratix engine to v$Version? [y/N]"
        if ($Choice -match "^[yY](es)?$") {
            Write-Host "[v] Express permission granted: Updating engine." -ForegroundColor Green
            $EngineAction = "update"
        } else {
            Write-Host "[i] Express permission declined: Preserving existing engine at $ExistingEngine." -ForegroundColor Cyan
            $EngineAction = "skip"
        }
    }
} else {
    Write-Host "[v] No existing engine detected. Clean install planned." -ForegroundColor Green
}

# ------------------------------------------------------------------------------
# Install Binaries
# ------------------------------------------------------------------------------
$RepoRoot = Split-Path -Parent $PSScriptRoot

if ($InstallMode -eq "cli" -or $InstallMode -eq "both") {
    Write-Host "`n📦 [1/2] Installing CLI Binaries..." -ForegroundColor Green

    # Socratix Engine
    if ($InstallTarget -eq "all" -or $InstallTarget -eq "engine" -or $EngineChoice -eq "with") {
        if ($EngineAction -ne "skip") {
            $srcEngine = "$RepoRoot\dist\bin\socratix.exe"
            if (Test-Path $srcEngine) {
                if ($EngineAction -eq "update" -and $ExistingEngine) {
                    $bak = "$ExistingEngine.bak.$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
                    Copy-Item $ExistingEngine $bak -Force
                    Write-Host "  Backed up previous engine to $bak"
                }
                Copy-Item $srcEngine "$BinDir\socratix.exe" -Force
                Write-Host "  [v] Installed $BinDir\socratix.exe" -ForegroundColor Green
            }
            $srcBench = "$RepoRoot\dist\bin\socratixbench.exe"
            if (Test-Path $srcBench) {
                Copy-Item $srcBench "$BinDir\socratixbench.exe" -Force
                Write-Host "  [v] Installed $BinDir\socratixbench.exe" -ForegroundColor Green
            }
        } else {
            Write-Host "  [i] Preserving existing Socratix engine per user choice." -ForegroundColor Cyan
        }
    }

    # Artix AI CLI
    if ($InstallTarget -eq "all" -or $InstallTarget -eq "artix") {
        $srcArtix = "$RepoRoot\dist\bin\artix.exe"
        if (Test-Path $srcArtix) {
            Copy-Item $srcArtix "$BinDir\artix.exe" -Force
            Write-Host "  [v] Installed $BinDir\artix.exe" -ForegroundColor Green
        }
        $srcArtixd = "$RepoRoot\dist\bin\artixd.exe"
        if (Test-Path $srcArtixd) {
            Copy-Item $srcArtixd "$BinDir\artixd.exe" -Force
            Write-Host "  [v] Installed $BinDir\artixd.exe" -ForegroundColor Green
        }
    }

    # Kritix AI CLI
    if ($InstallTarget -eq "all" -or $InstallTarget -eq "kritix") {
        $srcKritix = "$RepoRoot\dist\bin\kritix.exe"
        if (Test-Path $srcKritix) {
            Copy-Item $srcKritix "$BinDir\kritix.exe" -Force
            Write-Host "  [v] Installed $BinDir\kritix.exe" -ForegroundColor Green
        }
    }
}

# ------------------------------------------------------------------------------
# Install Desktop Packages (MSI / EXE)
# ------------------------------------------------------------------------------
if ($InstallMode -eq "desktop" -or $InstallMode -eq "both") {
    Write-Host "`n🖥️  [2/2] Locating Desktop Application Installers..." -ForegroundColor Green
    $VariantFolder = if ($EngineChoice -eq "with") { "with-engine" } else { "without-engine" }
    $DesktopDir = "$RepoRoot\dist\desktop\$VariantFolder"

    function Install-DesktopPackage($AppName) {
        $msi = Get-ChildItem -Path $DesktopDir -Filter "$AppName*.msi" -ErrorAction SilentlyContinue | Select-Object -First 1
        $exe = Get-ChildItem -Path $DesktopDir -Filter "$AppName*.exe" -ErrorAction SilentlyContinue | Select-Object -First 1

        if ($msi) {
            Write-Host "  Launching Windows MSI Installer for $AppName ($VariantFolder)..." -ForegroundColor Green
            if (-not $DryRun) {
                Start-Process "msiexec.exe" -ArgumentList "/i `"$($msi.FullName)`"" -Wait
            }
        } elseif ($exe) {
            Write-Host "  Launching Windows Executable Installer for $AppName ($VariantFolder)..." -ForegroundColor Green
            if (-not $DryRun) {
                Start-Process "$($exe.FullName)" -Wait
            }
        } else {
            Write-Host "  [i] $AppName ($VariantFolder) installer not found in $DesktopDir. Run ./scripts/build-dist.sh --desktop" -ForegroundColor Yellow
        }
    }

    if ($InstallTarget -eq "all" -or $InstallTarget -eq "dialex") { Install-DesktopPackage "DialexAI" }
    if ($InstallTarget -eq "all" -or $InstallTarget -eq "artix") { Install-DesktopPackage "ArtixAI" }
    if ($InstallTarget -eq "all" -or $InstallTarget -eq "kritix") { Install-DesktopPackage "KritixAI" }
}

# Add BinDir to User PATH if not present
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$BinDir*") {
    Write-Host "`nAdding $BinDir to User PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$BinDir", "User")
}

Write-Host "`n==============================================================================" -ForegroundColor Green
Write-Host "   🎉 DialexAI Ecosystem Windows Installation Finished!" -ForegroundColor Green
Write-Host "==============================================================================" -ForegroundColor Green
Write-Host "Quick commands: 'socratix serve', 'artix', 'kritix'"
