# Changelog

Todas las versiones relevantes de Convenios UAGro Desktop.

## [1.2.0] - 2026-10-23

### Agregado

- Radar de riesgo con semáforo alto, medio y bajo.
- Detección de expedientes incompletos: sin responsable, sin etiquetas, sin fecha o sin transcripción.
- Campo de responsable interno y filtro de clasificación pendiente.
- Convenios hermanos por contraparte, etiquetas y vigencias traslapadas.
- Asistente de preguntas en lenguaje natural con fuentes por convenio y página.
- Búsqueda de fragmentos en el texto original y en las transcripciones.
- Endpoint de respuesta con citas `[Convenio #N, página P]`.

### Corregido

- Riesgo calculado después de cargar las etiquetas.
- Migración de base de datos v2 a v3 para responsable y páginas del PDF.

## [1.1.0] - 2026-10-23

### Agregado

- Transcripción de páginas escaneadas con modelo de visión externo.
- Integración de la transcripción con la búsqueda FTS5.
- Progreso de transcripción, texto completo y descarga .txt.
- Modelo de texto y modelo de visión configurables por separado.
- Filtro para clasificar convenios con etiquetas pendientes.
- Migración de base de datos v1 a v2.
- Pruebas del flujo de transcripción y de la petición de visión.

### Corregido

- Actualización del texto y del índice de búsqueda después de transcribir.
- Recuperación de transcripciones interrumpidas al reiniciar.

## [1.0.0] - 2026-10-22

### Agregado

- Primera versión de escritorio en Go + Wails.
- Base local SQLite vacía.
- Importación de PDF, visor y descarga.
- Etiquetas, búsqueda, filtros, vigencias y borrado en lote.
- Respaldo ZIP.
- Extracción opcional de datos con IA.
- Instalador para Windows.
