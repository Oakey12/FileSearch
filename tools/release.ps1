param(
    [Parameter(Mandatory)] [string]$Version,
    [string]$Notes = "",
    [string]$PublishDir = "C:\Users\1108\nginx-1.28.3\html-filesearch\file-search"
)
$ErrorActionPreference = "Stop"
if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw "Версия должна быть вида 1.2.3" }

$root    = Split-Path -Parent $PSScriptRoot
$dist    = Join-Path $root "dist"
$exeName = "file-search-$Version.exe"
$exe     = Join-Path $dist $exeName
New-Item -ItemType Directory -Force $dist | Out-Null

Push-Location $root
try {
    go test ./...
    if ($LASTEXITCODE) { throw "Тесты не прошли" }
    go build -trimpath -ldflags "-s -w -H windowsgui -X file-search/internal/update.CurrentVersion=$Version" -o $exe .
    if ($LASTEXITCODE) { throw "Сборка не удалась" }
} finally { Pop-Location }

$hash = (Get-FileHash $exe -Algorithm SHA256).Hash.ToLower()
$size = (Get-Item $exe).Length

# Сначала exe, потом манифест: клиент не увидит версию без файла
New-Item -ItemType Directory -Force $PublishDir | Out-Null
Copy-Item $exe (Join-Path $PublishDir $exeName) -Force

$manifest = [ordered]@{
    version   = $Version
    url       = $exeName
    sha256    = $hash
    size      = $size
    notes     = $Notes
    published = (Get-Date -Format s)
} | ConvertTo-Json

$tmp = Join-Path $PublishDir "latest.json.tmp"
[IO.File]::WriteAllText($tmp, $manifest, (New-Object Text.UTF8Encoding $false))
Move-Item $tmp (Join-Path $PublishDir "latest.json") -Force

Write-Host "Опубликовано $Version -> http://192.168.150.100:1212/file-search/latest.json"