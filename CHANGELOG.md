# Changelog

## [1.2.0] - 2026-10-24

### Agregado

- **Nombres de archivo útiles**: aunque el PDF llegue como «Escaner123515.pdf», se descarga como `AAAA-MM-DD Tipo - Contraparte - Facultad.pdf` (por ejemplo `2024-03-15 Convenio Específico - IMSS - Facultad de Medicina Acapulco.pdf`). La IA identifica el tipo de documento y las siglas de la contraparte; ambos se pueden corregir en la ficha y el nombre se actualiza solo. El nombre original se conserva.
- **Aviso de duplicados al importar**: antes de subir, la app marca los PDF que ya están registrados o que se eligieron dos veces y no los importa, salvo que se indique «Importar de todos modos».
- **Marca de posible duplicado** en la lista y en la ficha mientras exista el otro documento; desaparece sola al borrar el duplicado o al marcar el par como distinto. Detecta el mismo archivo, el mismo contenido (otro escaneo, tolerando errores de lectura) y los mismos datos (contraparte, fecha, tipo y facultad).
- **Herramienta «Duplicados»** en el menú: muestra cada par lado a lado con sus datos para abrir, descargar o eliminar uno, marcarlos como distintos o unirlos como versiones.
- **Historial de versiones**: cuando un convenio renueva a otro, el anterior pasa a su historial y la lista muestra solo la versión más reciente (con la marca «N versiones»). La IA detecta si el documento declara que renueva un convenio anterior y lo une sola; las demás coincidencias (misma contraparte, tipo y facultad con otra fecha) se proponen como «posible renovación». La ficha permite agregar o quitar versiones a mano, y si se borra la vigente, la anterior vuelve a mostrarse.
- Descarga de varios PDF a la vez a una carpeta, cada uno con su nombre útil, sin sobrescribir archivos existentes.
- **Actualizaciones desde GitHub**: la app revisa al iniciar (y cada 6 horas) si hay un release nuevo compilado desde `main`, muestra un aviso con las novedades y permite actualizar con un clic. Antes de reemplazar el ejecutable guarda una copia de la base en `respaldos\antes-de-actualizar-….db`, verifica el SHA-256 de la descarga y reinicia la aplicación. Se puede desactivar en Ajustes.
- Flujo de GitHub Actions (`.github/workflows/release.yml`) que prueba, compila y publica el release `v<VERSION>` en cada push a `main`.
- **Exportar e importar convenios**: paquete `.zip` con datos, etiquetas, transcripción y PDF (todos, o solo los seleccionados). La importación también acepta un respaldo ZIP o un `convenios.db` de otra instalación; no borra nada y omite los PDF que ya existen.
- Exportación a CSV (compatible con Excel) de la selección, de los resultados filtrados o de toda la base.
- Etiqueta de facultad **«Convenio Abierto»** para los convenios que no indican con qué facultad o unidad académica de la UAGro se firmaron. Se asigna sola y se retira cuando se captura una facultad.
- La IA detecta nombres con posibles errores de lectura en la transcripción (por ejemplo «García» y «Garay» para la misma persona) y los unifica con la forma correcta; la ficha deja nota del cambio.
- Nivel de análisis de la IA en Ajustes: **Rápido** (predeterminado), **Detallado** y **Máximo**.
- **Marca «¿No es convenio?»**: la IA indica si el archivo es otro tipo de documento (acta, oficio, constancia…). En la lista lleva un borde violeta y hay un filtro «No son convenios»; en la ficha se puede confirmar que sí es convenio o eliminarlo, y también marcarlo a mano.
- **Comparar PDFs en Duplicados**: el botón «Comparar PDFs» abre los dos documentos del par lado a lado en la misma pantalla.
- **Ciudad ligada a su estado**: cada ciudad guarda su estado. No se puede guardar una ciudad con el estado de otra (por ejemplo, Acapulco de Juárez con Veracruz) ni un estado de México con «Internacional». Al elegir una ciudad se marcan su estado y «Nacional»; las ciudades de otro estado aparecen atenuadas. El catálogo de etiquetas muestra y permite editar el estado de cada ciudad.
- **Etiquetas obligatorias**: todo convenio analizado lleva ámbito («Nacional» si tiene un estado de México) y la etiqueta de su facultad o «Convenio Abierto». Al abrir la app se corrigen una sola vez los convenios ya analizados.
- La búsqueda web de ubicación también se usa cuando la ciudad sería nueva o el domicilio se leyó con dudas, con el nombre de la contraparte y el domicilio del documento (por ejemplo, «Huachinango, Pue.» → Huauchinango, Puebla).
- Arrastrar y soltar PDF, paquetes o respaldos sobre la ventana para importarlos.
- Fragmento del texto donde aparece la búsqueda, con la coincidencia resaltada.
- Las tarjetas de resumen filtran la lista (vigentes, por vencer, vencidos) y hay filtro «Por vencer».
- Buscadores dentro del filtro de etiquetas y del catálogo de etiquetas.
- Aviso de cambios sin guardar en la ficha, atajo Ctrl+S y confirmación al salir.

### Cambiado

- Búsqueda más útil: incluye firmantes, cargos, facultad, vigencia y nombres de etiquetas; ordena por relevancia al escribir; acepta frases entre comillas y, si ninguna ficha tiene todas las palabras, muestra coincidencias aproximadas.
- Instrucciones de la IA más cortas y precisas (menos tokens): texto compactado, catálogo reducido, esfuerzo bajo para visión y búsqueda de ubicación.
- La transcripción marca con `[?]` las letras dudosas y lee los nombres letra por letra.
- Si se importa un PDF repetido, se copian los datos y la transcripción del original en vez de volver a pagar el análisis.
- Ficha reorganizada en secciones (Contraparte, UAGro, Vigencia, Notas). El visor de PDF ocupa la altura de los datos y ya no se encima con otras secciones al desplazarse.
- La lista muestra firmantes y el tiempo restante de vigencia en palabras: «Vence en 1 año y 7 meses», «Venció hace 2 años y 8 meses», «Vence mañana».
- **Procesamiento más rápido**: la cola automática procesa 3 archivos a la vez y hasta 10 páginas escaneadas en paralelo entre todos (antes, un archivo y 4 páginas). El costo por documento no cambia.
- El nivel predeterminado pasa a «Rápido» (`reasoning_effort` bajo): en las pruebas la extracción tardó unas 2.7 veces menos y gastó menos tokens con los mismos datos principales. «Detallado» y «Máximo» equivalen a `high` y `max` en DeepSeek.
- Reintentos automáticos (3 s y 10 s) cuando el servicio de IA responde 429, un error 5xx o falla la red.
- Catálogo de facultades y sedes ampliado con los nombres oficiales: «Unidad Académica de Turismo» se etiqueta como «Facultad de Turismo Acapulco» y «EMVZ No. 3» como «Escuela Superior de Medicina Veterinaria y Zootecnia 03» (Tecpan de Galeana). Las etiquetas mal formadas de versiones anteriores se corrigen al abrir la app.
- La búsqueda web espacia las consultas y se pausa 15 minutos si DuckDuckGo pide verificación anti-robots.
- Avisos con botón para cerrarlos; los errores permanecen más tiempo.
- Ajustes muestra versión, compilación y ubicación del ejecutable.

### Técnico

- Esquema v6: columnas `dependencia_corta`, `tipo_documento`, `busqueda`, `huella` (SimHash del texto) y `reemplazado_por` (historial), tablas `convenio_duplicados` y `duplicados_descartados`, e índice FTS5 propio (`search.sql`) que se reconstruye solo si hace falta. Las bases anteriores se migran al abrirlas.
- Columnas `convenios.no_es_convenio` (0 normal, 1 marcado, 2 confirmado por el usuario; la IA no cambia un 2) y `etiquetas.estado` (estado de cada ciudad), y tabla `meta` para reparaciones que se ejecutan una sola vez.

## [1.1.6] - 2026-10-23

### Cambiado

- Al importar uno o varios PDF, la app permanece en la pantalla principal de convenios; ya no abre automáticamente la ficha ni la pantalla de transcripción.
- Las etiquetas sugeridas por la IA se aplican automáticamente, sin aprobación del usuario.
- La pantalla principal muestra un aviso de procesamiento y el estado por convenio (en cola, transcribiendo, analizando).
- La lista se actualiza automáticamente mientras hay transcripciones o análisis en curso.
- La ficha sigue disponible al pulsar un convenio o el botón Abrir.

### Corregido

- Se aplican automáticamente sugerencias pendientes de versiones anteriores.

## [1.1.5] - 2026-10-23

### Corregido

- La etiqueta de facultad ya no usa la ciudad de la dependencia. Ahora usa la ciudad/sede de la facultad de la UAGro.
- La extracción devuelve `facultad_ciudad` por separado y la ficha permite corregirla.
- Se agregó un catálogo de facultades y sedes de la UAGro para validar y normalizar nombres.
- Se reparan automáticamente etiquetas antiguas como “Facultad de Turismo Tijuana” a “Facultad de Turismo Acapulco”.
- Migración de base al esquema 5 con `facultad_ciudad`.

## [1.1.4] - 2026-10-23

### Cambiado

- Solo se permite **una etiqueta de ciudad** por convenio. La IA sugiere como máximo una y debe ser la ciudad de la dependencia, no la de la UAGro.
- Las etiquetas de facultad ahora son un **tipo propio** (`facultad`), separado de Tema / Rubro.
- El nombre de la etiqueta de facultad sigue el formato `Facultad de <nombre> <ciudad>`, conservando el número (01, 02, etc.) cuando forma parte del nombre.
- La etiqueta de facultad se actualiza al aceptar sugerencias, cambiar la ciudad de la facultad o editar la facultad.

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
