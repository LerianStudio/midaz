$ErrorActionPreference = 'Stop'; # Stop on all errors

# Define the paths
$binDir = Join-Path $env:ChocolateyInstall 'bin'
$mdzExecutable = Join-Path $binDir 'mdz.exe'

# Uninstallation process
Write-Host "Attempting to remove Mdz executable from $binDir..."

if (Test-Path $mdzExecutable) {
    try {
        Remove-Item -Path $mdzExecutable -Force
        Write-Host "Successfully removed Mdz executable from $binDir."
    } catch {
        Write-Warning "Failed to remove Mdz executable from $binDir. Error: $_"
    }
} else {
    Write-Warning "Mdz executable not found in $binDir. Nothing to remove."
}

# Clean up additional files in the tools directory
$toolsDir = "$(Split-Path -Parent $MyInvocation.MyCommand.Definition)"
Write-Host "Cleaning up tools directory: $toolsDir"

if (Test-Path $toolsDir) {
    try {
        Remove-Item -Path $toolsDir -Recurse -Force
        Write-Host "Successfully cleaned up tools directory."
    } catch {
        Write-Warning "Failed to clean up tools directory. Error: $_"
    }
} else {
    Write-Warning "Tools directory not found. Nothing to clean up."
}

Write-Host "Uninstallation complete."
