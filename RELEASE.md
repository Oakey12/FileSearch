# Выпуск обновлений FileSearch через Nginx (:1212)

## Как это устроено

```text
P:\FileSearch  ──(FileSearchReleaseTool.exe)──►  C:\Users\1108\nginx-1.28.3\html-filesearch\file-search\
                                                   ├── latest.json            ← манифест последней версии
                                                   ├── file-search-1.0.3.exe  ← файл обновления
                                                   └── FileSearch.zip         ← для ручной установки
                                                              │
                                  http://192.168.150.100:1212/file-search/latest.json
                                                              │
                              Кнопка «Обновить» в программе ◄─┘
```

1. Кнопка **«Обновить»** (слева внизу окна) читает `latest.json`.
2. Если версия на сервере новее, программа показывает её номер и примечания и спрашивает подтверждение.
3. Программа скачивает `exe` в `<программа>.exe.new` и сверяет SHA-256.
4. Текущий `exe` переименовывается в `.old`, новый ставится на его место и запускается, старый процесс закрывается.
5. При следующем запуске остатки `.old` и `.new` удаляются сами.

Текущая версия показывается в левой панели над кнопкой «Обновить». Номер версии подставляется
при сборке флагом `-X file-search/internal/update.CurrentVersion=<версия>`, и вручную его править не нужно.

## Сборка (Wails)

С версии 2.0 интерфейс сделан на [Wails](https://wails.io): логика на Go, окно — WebView2.
Обычный `go build` здесь не подходит, собирать нужно через `wails build`: он собирает
фронтенд (`frontend/`) и встраивает его в exe.

На машине сборки нужны Go 1.25+, Node.js 20+ и Wails CLI:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor
```

Собрать и опубликовать одной командой:

```powershell
.\tools\release.ps1 -Version 2.0.0 -Notes "Новый интерфейс"
```

Для разработки: `wails dev` — окно с горячей перезагрузкой интерфейса.

Программа работает на Windows 10 и 11. Если на компьютере нет WebView2 (на Windows 11 он
встроен), при первом запуске программа предложит его установить.

> Папка, где лежит `exe` у пользователя, должна быть доступна ему на запись.
> Из `C:\Program Files` обновление не сработает.

## Выпуск новой версии

1. Закоммитить и отправить изменения в GitHub:

   ```powershell
   cd P:\FileSearch
   git add -A
   git commit -m "Описание изменений"
   git push
   ```

2. При необходимости поправить текст примечаний `UpdateNotes` в
   `P:\FileSearchReleaseTool\scripts\config.go` и пересобрать утилиту:

   ```powershell
   cd P:\FileSearchReleaseTool
   go build -o FileSearchReleaseTool.exe ./src
   ```

3. Запустить `P:\FileSearchReleaseTool\FileSearchReleaseTool.exe` и ввести версию
   **больше опубликованной** (например, `1.0.3`). Утилита:
   - запускает `go test ./...` и при ошибке останавливается;
   - собирает `file-search-<версия>.exe` прямо в каталог Nginx;
   - обновляет `FileSearch.zip`;
   - считает SHA-256 и атомарно записывает `latest.json`.

4. Проверить публикацию:

   ```powershell
   Invoke-RestMethod http://192.168.150.100:1212/file-search/latest.json
   ```

Перезапускать Nginx для выпуска версии **не нужно**: он раздаёт статические файлы, и новый
`latest.json` виден сразу.

### Откат

Утилита не даёт перезаписать уже опубликованную версию. Чтобы откатиться, соберите старый
коммит (`git checkout <коммит>`) и опубликуйте его под **новым** номером, например `1.0.4`.
Клиенты обновляются только на версию с большим номером.

## Nginx: конфигурация и перезапуск

server-блок для FileSearch (в `C:\Users\1108\nginx-1.28.3\conf\nginx.conf`, рядом с другими
проектами; порты других сервисов не трогать):

```nginx
server {
    listen      1212;
    server_name _;
    root        C:/Users/1108/nginx-1.28.3/html-filesearch;

    charset       utf-8;
    charset_types application/json;

    location /file-search/ {
        autoindex off;
    }

    # Манифест нельзя кэшировать, иначе клиенты не увидят новую версию
    location = /file-search/latest.json {
        add_header Cache-Control "no-store";
    }
}
```

Nginx перезапускают только после изменения конфигурации:

```powershell
cd C:\Users\1108\nginx-1.28.3
.\nginx.exe -t              # проверка конфигурации
.\nginx.exe -s reload       # применить без остановки
```

Если Nginx не запущен:

```powershell
cd C:\Users\1108\nginx-1.28.3
start nginx
netstat -ano | findstr :1212   # порт должен быть в состоянии LISTENING
```

Если с других компьютеров порт недоступен, нужно правило брандмауэра (выполняется от администратора):

```powershell
New-NetFirewallRule -DisplayName "Nginx FileSearch 1212" -Direction Inbound -Protocol TCP -LocalPort 1212 -Action Allow
```

## Первый переход на версию с кнопкой

Уже установленные копии без кнопки «Обновить» (версии до той, что вышла с кнопкой) не могут
обновиться сами. Их один раз обновляют вручную: скачать
`http://192.168.150.100:1212/file-search/FileSearch.zip` и заменить `exe`. Дальше обновления идут через кнопку.

## Замечания

- `C:\Users\1108\Desktop\filesearch-release-tool` — старая копия утилиты, настроенная под 1Cproc
  (порт 8080, несуществующий `VersionFile`, пакет `./src`). Для FileSearch используйте
  `P:\FileSearchReleaseTool`.
- `P:\FileSearchReleaseTool` собирает через `go build ./cmd` — для версии на Wails это не подходит.
  В `scripts\build_release.go` сборку нужно заменить на `wails build` (как в `tools\release.ps1`),
  а `BuildTarget` в `config.go` больше не используется.
