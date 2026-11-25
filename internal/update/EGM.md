# update

## Overview
Busca versiones nuevas de la aplicación en GitHub y reemplaza el ejecutable de Windows. Cada push a `main` compila la aplicación en GitHub Actions y publica (o reemplaza) el release `v<VERSION>` con el ejecutable. El texto del release incluye las líneas `commit: <sha>` y `sha256: <hash>`. El paquete compara la versión y el commit locales con ese release, descarga el ejecutable verificándolo e instala la versión nueva apartando la actual como `.old`. Lo usa `app.go`.

## Interface
- `Owner = "Deigo05"`, `Repo = "ConveniosUAGro-Dev"`, `AssetName = "ConveniosUAGro.exe"` [update.go:Owner] [update.go:Repo] [update.go:AssetName]
- `type Build`: `Version`, `Commit` y `Date` del ejecutable en uso [update.go:Build]
- `type Info`: resultado de la búsqueda, con los campos JSON `available`, `pending`, `current_version`, `current_commit`, `version`, `commit`, `published`, `notes`, `url`, `size` y `checked_at`. Además tiene los campos internos `downloadURL` y `sha256`, que no se serializan [update.go:Info]
- `type Checker`: `HTTP`, `APIBase`, `Owner` y `Repo`. `APIBase` se puede cambiar en pruebas [update.go:Checker]
- `func New() *Checker`: cliente con timeout de 30 s y `APIBase = "https://api.github.com"` [update.go:New]
- `func (c *Checker) Check(ctx context.Context, local Build) (Info, error)`: compara el ejecutable actual con el último release publicado [update.go:Check]
- `func CompareVersions(a, b string) int`: compara versiones `1.2.3` y devuelve -1, 0 o 1 [update.go:CompareVersions]
- `func (c *Checker) Download(ctx context.Context, info Info, dst string, progress func(done, total int64)) error`: descarga el ejecutable del release a `dst` y lo verifica [update.go:Download]
- `func Install(exe, newFile string) error`: reemplaza `exe` por `newFile` [update.go:Install]
- `func Cleanup(exe string)`: borra los restos de una actualización anterior [update.go:Cleanup]

## Invariants
- Un 404 en `releases/latest` (sin release publicado, por ejemplo mientras se publica uno) hace que `Check` devuelva `Info` solo con los datos locales y el error `ErrNoRelease`, para que la interfaz no muestre «Al día» [update.go:Check] [update.go:ErrNoRelease]
- `Info.Version` es `tag_name` sin espacios y sin la `v` inicial. El commit sale de la línea que cumple `reCommit` (`commit: ` seguido de 7 a 40 caracteres hex) y el hash de la que cumple `reSHA` (`sha256: ` seguido de 64 caracteres hex), los dos en minúsculas. El `digest` del asset solo se usa si el texto no trae hash [update.go:Check] [update.go:reCommit] [update.go:reSHA]
- El asset se elige comparando su nombre con `AssetName` sin importar mayúsculas. `Available` exige que exista el asset y que `isNewer` sea verdadero [update.go:Check]
- `isNewer`: una versión mayor siempre cuenta y una menor nunca. Con la misma versión, cuenta un commit distinto, y si hay fechas, solo si el release se publicó después de compilar el ejecutable (cada push a `main` reemplaza el release) [update.go:isNewer]
- Los commits se comparan como prefijo en cualquier dirección, sin importar mayúsculas, así que un SHA corto sirve. Un valor vacío nunca coincide [update.go:sameCommit]
- `versionParts` quita la `v` inicial y todo lo que sigue a `-`, `+` o un espacio. Una parte que no se puede interpretar vale 0 [update.go:versionParts]
- `Pending` es verdadero si el HEAD de `main` no coincide ni con el commit del release ni con el local, o sea, hay una versión en camino. Un error en esa consulta no impide informar del release [update.go:Check]
- A las notas se les quitan las líneas `commit:` y `sha256:`, se normalizan los saltos de línea y se cortan a 4000 runas con «…» [update.go:cleanNotes]
- `getJSON`: un 404 se devuelve sin error, un 403 o 429 se reporta como límite de consultas de GitHub y cualquier otro código fuera de 2xx es error. Un fallo de red se reporta como problema de conexión. La respuesta JSON se limita a 4 MiB [update.go:getJSON]
- `Download` necesita el `downloadURL` interno, que solo llena `Check`. Un `Info` reconstruido desde JSON falla con «el release no incluye el ejecutable» [update.go:Download] [update.go:Info]
- `Download` exige HTTP 200 y rechaza un `Content-Length` mayor que `maxAsset` (300 MiB), una descarga de más de 300 MiB o de menos de 1024 bytes, un SHA-256 distinto al publicado (solo si se conoce el hash) y un archivo que no empiece con `MZ` (ejecutable de Windows) [update.go:Download] [update.go:maxAsset]
- `Install` aparta `exe` como `exe.old` antes de mover `newFile`. Si el segundo renombrado falla, intenta regresar `exe.old` a su lugar. Esto depende de que Windows permita renombrar un ejecutable en uso [update.go:Install]

## Side Effects
- Red: `GET {APIBase}/repos/{Owner}/{Repo}/releases/latest` y `GET {APIBase}/repos/{Owner}/{Repo}/commits/main`, con `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28` y `User-Agent: ConveniosUAGro-actualizador`, sin autenticación y con `c.HTTP` [update.go:Check] [update.go:getJSON]
- Red: `GET` a la `browser_download_url` del asset, con un `http.Client` propio de 15 minutos de timeout (no usa `c.HTTP`) [update.go:Download]
- `Check` lee el reloj para `CheckedAt` (RFC 3339) [update.go:Check]
- `Download` crea o trunca `dst` con `os.Create`, escribe en bloques de 256 KiB y llama a `progress(done, total)` tras cada bloque (`total` puede ser -1). Si algo falla, borra `dst` con un `os.Remove` diferido. Luego reabre `dst` para revisar el encabezado `MZ` [update.go:Download]
- `Install` borra `exe.old` si existe y renombra `exe` → `exe.old` y `newFile` → `exe`. El `.old` queda para borrarse al siguiente inicio [update.go:Install]
- `Cleanup` borra `exe.old` y `exe.new` sin reportar errores [update.go:Cleanup]

## Verification
- Run: `go test ./internal/update/...`
