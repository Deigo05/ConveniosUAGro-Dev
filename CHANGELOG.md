# Changelog

## [1.1.4] - 2026-10-23

### Cambiado

- Solo se permite **una etiqueta de ciudad** por convenio. La IA sugiere como máximo una y debe ser la ciudad de la dependencia, no la de la UAGro.
- Las etiquetas de facultad ahora son un **tipo propio** (`facultad`), separado de Tema / Rubro.
- El nombre de la etiqueta de facultad sigue el formato `Facultad de <nombre> <ciudad>`, conservando el número (01, 02, etc.) cuando forma parte del nombre.
- La etiqueta de facultad se actualiza al aceptar sugerencias, cambiar la ciudad o editar la facultad.

### Corregido

- Migración de base a esquema 4 para admitir el tipo `facultad`.
- Limpieza de ciudades duplicadas en convenios existentes.

## [1.1.3] - 2026-10-23

### Agregado

- Campos de firmante y cargo de la dependencia y de la UAGro.
- Campo de facultad/unidad académica y etiqueta automática de facultad.
- Búsqueda web opcional (DuckDuckGo) para confirmar ubicación cuando faltan estado o ciudad.
- Etiquetas de ciudad en la clasificación de convenios.

### Corregido

- La extracción automática ahora se ejecuta también al terminar una transcripción manual.
- Los campos de firmantes y facultad se guardan con la extracción IA.
- Límite de ciudades ampliado de 3 a 5 en las sugerencias de IA.

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
