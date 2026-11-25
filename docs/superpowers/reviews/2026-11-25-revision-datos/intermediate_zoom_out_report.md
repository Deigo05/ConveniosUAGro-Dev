# Etapa 8: simplificación intermedia

Documento: `docs/superpowers/specs/2026-11-25-revision-puntual-fechas-design.md` (líneas 1-142, sin corregir). Revisor: `egm-zoom-out` (estándar zoom-out), con los reportes de las etapas 5, 6 y 7 como contexto. Tarea: separar lo que vale la pena de lo que sobra para una app de un solo usuario.

1. **Blocker. Fechas: mostrar la fuente, no confirmar el valor** (47-48, 78-81, 93-94, 129, 141; D2, D3, D4, D11, D18, zoom-out 2 y 4, catalizador 2, comprensión 6). Buscar la fecha capturada encuentra la del nombramiento si la IA se equivocó, y la presenta como buena. En vez de agregar maquinaria (columna, migración, comparar la cita, fechas en palabras), cambiar la pregunta:
   - «Fecha de firma» ubica el cierre con un patrón local («días del mes de», «firman», «firmado en»), desde la última página, con la etiqueta «posible cierre».
   - «Fecha de vigencia» y «Cláusula de vigencia» buscan siempre `vigencia_texto`.
   - El usuario compara, con el campo a la vista.
   - Se quitan la columna `fecha_firma_texto`, la migración al esquema 8, las formas de fecha, la decisión JS/Go para fechas y la dependencia de `vigencia_calculada`. Sirve para los 120 convenios. Rechaza D3, D11 y catalizador 2.
2. **Blocker. Recortar la primera entrega** (9, 15-24, 29, 92, 98; D1, zoom-out 1, S3, E3, comprensión 4, catalizador 3). Primera entrega: Vigencia. Opcional en la misma: los dos firmantes y la dependencia (partida por «; »), solo con búsqueda exacta. Fuera: cargos, siglas, tipo de documento, facultad y ciudad de la facultad, que pasan a No objetivos. Rechaza la tabla de origen y los alias del catalizador 3.
3. **Major. Solo dos estados** (40, 90, 95, 97, 100; comprensión 2, E2, S4, D12, D13, zoom-out 7). Con recuadro: coincidencia completa, exacta, por palabra completa, en página con texto. Sin recuadro: todo lo demás, con «No se encontró … en el texto del documento» (acepta E1). Sin variantes de nombre. Excepción: la cláusula, con una ventana larga de palabras seguidas.
4. **Major. Páginas transcritas: solo página probable** (50, 53, 96, 100; zoom-out 3 y 6, D6, comprensión 5 y 7, catalizador 1). Aviso «según la transcripción de la IA; compare con la imagen», nunca recuadro. Reutilizar `GetTranscription` partiendo por `[Página N]`. Corregir la línea 50. Rechaza el catalizador 1.
5. **Major. Una regla de seguridad para el recuadro** (64, 86-100, 137-141; D5, comprensión 3, zoom-out 9). Se dibuja solo si la coincidencia corresponde sin ambigüedad a fragmentos contiguos de PDF.js; si no, sin recuadro. El algoritmo queda para quien implemente.
6. **Major. Todo en JS, con verificación manual** (135, 141; D10, zoom-out 5, S11, S12). Con el recorte ya no hay lógica que justifique un método nuevo en Go. Plan manual con una docena de convenios reales (con texto y escaneados) y el criterio «cero recuadros sobre un lugar equivocado». Quitar la línea 141.
7. **Major. Registrar la decisión** (56-76; S2, D16). Preguntar al usuario: B, con primera entrega de Vigencia y firmantes y dependencia como opción.
8. **Nit. Rendimiento** (134; D7, catalizador 5). Leer el texto una vez por ficha, recorrer desde el final para el cierre, destruir PDF.js al cerrar. Rechaza mediciones y cancelación.
9. **Nit. Ciclo de vida** (122-125; D8). Cualquier `renderDetail`, navegación, transcripción o reanálisis cierra la vista.
10. **Nit. Botón accesible** (43, 123; D9). Aceptar: `type="button"`, fuera del `<label>`, con `aria-label`.
11. **Nit. Boceto** (104-120; comprensión 1, E4, S5, S6). Rehacerlo con el cierre y «posible cierre».
12. **Nit. Lo que desaparece con el recorte** (S1, D17, catalizador 4, comprensión 6, 10-12, E5-E22, S7-S13). Rechazar guardar `dudas`. Sin sección nueva de riesgos: agregar a «Lo que haría fracasar» el escape con `esc()` y los falsos positivos. Una frase sobre la marca futura. Aplicar la redacción solo al texto que sobreviva.
