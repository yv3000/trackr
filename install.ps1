# trackr installer
# Usage: irm https://raw.githubusercontent.com/yv3000/trackr/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$repo    = "yv3000/trackr"
$exeName = "trackr.exe"
# Install to a directory that doesn't need admin and is easy to add to PATH
$installDir = "$env:USERPROFILE\.trackr\bin"

Write-Host ""
Write-Host "  trackr installer" -ForegroundColor Cyan
Write-Host "  ─────────────────────────────────" -ForegroundColor DarkGray
Write-Host ""

# Step 1: Find latest release
Write-Host "  Fetching latest release..." -ForegroundColor Gray
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest"
} catch {
    Write-Host "  ERROR: Could not reach GitHub API. Check your internet connection." -ForegroundColor Red
    exit 1
}

$version = $release.tag_name
$asset   = $release.assets | Where-Object { $_.name -eq $exeName } | Select-Object -First 1

if (-not $asset) {
    Write-Host "  ERROR: trackr.exe not found in release $version" -ForegroundColor Red
    exit 1
}

Write-Host "  Found: trackr $version" -ForegroundColor Green

# Step 2: Create install directory
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# Step 3: Download
$dest = Join-Path $installDir $exeName
Write-Host "  Downloading to $dest ..." -ForegroundColor Gray
try {
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $dest -UseBasicParsing
} catch {
    Write-Host "  ERROR: Download failed. $_" -ForegroundColor Red
    exit 1
}

Write-Host "  Downloaded successfully." -ForegroundColor Green

# Step 4: Add to PATH (User level — no admin needed)
$currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
if ($currentPath -notlike "*$installDir*") {
    [System.Environment]::SetEnvironmentVariable(
        "PATH",
        "$currentPath;$installDir",
        "User"
    )
    Write-Host "  Added to PATH." -ForegroundColor Green
} else {
    Write-Host "  Already in PATH." -ForegroundColor DarkGray
}

# Step 5: Done
Write-Host ""
Write-Host "  ✓ trackr $version installed!" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Restart your terminal, then run: trackr" -ForegroundColor White
Write-Host ""
