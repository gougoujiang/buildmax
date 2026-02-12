# Install buildmax locally to %userprofile%/.local/bin
# This allows testing with other projects

# Get the current directory where this script is located
$scriptDir = $PSScriptRoot

# Define the source and destination paths
$sourceExe = Join-Path $scriptDir "buildmax.exe"
$localBin = Join-Path $env:USERPROFILE ".local\bin"
$destinationExe = Join-Path $localBin "buildmax.exe"

Write-Host "Installing buildmax to $localBin..."

# Check if buildmax.exe exists in the current directory
if (-not (Test-Path $sourceExe)) {
    Write-Error "buildmax.exe not found in the current directory: $scriptDir"
    Write-Host "Please make sure buildmax.exe is in the same directory as this script."
    exit 1
}

# Create the .local/bin directory if it doesn't exist
if (-not (Test-Path $localBin)) {
    Write-Host "Creating directory: $localBin"
    New-Item -ItemType Directory -Path $localBin -Force | Out-Null
}

# Copy the executable
Write-Host "Copying buildmax.exe to $destinationExe"
Copy-Item $sourceExe $destinationExe -Force

# Check if .local/bin is already in PATH
$localBinInPath = $env:PATH -split ';' -contains $localBin

if (-not $localBinInPath) {
    Write-Host "`n$localBin is not in your PATH."
    Write-Host "To use buildmax from any directory, add it to your PATH:"
    Write-Host "`$env:PATH += `";$localBin`""
    Write-Host "Or add this to your PowerShell profile to make it permanent:"
    Write-Host "[System.Environment]::SetEnvironmentVariable('PATH', `$env:PATH + `";$localBin`", 'User')"
    Write-Host "`nAfter adding to PATH, start a new terminal session."
} else {
    Write-Host "`n$localBin is already in your PATH."
    Write-Host "buildmax is now available from any directory."
}

Write-Host "`nInstallation complete!"
Write-Host "You can now run 'buildmax' from any directory (after updating PATH if needed)."