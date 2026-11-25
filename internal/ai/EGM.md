# Paquete `ai`

## Overview

`internal/ai` es el cliente de IA de la aplicación: habla con un servicio
compatible con la API de chat de OpenAI (DeepSeek por omisión) para cuatro
tareas sobre convenios de la UAGro:

1. **Extraer** los datos de un convenio a partir de su texto (`Extract`) y
   validarlos contra el catálogo local de etiquetas.
2. **Transcribir** una página escaneada con el modelo de visión (`TranscribeImage`).
3. **Verificar** con la imagen de la página las lecturas dudosas que detectó la
   extracción (`VerifyPage`).
4. **Responder** preguntas con base en fragmentos ya recuperados, con citas
   (`Answer`).

Cuando la extracción no deja clara la ubicación de la contraparte, el paquete
busca en DuckDuckGo (`websearch.go`) y le pide al modelo que confirme el estado
y la ciudad con esos resultados. Todo lo que devuelve el modelo pasa por
validación local: se recortan longitudes, se normalizan fechas a `AAAA-MM-DD`
y las etiquetas se aceptan solo si existen en el catálogo (salvo la ciudad, que
puede ser nueva). Los mensajes de error son en español y están pensados para
mostrarse tal cual al usuario. Lo usa `app.go` en la raíz del repositorio;
depende de `internal/municipios` (nombres oficiales de municipios) y de
`internal/uagro` (catálogo de facultades incrustado en el prompt).

## Interface

**Construcción y configuración**

- `New(apiKey, baseURL, model, visionModel string) *Client`: crea el cliente.
  Valores por omisión: `baseURL = "https://api.deepseek.com"`,
  `model = "deepseek-flash"`, `visionModel = model`; recorta espacios y la `/`
  final de `baseURL`; deja `WebSearch = true`, `Effort = EffortLow` y
  `HTTP = sharedHTTP`. [ai.go:New]
- `type Client struct { APIKey, BaseURL, Model, VisionModel string; WebSearch bool; Effort string; ReferenceDate string; HTTP *http.Client }`.
  `ReferenceDate` (`AAAA-MM-DD`) es la fecha de registro del documento;
  `Effort` es el nivel de razonamiento de la extracción. [ai.go:Client]
- Constantes `EffortLow = "low"`, `EffortMedium = "medium"`, `EffortHigh = "high"`. [ai.go:EffortLow]
- `NormalizeEffort(e string) string`: devuelve `medium` o `high` si coinciden
  (sin distinguir mayúsculas ni espacios); cualquier otro valor da `low`. [ai.go:NormalizeEffort]

**Operaciones con el modelo**

- `(*Client) Extract(ctx, texto string, catalogo []Tag, transcrito bool) (Extraction, error)`:
  compacta el texto, pide al modelo un JSON con los campos del convenio,
  lo interpreta y valida, revisa la fecha de firma, enriquece la ubicación por
  web si `WebSearch` está activo y anota en `Notas` lo que falte. `transcrito`
  indica que el texto trae páginas leídas por visión y activa las reglas de
  `correcciones` y `dudas`. [ai.go:Extract]
- `(*Client) TranscribeImage(ctx, imageBase64 string, page int) (string, error)`:
  transcripción literal de una página (JPEG en base64 sin prefijo `data:`) con
  `VisionModel`; quita cercas de código del resultado. [ai.go:TranscribeImage]
- `(*Client) VerifyPage(ctx, imageBase64 string, page int, transcripcion string, dudas []Doubt) ([]Correction, error)`:
  segunda lectura de la página enfocada en las dudas; devuelve solo las
  correcciones relacionadas con ellas. [ai.go:VerifyPage]
- `(*Client) Answer(ctx, question, contexto string) (string, error)`: responde
  usando solo los fragmentos de `contexto`, con citas `[Convenio #N, página P]`. [ai.go:Answer, ai.go:answerPrompt]
- `CompactText(texto string, limit int) string`: normaliza espacios y líneas en
  blanco y, si el texto excede `limit` runas, conserva inicio, final y párrafos
  con palabras clave. [ai.go:CompactText]

**Tipos de datos**

- `type Tag struct { Nombre, Tipo, Estado string }`: elemento del catálogo
  local; `Estado` acompaña a las ciudades. [ai.go:Tag]
- `type Suggestion struct { Tipo, Nombre string; Nueva bool }` (JSON `tipo`,
  `nombre`, `nueva`): etiqueta propuesta; `Nueva` indica una ciudad que no está
  en el catálogo. [ai.go:Suggestion]
- `type Extraction struct`: resultado validado. Campos: `Dependencia`,
  `DependenciaCorta`, `TipoDocumento`, `FirmanteDependencia`,
  `CargoDependencia`, `FirmanteUAGro`, `CargoUAGro`, `Facultad`,
  `FacultadCiudad`, `FechaFirma`, `FechaVigencia`, `VigenciaIndefinida`,
  `VigenciaCalculada`, `VigenciaTexto`, `Notas`, `Estado` (`"ok"` o
  `"parcial"`), `Sugerencias`, `Correcciones`, `Renovacion`, `FechaAnterior`,
  `EsConvenio`, `Domicilio`, `LugarFirma`, `Marcas`, `Dudas`. [ai.go:Extraction]
- `type Doubt struct { Paginas []int; Dato string; Lecturas []string }` (JSON
  `paginas`, `dato`, `lecturas`): dato leído de formas distintas. [ai.go:Doubt]
- `type Correction struct { Original, Correcto string }` (JSON `original`,
  `correcto`): nombre mal leído y su forma correcta. [ai.go:Correction]

**Internos relevantes (no exportados)**

- `chat` / `post`: única salida HTTP al servicio de IA, con reintentos y
  traducción de errores. [ai.go:chat, ai.go:post]
- `interpret`, `validateSuggestions`, `officialCityNames`, `parseDoubts`,
  `parseCorrections`, `relevantCorrections`, `parseDate`, `parseSpanishDate`:
  validación de la respuesta del modelo. [ai.go:interpret, ai.go:validateSuggestions]
- `enrichLocation`, `confirmLocation`, `searchWeb`, `fetchSearch`,
  `locationQuery`, `brandQuery`: búsqueda web de la ubicación de la contraparte,
  llamada solo desde `Extract`. [websearch.go:enrichLocation, websearch.go:searchWeb]

No hay subdirectorios con contrato propio.

## Invariants

**Llamadas al servicio**

- `Extract`, `Answer`, `TranscribeImage` y `VerifyPage` fallan con
  `"no hay una clave de IA configurada"` si `APIKey` está vacía, sin hacer red. [ai.go:Extract, ai.go:Answer, ai.go:TranscribeImage, ai.go:VerifyPage]
- `VerifyPage` devuelve `nil, nil` sin llamar al modelo si no hay imagen o no
  hay dudas; `TranscribeImage` falla si la imagen está vacía; `Answer` falla si
  `contexto` está vacío; `Extract` falla si el texto compactado queda vacío. [ai.go:VerifyPage, ai.go:TranscribeImage, ai.go:Answer, ai.go:Extract]
- Reintentos solo ante errores pasajeros: HTTP 429, HTTP 5xx o error de red que
  no sea cancelación ni `DeadlineExceeded`; como máximo 2 reintentos, con
  esperas de 3 s y 10 s, abortando si se cancela el contexto. [ai.go:retryDelays, ai.go:transient, ai.go:chat]
- Errores HTTP traducidos a mensajes fijos: 401/403 → clave inválida, 429 →
  límite de uso, 400 → solicitud rechazada (sugiere revisar el modelo de visión),
  otro → código numérico. Una respuesta sin `choices` es error. [ai.go:chat]
- Si el modelo corta por `finish_reason == "length"` se repite la petición con
  más `max_tokens`, una sola vez: `Extract` 12000→24000 (24000→48000 si el
  esfuerzo no es `low`), `TranscribeImage` 12000→24000, `VerifyPage`
  8000→16000, `Answer` 4000→8000. Si el segundo intento también se corta,
  error. [ai.go:Extract, ai.go:TranscribeImage, ai.go:VerifyPage, ai.go:Answer]
- Con un `BaseURL` que contiene `deepseek`, el esfuerzo se traduce
  `low→low`, `medium→high`, `high→max`; con otro proveedor se envía tal cual
  (normalizado). `TranscribeImage` y `VerifyPage` siempre envían `low` sin
  traducir. [ai.go:apiEffort, ai.go:TranscribeImage, ai.go:VerifyPage]
- `ReferenceDate` va en el mensaje del usuario, no en el system prompt, para no
  romper la caché del prompt fijo. [ai.go:Extract]
- Temperatura 0.1 en `Extract` y `Answer`, 0.0 en `TranscribeImage`,
  `VerifyPage` y `confirmLocation`; `Answer` no envía `reasoning_effort`. [ai.go:Extract, ai.go:Answer, ai.go:TranscribeImage, ai.go:VerifyPage, websearch.go:confirmLocation]
- La respuesta JSON del modelo se toma entre la primera `{` y la última `}`,
  quitando cercas de código; si no se puede leer, error de formato inválido. [ai.go:parseJSON]
- `systemPrompt` fija las claves JSON que se piden al modelo y se arma al
  iniciar el paquete con el catálogo de facultades de `uagro.CatalogText()`. [ai.go:systemPrompt]
- El texto de entrada se limita a `maxTextChars = 32000` runas: se conservan
  16000 del inicio, 9000 del final y hasta 7000 de párrafos intermedios que
  contengan palabras clave (vigencia, firma, facultad, domicilio…), cada uno
  recortado a 1500. [ai.go:maxTextChars, ai.go:CompactText, ai.go:reKeyClause]

**Validación de la extracción**

- Longitudes máximas en runas: `Dependencia` 500, `DependenciaCorta` 60,
  `TipoDocumento` 80, `Domicilio` 200, `LugarFirma` 120, `Marcas` 120,
  firmantes y cargos 200, `Facultad` 200, `FacultadCiudad` 100,
  `VigenciaTexto` 500. [ai.go:interpret]
- `EsConvenio` vale `true` si el modelo no envía `es_convenio`. [ai.go:interpret]
- Las fechas son `AAAA-MM-DD` con año entre 1950 y 2100, o cadena vacía;
  también se aceptan fechas en texto español (`"5 de marzo de 2024"`, `5/3/2024`)
  validando que el día exista. [ai.go:parseDate, ai.go:parseSpanishDate, ai.go:meses]
- Vigencia por duración: se suma a la fecha de firma (o a `vigencia_inicio`) y
  marca `VigenciaCalculada`; si no se puede calcular, se anota en `Notas`.
  `indefinida` pone `VigenciaIndefinida`. [ai.go:interpret]
- `Estado = "ok"` solo si hay `Dependencia` y además `FechaVigencia` o
  vigencia indefinida; si no, `"parcial"`. [ai.go:interpret]
- Se anota en `Notas` cuando la vigencia es anterior a la firma y cuando la
  fecha de firma es posterior a `ReferenceDate`. [ai.go:interpret, ai.go:checkSignatureDate]
- Etiquetas (`Sugerencias`): máximo 8; nunca de tipo `facultad` (la crea el
  sistema con el campo `facultad`); a lo sumo una ciudad, un ámbito y un estado,
  y hasta 4 temas; ámbito, estado y tema solo si existen en el catálogo
  (comparación sin acentos ni mayúsculas); la ciudad puede quedar con
  `Nueva = true`; se aceptan alias de tipo (`rubro`→`tema`) y de estado
  (`cdmx`→`Ciudad de México`, `edomex`→`Estado de México`…); sin duplicados ni
  caracteres de control. [ai.go:validateSuggestions, ai.go:key]
- La ciudad sugerida se cambia por el nombre oficial del municipio según el
  estado sugerido, y deja de ser nueva si ese nombre ya está en el catálogo. [ai.go:officialCityNames]
- `Correcciones` y `Dudas` solo se llenan cuando `transcrito` es `true`.
  Correcciones: máximo 10, ambos lados no vacíos, distintos y de hasta 120
  runas. Dudas: máximo 3, cada una con al menos una página (hasta 3 páginas) y
  hasta 4 lecturas. [ai.go:Extract, ai.go:parseCorrections, ai.go:parseDoubts]
- `VerifyPage` descarta correcciones cuyo `Original` no comparta una palabra
  significativa (4 letras o más, o un número) con alguna lectura en duda. [ai.go:relevantCorrections, ai.go:significantWords]
- Cuando un convenio sin ámbito `Internacional` queda sin estado, se agrega una
  nota que distingue si no se buscó, si no se confirmó o si no hubo internet. [ai.go:noteMissingLocation]

**Búsqueda web de la ubicación**

- Solo se busca si `EsConvenio` y falta estado o ciudad, si la ciudad sería
  nueva o si el domicilio trae `[?]` o `[ilegible]`. Primero por razón social
  más domicilio (o lugar de firma), luego por marcas (máximo 3); consultas de
  hasta 220 runas. [websearch.go:enrichLocation, websearch.go:locationQuery, websearch.go:brandQuery, websearch.go:reDoubt]
- Las búsquedas a DuckDuckGo van de una en una (mutex global), con al menos
  2.5 s entre ellas; los resultados se guardan en caché por consulta durante la
  vida del proceso (también las búsquedas sin resultados; los fallos no se
  guardan); si DuckDuckGo responde con el reto anti-robots se dejan de
  intentar durante 15 min. [websearch.go:search, websearch.go:searchGap, websearch.go:searchBackoff, websearch.go:searchWeb]
- Se toman como máximo 5 resultados (título y fragmento). [websearch.go:parseResults]
- La ubicación que confirma el modelo pasa por `validateSuggestions` y solo
  reemplaza las etiquetas de ámbito, estado y ciudad; las demás se conservan, y
  también el ámbito o estado originales si la web no los confirmó (la ciudad
  original no se recupera). Si la
  ubicación cambió y no hay domicilio (o cambió el estado leído), se anota en
  `Notas` que salió de internet. [websearch.go:enrichLocation, websearch.go:confirmLocation]
- Un fallo de búsqueda o de `confirmLocation` nunca hace fallar `Extract`;
  solo se reporta como `unavailable` cuando ninguna consulta llegó a hacerse. [websearch.go:enrichLocation, websearch.go:confirmLocation]

## Side Effects

- Red: `POST {BaseURL}/chat/completions` con `Authorization: Bearer {APIKey}` y
  `Content-Type: application/json`; cuerpo con `model`, `messages`,
  `temperature`, `max_tokens` y, según la operación, `reasoning_effort` y
  `response_format: json_object`. Respuesta leída hasta 8 MiB. [ai.go:post, ai.go:chat]
- Las imágenes se envían como `image_url` con URL `data:image/jpeg;base64,...`
  al `VisionModel`. [ai.go:TranscribeImage, ai.go:VerifyPage]
- Estado global compartido: `sharedHTTP` (timeout 300 s, hasta 16 conexiones
  inactivas por host) lo reutilizan todos los clientes creados con `New`. [ai.go:sharedHTTP]
- Red: `GET https://html.duckduckgo.com/html/?q=...` con `User-Agent`
  `"Mozilla/5.0 (Windows NT 10.0; Win64; x64) ConveniosUAGro/1.0"` y
  `Accept-Language: es-MX,es;q=0.9`, cliente propio con timeout de 20 s,
  respuesta leída hasta 2 MiB. Sin clave. [websearch.go:fetchSearch]
- Estado global mutable protegido por mutex: caché de búsquedas (sin límite de
  tamaño ni expiración), hora de la última búsqueda y bloqueo temporal. El
  mutex se mantiene durante la espera y la petición, así que búsquedas
  concurrentes se serializan. [websearch.go:search, websearch.go:searchWeb]
- Log: `log.Printf` cuando DuckDuckGo pide verificación anti-robots. [websearch.go:searchWeb]
- Modifica el `*Extraction` del llamador: reemplaza `Sugerencias` y agrega texto
  a `Notas`. [websearch.go:enrichLocation, ai.go:checkSignatureDate, ai.go:noteMissingLocation]
- Sin acceso a disco ni base de datos.

## Verification

- Run: `go test ./internal/ai/...` (desde la raíz del repositorio, donde está `go.mod`)
