# Convenios UAGro · Aplicación de escritorio

Aplicación local para registrar, buscar, clasificar y descargar los convenios de la Universidad Autónoma de Guerrero.

La aplicación **no incluye datos de ejemplo**: al abrirla por primera vez la base está vacía y muestra una pantalla para importar el primer PDF.

## Inicio rápido

1. Copia la carpeta `ConveniosUAGro` a la PC.
2. Haz doble clic en `ConveniosUAGro.exe`.
3. Importa uno o varios PDF con **Importar PDF**.
4. Completa los datos y las etiquetas de cada convenio.
5. Usa **Descargar PDF** en la ficha para guardar una copia del documento.

No hay que instalar un servidor ni iniciar sesión. Todo funciona en la PC.

## Requisitos

- Windows 10 u 11, 64 bits.
- WebView2 (incluido en Windows 11 y en la mayoría de Windows 10). Si falta, Windows mostrará el aviso para instalarlo.
- Para usar la IA opcional: conexión a internet y una clave de API compatible con OpenAI/DeepSeek.

## Qué incluye

- Importación de PDF individual o múltiple.
- Datos del convenio: dependencia, fecha de firma, vigencia, cláusula y notas.
- Etiquetas por ámbito, estado, ciudad, facultad y tema, con colores. Solo se permite **una ciudad** (la de la dependencia) y **una facultad** por convenio. Cada ciudad guarda su estado: no se puede combinar con otro (por ejemplo, Acapulco de Juárez con Veracruz) y todo estado de México va con «Nacional».
- Datos de firmantes: nombre y cargo por la dependencia y por la UAGro, más facultad/unidad académica y **ciudad de la facultad**.
- Etiqueta automática de facultad con formato `Facultad de <nombre> <ciudad de la facultad>`. La ciudad de la facultad es la sede de la UAGro, **nunca** la ciudad de la dependencia.
- Validación contra el catálogo de facultades y sedes de la UAGro para evitar etiquetas como “Turismo Tijuana”.
- Búsqueda de texto completo dentro del PDF y de los datos capturados.
- Filtros por etiquetas y por vigencia; columna de estado (vigente, por vencer, vencido, indefinida, sin fecha).
- Selección múltiple y eliminación en lote.
- Visor de PDF integrado y **descarga del PDF** con diálogo Guardar como, con un **nombre útil** aunque el archivo se haya subido como «Escaner123515.pdf»: `AAAA-MM-DD Tipo - Contraparte - Facultad.pdf`. También se pueden descargar varios a una carpeta.
- **Transcripción de páginas escaneadas** con el modelo de visión del servicio externo; el texto se guarda por página y se integra a la búsqueda.
- Si la IA está activa, la extracción y la transcripción **comienzan automáticamente al confirmar la importación**. También se extraen los datos al terminar una transcripción manual.
- Al importar, la app **permanece en la lista de convenios**; el avance de transcripción/análisis aparece en la misma pantalla.
- Las etiquetas sugeridas por la IA se aplican automáticamente, sin aprobación manual.
- Si falta la ciudad o el estado, si la ciudad sería nueva o si el domicilio se leyó con dudas, el modelo consulta DuckDuckGo con el nombre de la contraparte y su domicilio para confirmar el municipio (por ejemplo, «Huachinango» → Huauchinango, Puebla).
- Respaldo completo en un archivo ZIP (base de datos + PDF).
- **Exportar e importar** convenios en un paquete `.zip` (datos, etiquetas, transcripción y PDF) y exportar la lista a CSV. La importación acepta también respaldos ZIP y `convenios.db` de otra instalación.
- **Duplicados**: aviso antes de importar un PDF ya registrado, marca de «Posible duplicado» mientras exista el otro y una herramienta para revisarlos lado a lado, con los dos PDF abiertos juntos («Comparar PDFs»).
- **Historial de versiones**: un convenio renovado deja al anterior en su historial y la lista muestra solo la versión más reciente.
- Etiqueta **Convenio Abierto** para convenios que no especifican la facultad de la UAGro. Todo convenio analizado lleva su facultad (o Convenio Abierto) y su ámbito.
- Marca **«¿No es convenio?»** para archivos que la IA identifica como otro tipo de documento (actas, oficios, constancias), con su filtro en la lista.
- Unificación de nombres con posibles errores de lectura en la transcripción.
- **Aviso de actualizaciones** desde GitHub con instalación en un clic.
- Análisis opcional con IA para extraer dependencia, fechas y etiquetas desde PDF con texto.
- Filtro para clasificar los convenios que aún no tienen etiquetas.
- Aplicación de un solo usuario: sin login, sin cuentas y sin servidor propio.

## Dónde se guardan los datos

En Windows:

```text
%APPDATA%\ConveniosUAGro\
├── convenios.db      Base SQLite
├── pdfs\             PDF originales
├── respaldos\        Respaldos
└── settings.json     Configuración local
```

La clave de IA y la configuración se guardan en `settings.json`, solo en este equipo.

La versión portable y la instalada usan **la misma carpeta**. Para pasar a una versión nueva en la misma PC basta con cerrar la anterior y abrir el ejecutable nuevo; la base se migra sola al esquema actual (la versión anterior ya no podrá abrirla, por lo que conviene copiar la carpeta antes). Para otra PC, exporta un paquete desde **Ajustes → Exportar e importar** o copia la carpeta completa con la app cerrada.

## Actualizaciones

Cada push a `main` ejecuta `.github/workflows/release.yml`: corre las pruebas, compila con el commit y la fecha incrustados y publica el release `v<VERSION>` con `ConveniosUAGro.exe`, un ZIP y las notas (`commit:` y `sha256:`). La app consulta `releases/latest` al iniciar y cada 6 horas; si el commit o la versión son más nuevos muestra el aviso. Al actualizar, respalda la base, descarga, verifica el SHA-256, renombra el ejecutable actual a `.old`, coloca el nuevo y reinicia.

Para que el aviso funcione el repositorio debe ser público (o el release accesible) y debe existir al menos un release publicado por el flujo.

## IA opcional

En **Ajustes** se puede activar el análisis con IA, pegar una clave de API y elegir dos modelos:

- **Modelo de texto**: extrae dependencia, fechas, vigencia y etiquetas.
- **Modelo de visión**: transcribe las páginas escaneadas.
- **Nivel de análisis**: Rápido (predeterminado), Detallado o Máximo. En DeepSeek equivalen a `reasoning_effort` `low`, `high` y `max`; los niveles altos razonan más, tardan más y gastan más tokens.

La cola automática procesa 3 archivos a la vez y hasta 10 páginas escaneadas en paralelo entre todos. El costo por documento es el mismo (se cobra por token); solo cambia el tiempo total. Si el servicio responde 429 o un error temporal, la app reintenta sola.

El texto de cada página se guarda y se reconstruye el índice FTS, por lo que las palabras transcritas aparecen en la búsqueda. La ficha permite ver y descargar el texto completo como `.txt`.

- La extracción de datos necesita PDF con texto; la transcripción necesita un modelo que acepte imágenes.
- La transcripción se inicia desde la ficha y se ejecuta mientras la aplicación está abierta. Si se cierra, las páginas ya guardadas se conservan y se puede reintentar.
- El servicio externo puede cobrar por las imágenes enviadas. La app avisa el número de páginas antes de empezar.
- Si no se configura la IA, la aplicación funciona por completo de forma manual.

## Compilar desde el código

Requisitos: Go 1.26 o superior, Wails CLI v2.17 y Node.js (solo si se reconstruye el frontend; el frontend ya está incluido en `frontend/dist`).

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
wails build -s -skipbindings -clean -platform windows/amd64
```

El resultado queda en `build/bin/ConveniosUAGro.exe`.

La opción `-s` omite el build del frontend porque `frontend/dist` ya contiene los archivos estáticos listos. Si se modifica el HTML/CSS/JS, no hace falta compilar nada: se vuelve a ejecutar el comando anterior.

### Pruebas

```bash
go test ./internal/...
```

Para la prueba de importación de PDF:

```bash
SAMPLE_PDF=/ruta/a/un.pdf go test ./internal/pdfutil
```

## Estructura

```text
app.go                     Puente Wails: API local, diálogos nativos y servidor de PDF
main.go                    Arranque de Wails y assets embebidos
internal/store/            SQLite, convenios, etiquetas, búsqueda FTS5 y respaldos
internal/pdfutil/          Validación, copia y extracción de texto del PDF
internal/ai/               Cliente de IA, prompt, validación de fechas y etiquetas
internal/config/           settings.json (IA, clave, nivel de análisis y actualizaciones)
internal/update/           Consulta de releases de GitHub, descarga verificada y reemplazo del ejecutable
.github/workflows/         Compilación y publicación automática del release
frontend/dist/             Interfaz HTML/CSS/JS sin dependencias externas
build/appicon.png          Icono de la aplicación
```

## Limitaciones conocidas

- Compilación y entrega probadas para Windows 64 bits. El código es portable, pero el paquete entregado es Windows.
- No hay usuarios ni permisos: es una aplicación personal.
- No hay OCR local: la transcripción usa el modelo de visión externo y requiere clave/API.
- La transcripción se ejecuta desde la interfaz; si se cierra la app se detiene, aunque las páginas ya procesadas se conservan.
- La IA es opcional y depende de un servicio externo; su disponibilidad y costo no dependen de esta aplicación.
- El PDF se muestra con el visor de WebView2; en equipos sin WebView2 hay que instalarlo.
- El formato visible de los campos `date` depende del idioma de Windows (el valor guardado siempre es ISO `AAAA-MM-DD`).
- El ejecutable no está firmado digitalmente; Windows puede mostrar un aviso de SmartScreen la primera vez.

## Respaldo y restauración

Desde **Ajustes → Crear respaldo ZIP** se genera un archivo con `convenios.db`, la carpeta `pdfs/` y un `LEEME.txt`.

Para restaurar:

1. Cierra la aplicación.
2. Descomprime el respaldo.
3. Copia `convenios.db` y la carpeta `pdfs/` dentro de `%APPDATA%\ConveniosUAGro\`, reemplazando lo que exista.
4. Abre la aplicación.

## Estado de la entrega

- Versión: 1.2.0
- Base inicial: vacía (solo se crean las etiquetas base Nacional/Internacional, los 32 estados y «Convenio Abierto»).
- Sin datos de producción ni credenciales.
