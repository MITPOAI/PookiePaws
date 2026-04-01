# PookiePaws Installer for Windows
# Usage: powershell -c "irm https://raw.githubusercontent.com/MITPOAI/PookiePaws/main/install.ps1 | iex"

$ErrorActionPreference = "Stop"

$owner   = "MITPOAI"
$repo    = "PookiePaws"
$bin     = "pookie.exe"
$installDir = "$env:LOCALAPPDATA\pookie"

function Write-Step { param($msg) Write-Host "  -> $msg" -ForegroundColor Cyan }
function Write-Ok   { param($msg) Write-Host "  OK $msg" -ForegroundColor Green }
function Write-Fail { param($msg) Write-Host "  ERR $msg" -ForegroundColor Red; exit 1 }

Write-Host ""
Write-Host "  PookiePaws Installer" -ForegroundColor Magenta
Write-Host "  Local-first marketing ops runtime" -ForegroundColor DarkGray
Write-Host ""

# Detect architecture
Write-Step "Detecting architecture..."
$arch = if ([System.Environment]::Is64BitOperatingSystem) { "amd64" } else { Write-Fail "Only 64-bit Windows is supported." }
Write-Ok "windows/$arch"

# Fetch latest release version
Write-Step "Fetching latest release..."
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$owner/$repo/releases/latest" -Headers @{ "User-Agent" = "pookie-installer" }
    $version = $release.tag_name -replace '^v', ''
    $tag     = $release.tag_name
} catch {
    Write-Fail "Could not fetch release info. Check your internet connection or visit https://github.com/$owner/$repo/releases"
}
Write-Ok "Latest version: $tag"

# Build download URL
$asset   = "pookie_${version}_windows_${arch}.zip"
$url     = "https://github.com/$owner/$repo/releases/download/$tag/$asset"
$tmpZip  = "$env:TEMP\pookie_install.zip"
$tmpDir  = "$env:TEMP\pookie_extract"

# Download
Write-Step "Downloading $asset..."
try {
    Invoke-WebRequest -Uri $url -OutFile $tmpZip -UseBasicParsing
} catch {
    Write-Fail "Download failed: $url"
}
Write-Ok "Downloaded $asset"

# Extract
Write-Step "Extracting..."
if (Test-Path $tmpDir) { Remove-Item $tmpDir -Recurse -Force }
Expand-Archive -Path $tmpZip -DestinationPath $tmpDir -Force
Write-Ok "Extracted"

# Install binary
Write-Step "Installing to $installDir..."
if (-not (Test-Path $installDir)) { New-Item -ItemType Directory -Path $installDir | Out-Null }
$exePath = Get-ChildItem -Recurse -Filter $bin $tmpDir | Select-Object -First 1 -ExpandProperty FullName
if (-not $exePath) { Write-Fail "Could not find $bin in archive." }
Copy-Item $exePath "$installDir\$bin" -Force
Write-Ok "Installed $bin"

# Add to PATH for current user
Write-Step "Updating PATH..."
$currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
if ($currentPath -notlike "*$installDir*") {
    [System.Environment]::SetEnvironmentVariable("PATH", "$currentPath;$installDir", "User")
    $env:PATH += ";$installDir"
    Write-Ok "Added $installDir to your PATH"
} else {
    Write-Ok "PATH already contains $installDir"
}

# Cleanup
Remove-Item $tmpZip  -Force -ErrorAction SilentlyContinue
Remove-Item $tmpDir  -Recurse -Force -ErrorAction SilentlyContinue

# Verify
Write-Step "Verifying installation..."
$installed = & "$installDir\$bin" version 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Ok $installed
} else {
    Write-Fail "Verification failed. Try running: $installDir\$bin version"
}

Write-Host ""
Write-Host "  PookiePaws $tag installed successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "  Next steps:" -ForegroundColor White
Write-Host "    1. Restart your terminal (to reload PATH)" -ForegroundColor DarkGray
Write-Host "    2. pookie init     <- configure your providers" -ForegroundColor DarkGray
Write-Host "    3. pookie start    <- launch the console" -ForegroundColor DarkGray
Write-Host "    4. Open http://127.0.0.1:18800 in your browser" -ForegroundColor DarkGray
Write-Host ""
