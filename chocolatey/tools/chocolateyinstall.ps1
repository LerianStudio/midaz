$version = 'v1.44.1'

$ErrorActionPreference = 'Stop';

$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$binDir     = Join-Path $env:ChocolateyInstall 'bin'
$outputFile = Join-Path $toolsDir 'mdz.exe'

$versionFmt = $version -replace '^v', ''

# Zipped file URL
$url        = "https://github.com/LerianStudio/midaz/releases/download/"+$version+"/midaz_"+$versionFmt+"_windows_amd64.zip"
$checksum   = '{{CHECKSUM}}'
$silentArgs = ''

# Package arguments
$packageArgs = @{
    packageName   = 'mdz'
    unzipLocation = $toolsDir
    url           = $url
    softwareName  = 'mdz*'
    checksum      = $checksum
    checksumType  = 'sha256'
}

# Install and unzip the package
Install-ChocolateyZipPackage @packageArgs

# Check that the .exe file has been extracted correctly
if (-Not (Test-Path $outputFile)) {
    throw "The file mdz.exe was not found after extracting the zip."
}

# Make sure the global directory 'bin' exists
if (-Not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir | Out-Null
}

# Move the executable to the global directory
Write-Host "Copying $outputFile to $binDir"
Copy-Item -Path $outputFile -Destination $binDir -Force

# Confirm installation
Write-Host "Installation complete. The mdz executable is available globally.
