param(
    [Parameter(Mandatory)] [string]$Version,
    [string]$Notes = "",
    [string]$PublishDir = "C:\Users\1108\nginx-1.28.3\html-filesearch\file-search",
    # Пересобрать интерфейс через npm. Нужен доступ к registry.npmjs.org.
    # Без ключа берётся готовый frontend/dist из репозитория — так собирает сервер без интернета.
    [switch]$BuildFrontend
)
$ErrorActionPreference = "Stop"
if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw "Версия должна быть вида 1.2.3" }

$root    = Split-Path -Parent $PSScriptRoot
$exeName = "file-search-$Version.exe"
$exe     = Join-Path $root "build\bin\$exeName"
$wails   = (Get-Command wails -ErrorAction SilentlyContinue).Source
if (-not $wails) { $wails = Join-Path (go env GOPATH) "bin\wails.exe" }
if (-not (Test-Path $wails)) { throw "Не найден wails. Установите: go install github.com/wailsapp/wails/v2/cmd/wails@latest" }

$buildArgs = @("build", "-clean", "-trimpath", "-webview2", "embed", "-o", $exeName,
    "-ldflags", "-s -w -X file-search/internal/update.CurrentVersion=$Version")
if (-not $BuildFrontend) {
    if (-not (Test-Path (Join-Path $root "frontend\dist\index.html"))) {
        throw "Нет готового интерфейса frontend\dist. Выполните git pull или запустите с -BuildFrontend"
    }
    # -s: не запускать npm, взять готовый frontend/dist.
    # -skipbindings: привязки frontend/wailsjs уже в репозитории; без флага Wails
    # перезаписывает их, и следующий git pull отказывается обновлять код.
    $buildArgs += "-s", "-skipbindings"
}

Push-Location $root
try {
    # Собираем только то, что лежит в GitHub: иначе выйдет старый код под новым номером.
    # git diff, а не git status: Wails перезаписывает файлы с другими окончаниями
    # строк, и status считает их изменёнными, хотя содержимое то же.
    $dirty = git diff --name-only HEAD
    if ($dirty) {
        throw ("В $root есть изменённые файлы, сборка взяла бы не тот код:`n" + ($dirty -join "`n") +
            "`nЕсли в списке только frontend/wailsjs, go.mod или go.sum — это следы прошлой сборки:" +
            "`n  git checkout -- frontend/wailsjs go.mod go.sum`n  git pull`nи запустите выпуск снова.")
    }
    git fetch -q origin
    if ($LASTEXITCODE) {
        Write-Warning "Не удалось связаться с GitHub — не проверено, что код свежий"
    } else {
        $behind = [int](git rev-list --count HEAD..origin/main)
        if ($behind -gt 0) { throw "Код отстаёт от GitHub на $behind коммит(а). Выполните git pull и запустите выпуск снова." }
    }
    Write-Host ("Собирается коммит: " + (git log -1 --format="%h %s"))

    go test ./...
    if ($LASTEXITCODE) { throw "Тесты не прошли" }
    # wails build встраивает интерфейс из frontend/dist в exe.
    # -webview2 embed: если на ПК нет WebView2, программа предложит его установить.
    & $wails @buildArgs
    if ($LASTEXITCODE) { throw "Сборка не удалась" }
} finally {
    # Вернуть файлы, которые Wails перезаписывает при сборке, — чтобы git pull не спотыкался.
    git checkout -q -- frontend/wailsjs go.mod go.sum
    Pop-Location
}

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

# Архив для новых сотрудников: внутри один FileSearch.exe
$staging = Join-Path $env:TEMP "FileSearch-$Version"
New-Item -ItemType Directory -Force $staging | Out-Null
Copy-Item $exe (Join-Path $staging "FileSearch.exe") -Force
$zipTmp = Join-Path $PublishDir "FileSearch.tmp.zip"
Compress-Archive -Path (Join-Path $staging "FileSearch.exe") -DestinationPath $zipTmp -Force
Move-Item $zipTmp (Join-Path $PublishDir "FileSearch.zip") -Force
Remove-Item -Recurse -Force $staging

Write-Host "Опубликовано $Version -> http://192.168.150.100:1212/file-search/latest.json"
Write-Host "Архив для скачивания -> http://192.168.150.100:1212/file-search/FileSearch.zip"
