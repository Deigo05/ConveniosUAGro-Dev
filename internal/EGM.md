# internal

## Overview
Agrupa los paquetes Go de la aplicación que no se exponen fuera del módulo. `app.go`, en la raíz, los une y publica sus funciones a la interfaz. Ningún paquete de aquí importa a `app.go`. Entre ellos, `store` y `ai` dependen de `municipios`, y `store` depende de `pdfutil` y de `fechas`.

## Subdirectories & Boundaries
- ai: [./ai/EGM.md](./ai/EGM.md) - Cliente del servicio de IA compatible con OpenAI (DeepSeek por omisión): extracción de datos, transcripción de páginas escaneadas, verificación con la imagen y búsqueda de ubicación en DuckDuckGo. Solo red, sin disco ni base de datos.
- config: [./config/EGM.md](./config/EGM.md) - `settings.json` del usuario: valores iniciales, validación de lo que llega de la interfaz y una vista que solo muestra los últimos 4 caracteres de la clave de API. Escritura atómica (permisos 0600, que en Windows no aplican ACL).
- fechas: [./fechas/EGM.md](./fechas/EGM.md) - Funciones puras que comparan la fecha de firma y la de vigencia capturadas con el cierre y la cláusula del texto, y explican las que conviene confirmar. Sin E/S.
- municipios: [./municipios/EGM.md](./municipios/EGM.md) - Función pura que convierte variantes de nombre de municipio de Guerrero en el nombre oficial («Acapulco» → «Acapulco de Juárez»).
- pdfutil: [./pdfutil/EGM.md](./pdfutil/EGM.md) - Importa PDF (valida tamaño y firma, copia con nombre aleatorio, SHA-256, texto, detección de escaneados) y exporta copias.
- store: [./store/EGM.md](./store/EGM.md) - Persistencia SQLite con FTS5, migraciones hasta el esquema 7, reglas de etiquetas, transcripciones, duplicados, versiones, archivados, respaldos, paquetes y CSV. Sin red.
- uagro: [./uagro/EGM.md](./uagro/EGM.md) - Catálogo de 35 facultades de la UAGro y resolución de nombres de facultad escritos libremente a nombre canónico y sede.
- update: [./update/EGM.md](./update/EGM.md) - Autoactualización desde los releases de GitHub: comprobación, descarga verificada (tamaño, SHA-256, encabezado MZ) e instalación con respaldo `.old`.
