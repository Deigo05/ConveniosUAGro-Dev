# Etapa 7: crítica de diseño, estructura y redacción

Documento: `docs/superpowers/specs/2026-11-25-revision-puntual-fechas-design.md` (líneas 1-142). Modo de corrido: una sola pasada, sin ciclo de corrección.

| Revisor | Estándar | Estado |
| --- | --- | --- |
| `egm-design` | feature-spec-standard | completo: 11 Major, 7 Nit |
| `egm-structural` | feature-spec-standard | completo: 3 Major, 10 Nit |
| `egm-style` | (sin guía de estilo) | completo: 4 Major, 18 Nit |

## Diseño (`egm-design`)

- **D1 Major.** Campos con coincidencias falsas presentadas con confianza alta (17-24, 29, 40, 90-92, 98, 100):
  - «convenio general» citado dentro de un Convenio Específico;
  - Facultad normalizada al catálogo (`ai.go:293`);
  - Ciudad de la facultad que coincide con el domicilio de la contraparte;
  - «Rector» dentro de «Rectoría»;
  - Siglas que pueden ser un nombre corto.
  
  Clasificar los campos en literales y elegidos o deducidos; en los deducidos, avisar aunque haya coincidencia; buscar por palabra completa siempre; buscar el tipo de documento solo en el título.
- **D2 Major.** Buscar el valor demuestra dónde aparece, no dónde está el dato (70, 93-94, 97, 129). Una fecha de nombramiento mal tomada se encuentra y se presenta como «la fecha». Ubicar el cierre con un patrón local, interpretar su fecha y compararla con el campo.
- **D3 Major.** `fecha_firma_texto` se usa aunque ya no corresponda al campo (80-82, 93): el usuario corrigió la fecha, o la cita es el cierre tachado. Usarla solo si da la misma fecha que el campo.
- **D4 Major.** Faltan alternativas para dos decisiones (56-80): la migración al esquema 8, que no tiene vuelta atrás y solo sirve para análisis nuevos, y si la coincidencia corre en JS o en Go.
- **D5 Major.** Falta el algoritmo para pasar de la coincidencia al recuadro (51, 64, 86-100, 137-141): fragmentos cortados, columnas de firmas intercaladas, ligaduras, mapa de posiciones y varias líneas.
- **D6 Major.** Ningún método da el texto transcrito por página; usar las marcas `[Página N]` o un método nuevo (53, 96). Falta el umbral de «sin texto»; hay que buscar en toda página transcrita y escapar con `esc()`.
- **D7 Major.** `MaxPages` = 2000: «un par de segundos» no está demostrado, y guardar el texto en memoria pesa (134). Buscar página por página, poder cancelar, medir y destruir el documento de PDF.js.
- **D8 Major.** Falta el ciclo de vida de la vista (122-125): qué pasa con `renderDetail`, al navegar, al transcribir o al reanalizar con la vista abierta.
- **D9 Major.** El ícono dentro de `<label>` rompe la accesibilidad y por omisión hace `submit` (43, 123). Pedir `type="button"`, ponerlo fuera de la etiqueta y darle `aria-label`.
- **D10 Major.** La lógica riesgosa quedaría en JS sin pruebas (135, 141). Pasarla a Go con `go test`, y definir un plan manual con convenios reales y el criterio «cero resaltados equivocados presentados como texto exacto».
- **D11 Major.** Una lista fija de formas de fecha no cubre las variantes reales (93-94). Buscar por cercanía de día, mes y año.
- **D12-D18 Nit:**
  - ventana de 6 palabras demasiado genérica;
  - nombres: títulos, partículas e iniciales, y el boceto;
  - regla de puntuación (quitarla o cambiarla por espacio);
  - `#page` sin verificar y costo de tokens de la opción 2;
  - decisión pendiente;
  - falta una sección de riesgos;
  - detalles de `fecha_firma_texto`: tope de longitud, que no viaje en `ConventionInput`, `omitempty` en paquetes.

## Estructura (`egm-structural`)

- **S1 Major.** Falta una sección «Riesgos y mitigaciones»: seguridad (`esc()`), migración sin vuelta atrás, paquetes viejos, memoria y falsos positivos.
- **S2 Major.** «Diseño (B con 1)» se escribe como decidido mientras la línea 74 dice «pendiente».
- **S3 Major.** «Ciudad de la facultad» está en la tabla, pero no tiene regla de búsqueda.
- **S4-S13 Nit:**
  - un solo aviso para tres casos distintos;
  - boceto incoherente (sección y etiqueta);
  - marco descuadrado y bloque sin lenguaje;
  - `#page` dicho de dos formas;
  - la IA sí ve marcas de página en las páginas transcritas (línea 72);
  - página escaneada sin transcripción;
  - comillas mezcladas;
  - pruebas puestas como restricción;
  - JS o Go dejado al constructor, cuando cambia la API;
  - tiempos verbales en la intención.

## Redacción (`egm-style`)

- **E1 Major.** «No se encontró» se presenta como «el documento no lo dice» (70, 98). Es un indicio, no una prueba. Cambiar el mensaje a «No se encontró … en el texto del documento».
- **E2 Major.** No se define «dudosa»; las etiquetas no corresponden a todos los pasos (40, 100, 140). Hacer una tabla de estrategia, etiqueta y aviso.
- **E3 Major.** Las siglas aparecen como literales en la tabla y como deducidas en la línea 29; los avisos no sirven para todos los casos.
- **E4 Major.** El boceto contradice el algoritmo.
- **E5-E22 Nit:**
  - afirmaciones sin respaldo: «las que más fallan», «lo que confunde a la IA», «un par de segundos»;
  - vocabulario inconsistente: zona, área, fragmento, recuadro;
  - relleno («claramente»);
  - restricción puesta en No objetivos;
  - «ciudad» como etiqueta frente a «Ciudad de la facultad»;
  - título contradictorio;
  - frases demasiado cortas;
  - 6 palabras dadas como regla y como propuesta;
  - «deberían»;
  - «tamaño legible»;
  - repeticiones de «sin volver a analizar»;
  - «pulsar» y «constructor» frente a «hacer clic» y «quien implemente»;
  - marco descuadrado.
