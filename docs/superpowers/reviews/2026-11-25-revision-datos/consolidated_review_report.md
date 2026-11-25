# Revisión consolidada: revisión puntual de los datos extraídos por la IA

Documento: `docs/superpowers/specs/2026-11-25-revision-puntual-fechas-design.md` (142 líneas).
Modo: de corrido, etapas 5 a 8. La etapa 9 no aplica: el documento no tiene plan de implementación.
Reportes por etapa: `zoom_out_report.md` (5), `comprehension_report.md` (6), `critic_report.md` (7), `intermediate_zoom_out_report.md` (8).
Revisores fallidos: ninguno.

## Conclusión

La idea se sostiene: una vista con PDF.js (opción B) que muestra la página junto a la ficha es lo que se pidió, y ningún revisor la cuestionó. Lo que no se sostiene es **cómo se busca**. Buscar el valor capturado confirma el error en vez de mostrarlo: si la IA tomó la fecha de un nombramiento, la búsqueda encuentra esa fecha y la resalta. Lo mismo pasa con los campos que la IA elige de una lista o deduce. La salida más simple es que la vista **enseñe la fuente** (el cierre, la cláusula) y deje la comparación al usuario, y recortar la primera entrega a lo que originó el pedido: las fechas.

## Acciones priorizadas

### 🔴 Bloqueantes

1. **Fechas: enseñar la fuente, no buscar el valor.** «Fecha de firma» ubica el cierre con un patrón local, desde el final, etiquetado «posible cierre». «Fecha de vigencia» y «Cláusula de vigencia» buscan `vigencia_texto`. Se elimina la columna `fecha_firma_texto` y la migración al esquema 8 (sin vuelta atrás); sirve para los 120 convenios actuales.
2. **Recortar la primera entrega.** Entra la sección Vigencia. Opcional: firmantes y dependencia (partida por «; »), solo con coincidencia exacta. Salen a No objetivos: cargos, siglas, tipo de documento, facultad y ciudad de la facultad.

### 🟡 Importantes

3. **Dos estados, sin escala de confianza.** Recuadro solo con coincidencia completa, exacta y por palabra completa en página con texto. Todo lo demás, sin recuadro y con «No se encontró … en el texto del documento». Sin variantes de nombre. La cláusula admite una ventana larga de palabras seguidas.
4. **Páginas transcritas: solo la página probable.** La transcripción ya está reescrita con lo que eligió la IA, así que una coincidencia ahí no prueba nada. Aviso «según la transcripción de la IA; compare con la imagen», nunca recuadro. Usar `GetTranscription` partido por `[Página N]`. Corregir la línea 50.
5. **Regla de seguridad del recuadro.** Solo si la coincidencia corresponde sin ambigüedad a fragmentos contiguos de PDF.js (los bloques de firmas a dos columnas caen fuera). El algoritmo queda para quien implemente.
6. **Lógica en JS, verificación manual explícita.** Reutilizar `normalize()`. Plan con una docena de convenios reales (con texto, escaneados, mixtos) y criterio «cero recuadros sobre un lugar equivocado». Quitar la decisión JS/Go.
7. **Registrar la decisión del usuario** antes de reescribir: enfoque B y alcance de la primera entrega.

### 🟢 Menores

8. **Rendimiento sencillo.** Leer el texto una vez por ficha, recorrer desde el final para el cierre, destruir el documento de PDF.js al cerrar. «Buscando…» mientras tanto.
9. **Ciclo de vida.** Cualquier redibujo de la ficha, navegación, transcripción o reanálisis cierra la vista y regresa al visor de Edge.
10. **Botón accesible.** `type="button"`, fuera del `<label>`, con `title` y `aria-label`.
11. **Rehacer el boceto** con el cierre y «posible cierre», sección y etiqueta coherentes, marco alineado.
12. **Riesgos en «Lo que haría fracasar».** Escapar con `esc()` el texto transcrito que se pinta; falsos positivos. Una frase sobre la marca de verificado futura (el diseño no guarda estado).
13. **Redacción sobre el texto que sobreviva.** `#page` dicho de una sola forma; vocabulario fijo («fragmento», «recuadro»); «hacer clic» y «quien implemente»; comillas uniformes; explicar proemio y cierre; quitar afirmaciones sin respaldo y repeticiones; precisar que la IA sí ve marcas de página en las transcritas; decir cómo se cambia de página para buscar a mano.

## Rechazados (con razón)

- **Guardar la lectura original de la transcripción** (catalizador 1): otra migración y texto duplicado, sin recuperar los convenios actuales. Lo cubre la acción 4.
- **Tabla de origen por campo y alias de facultad** (catalizador 3): complejidad para cuatro campos que salen del alcance (acción 2).
- **Comparar la cita con el campo y fechas en palabras** (catalizador 2, D3, D11): dejan de hacer falta con la acción 1.
- **Pasar la lógica a Go** (D10 parcial, zoom-out 5): sin fechas en palabras ni variantes, no justifica un método nuevo.
- **Guardar `dudas` y correcciones como JSON** (catalizador 4): fuera de alcance.
- **Mediciones, cancelación y búsqueda incremental** (D7, catalizador 5): optimización prematura para convenios de pocas decenas de páginas.
- **Sección nueva de riesgos** (S1, D17): basta con ampliar «Lo que haría fracasar».

## Matriz de trazabilidad

| Hallazgo | Etapa | Acción |
| --- | --- | --- |
| Zoom-out 1 | 5 | 2 |
| Zoom-out 2 | 5 | 1 |
| Zoom-out 3 | 5 | 4 |
| Zoom-out 4 | 5 | 1 |
| Zoom-out 5 | 5 | 6 |
| Zoom-out 6 | 5 | 4 |
| Zoom-out 7 | 5 | 3 |
| Zoom-out 8 | 5 | 6 |
| Zoom-out 9 | 5 | 5 |
| Catalizador 1 | 5 | 4 (rechazado) |
| Catalizador 2 | 5 | 1 (rechazado) |
| Catalizador 3 | 5 | 2 (rechazado) |
| Catalizador 4 | 5 | 12 (rechazado) |
| Catalizador 5 | 5 | 8 (rechazado) |
| Comprensión 1 | 6 | 11 |
| Comprensión 2 | 6 | 3 |
| Comprensión 3 | 6 | 5 |
| Comprensión 4 | 6 | 2 |
| Comprensión 5 | 6 | 4 |
| Comprensión 6 | 6 | 1 |
| Comprensión 7 | 6 | 4 |
| Comprensión 8 | 6 | 13 |
| Comprensión 9 | 6 | 13 |
| Comprensión 10 | 6 | 1, 8 |
| Comprensión 11 | 6 | 12 |
| Comprensión 12 | 6 | 13 |
| D1 | 7 | 2 |
| D2 | 7 | 1 |
| D3 | 7 | 1 (rechazado) |
| D4 | 7 | 1, 6 |
| D5 | 7 | 5 |
| D6 | 7 | 4 |
| D7 | 7 | 8 |
| D8 | 7 | 9 |
| D9 | 7 | 10 |
| D10 | 7 | 6 |
| D11 | 7 | 1 (rechazado) |
| D12 | 7 | 3 |
| D13 | 7 | 3, 11 |
| D14 | 7 | 6 |
| D15 | 7 | 13 |
| D16 | 7 | 7 |
| D17 | 7 | 12 |
| D18 | 7 | 1 |
| S1 | 7 | 12 |
| S2 | 7 | 7 |
| S3 | 7 | 2 |
| S4 | 7 | 2, 3 |
| S5 | 7 | 11 |
| S6 | 7 | 11 |
| S7 | 7 | 13 |
| S8 | 7 | 13 |
| S9 | 7 | 4 |
| S10 | 7 | 13 |
| S11 | 7 | 6 |
| S12 | 7 | 6 |
| S13 | 7 | 13 |
| E1 | 7 | 3 |
| E2 | 7 | 3 |
| E3 | 7 | 2 |
| E4 | 7 | 11 |
| E5-E22 | 7 | 13 (E9: 8; E13: 12; E14: 2; E17: 3; E18: 6; E22: 11) |
| Intermedia 1 | 8 | 1 |
| Intermedia 2 | 8 | 2 |
| Intermedia 3 | 8 | 3 |
| Intermedia 4 | 8 | 4 |
| Intermedia 5 | 8 | 5 |
| Intermedia 6 | 8 | 6 |
| Intermedia 7 | 8 | 7 |
| Intermedia 8 | 8 | 8 |
| Intermedia 9 | 8 | 9 |
| Intermedia 10 | 8 | 10 |
| Intermedia 11 | 8 | 11 |
| Intermedia 12 | 8 | 12, 13 |
