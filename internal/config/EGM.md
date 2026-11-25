# config

## Overview
Paquete que guarda la configuración local del usuario en un archivo JSON (`settings.json`), separado de la base de datos para no mezclar datos con ajustes. Define los valores iniciales del servicio de IA (DeepSeek), valida los cambios que llegan desde la interfaz y entrega a la interfaz una vista que muestra solo los últimos 4 caracteres de la clave de API. Lo usa `app.go` en la raíz del repositorio.

## Interface
- `DefaultBaseURL = "https://api.deepseek.com"`, `DefaultModel = "deepseek-flash"`, `DefaultAnalysisLevel = "low"` [config.go:DefaultBaseURL] [config.go:DefaultModel] [config.go:DefaultAnalysisLevel]
- `type Config`: lo que se guarda en `settings.json`, con los campos `AIEnabled` (`ai_enabled`), `WebSearchEnabled` (`web_search_enabled`), `APIKey` (`api_key`), `BaseURL` (`base_url`), `Model` (`model`), `VisionModel` (`vision_model`), `AnalysisLevel` (`analysis_level`) y `UpdateCheck` (`update_check`) [config.go:Config]
- `type View`: vista para la interfaz, sin `APIKey`. En su lugar trae `HasAPIKey` (`has_api_key`) y `APIKeyHint` (`api_key_hint`) [config.go:View]
- `type SaveInput`: solo los campos que el usuario puede cambiar. `AIEnabled`, `WebSearchEnabled` y `UpdateCheck` son `*bool`, y además incluye `ClearAPIKey` (`clear_api_key`) [config.go:SaveInput]
- `func Default() Config`: configuración inicial, sin clave [config.go:Default]
- `func Load(path string) (Config, error)`: lee el archivo y, si no existe, lo crea con los valores iniciales [config.go:Load]
- `func Save(path string, cfg Config) error`: escribe el archivo de forma atómica y con permisos restrictivos [config.go:Save]
- `func (c *Config) Apply(in SaveInput) error`: valida y aplica en el receptor los cambios enviados por la interfaz [config.go:Config.Apply]
- `func (c Config) ToView() View`: oculta la clave y deja solo una pista [config.go:Config.ToView]

## Invariants
- Valores iniciales: `AIEnabled=false`, `WebSearchEnabled=true`, `BaseURL=DefaultBaseURL`, `Model` y `VisionModel` iguales a `DefaultModel`, `AnalysisLevel="low"`, `UpdateCheck=true` y la clave vacía [config.go:defaults]
- Los únicos niveles de análisis válidos son `low`, `medium` y `high` [config.go:analysisLevels]
- `Load` con un archivo vacío o solo con espacios devuelve los valores iniciales y error `nil`, sin reescribir el archivo [config.go:Load]
- Si el JSON no se puede interpretar, `Load` devuelve `defaults()` junto con el error de `json.Unmarshal`. Cualquier otro error de lectura se devuelve junto con los valores iniciales [config.go:Load]
- Si el texto del archivo no contiene `"web_search_enabled"`, `Load` fuerza `WebSearchEnabled=true`. Es una búsqueda de texto simple, para compatibilidad con archivos anteriores [config.go:Load]
- `Load` y `Save` normalizan igual: a `BaseURL` le quitan espacios y diagonales finales, y si queda vacía usan `DefaultBaseURL`. Un `Model` vacío pasa a `DefaultModel` y un `VisionModel` vacío copia `Model` [config.go:Load] [config.go:Save]
- `Load` reemplaza un `AnalysisLevel` no válido por `DefaultAnalysisLevel`. `Save` no valida el nivel, y ninguna de las dos valida el esquema de `BaseURL` [config.go:Load] [config.go:Save]
- En `Apply`, un puntero `nil` o una cadena vacía (o solo con espacios) deja el valor actual sin cambios [config.go:Config.Apply]
- En `Apply`, `ClearAPIKey=true` borra la clave y tiene prioridad sobre una `APIKey` enviada en la misma llamada. Si no, una clave no vacía se guarda recortada [config.go:Config.Apply]
- `Apply` rechaza: un nivel fuera de `low`/`medium`/`high` («el nivel de análisis no es válido»), una `BaseURL` vacía o que no empiece con `https://` o `http://`, una `BaseURL` con espacios, tabuladores o saltos de línea, un modelo vacío («indique el modelo de IA») y un modelo de visión vacío («indique el modelo de visión») [config.go:Config.Apply]
- `Apply` no es transaccional: los campos que asignó antes de fallar una validación quedan modificados en el receptor, incluida la clave [config.go:Config.Apply]
- `ToView` nunca entrega `APIKey` completa. La pista es `""` si no hay clave, `••••` si la clave mide 4 bytes o menos, y `••••••••` seguido de los últimos 4 bytes en otro caso [config.go:Config.ToView]
- `APIKey` no lleva la etiqueta `json:"-"`, así que se guarda en texto plano en `settings.json`. En Windows, `Chmod(0600)` solo cambia el atributo de solo lectura: la protección real es la ACL del perfil en `%APPDATA%`. Si el archivo está dañado, `app.go` lo aparta como `settings.json.danado-<fecha>` en vez de sobrescribirlo [config.go:Config] [config.go:Save]

## Side Effects
- `Load` lee `path` con `os.ReadFile` y, si el archivo no existe, lo crea llamando a `Save` con los valores iniciales [config.go:Load]
- `Save` crea la carpeta padre con `os.MkdirAll(..., 0o700)` [config.go:Save]
- `Save` escribe un temporal `.settings-*.tmp` en la misma carpeta, con JSON indentado a 2 espacios y permisos `0o600`, y lo renombra sobre `path` con `os.Rename`. Un `os.Remove` diferido borra el temporal si algo falla. No hace `fsync` [config.go:Save]
- `Apply` y `ToView` solo trabajan en memoria, sin E/S [config.go:Config.Apply] [config.go:Config.ToView]

## Verification
- Run: `go test ./internal/config/...` (por ahora el paquete no tiene pruebas, así que este comando solo comprueba que compila)
