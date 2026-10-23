# Changelog

## [1.1.2] - 2026-10-23

### Cambiado

- Extracción de datos con `reasoning_effort=high` y hasta 32,000 tokens para mejorar la calidad.
- Estado "Analizando" visible en el listado mientras corre la extracción automática.
- Aviso explícito de "transcripción completa, extrayendo datos" en el procesamiento automático.
- Límite de espera del cliente ampliado a 300 segundos para páginas complejas.

## [1.1.1] - 2026-10-23

### Agregado

- Extracción y transcripción automáticas al confirmar la importación si la IA está activa.
- Procesamiento paralelo de hasta 4 páginas escaneadas.
- Indicador de actividad de IA en la barra superior.

### Corregido

- Reintentos de tokens para evitar transcripciones cortadas por longitud.
- Recuperación de transcripciones interrumpidas.
- Imagen de página reducida a 1600 px y JPEG de menor peso para acelerar el envío.
