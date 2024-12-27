$version = 'v1.41.0'

$ErrorActionPreference = 'Stop';

$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$binDir     = Join-Path $env:ChocolateyInstall 'bin'
$outputFile = Join-Path $toolsDir 'mdz.exe'

$versionFmt = $version -replace '^v', ''

# URL do arquivo zipado
$url        = "https://github.com/LerianStudio/midaz/releases/download/"+$version+"/midaz_"+$versionFmt+"_windows_amd64.zip"
$checksum   = '{{CHECKSUM}}'
$silentArgs = ''

# Argumentos do pacote
$packageArgs = @{
    packageName   = 'mdz'
    unzipLocation = $toolsDir
    url           = $url
    softwareName  = 'mdz*'
    checksum      = $checksum
    checksumType  = 'sha256'
}

# Instalar e descompactar o pacote
Install-ChocolateyZipPackage @packageArgs

# Verificar se o arquivo .exe foi extraído corretamente
if (-Not (Test-Path $outputFile)) {
    throw "O arquivo mdz.exe não foi encontrado após a extração do zip."
}

# Certificar-se de que o diretório global 'bin' existe
if (-Not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir | Out-Null
}

# Mover o executável para o diretório global
Write-Host "Copiando $outputFile para $binDir"
Copy-Item -Path $outputFile -Destination $binDir -Force

# Confirmar a instalação
Write-Host "Instalação completa. O executável mdz está disponível globalmente."
