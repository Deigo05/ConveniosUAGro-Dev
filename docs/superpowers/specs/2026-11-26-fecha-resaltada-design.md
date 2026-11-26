# Fecha resaltada dentro del recuadro (versión 1.4.1)

Diseño corto, aprobado por el usuario el 2026-11-26. Amplía `2026-11-25-revision-puntual-fechas-design.md`.

## Intención

En la 1.4.0, «Ver en el documento» encierra toda la oración del cierre (hasta 80 palabras) o toda la cláusula de vigencia. La fecha queda en algún punto de ese bloque y el usuario tiene que buscarla. El usuario pidió que se vea resaltada la posición específica de la fecha.

## Qué cambia

- **Dos niveles de marca.** El recuadro actual se queda como contexto, más tenue cuando hay marcas. Encima, un marcador sólido amarillo, como de resaltador, va sobre cada fecha o plazo encontrado *dentro* de ese recuadro.
- **Fecha de firma:** se marcan las fechas del cierre. Cuentan con año y escritas con letra, por ejemplo «25 de noviembre de 2025», «a los veinticinco (25) días del mes de noviembre del año dos mil veinticinco» y «25/11/2025».
- **Fecha de vigencia y cláusula:** se marcan las fechas y los plazos de la cláusula.
  - Ejemplos de plazo: «tres (3) años», «36 meses», «por tiempo indefinido».
  - Solo cuentan los plazos en años o meses, o indefinidos. Los días casi siempre son avisos de terminación («30 días naturales»), no la vigencia.
  - Un plazo seguido de «de anticipación» o «antes» no se marca.
  - Cuando la fecha de vigencia se calculó de un plazo, lo marcado es el plazo.
- **Páginas transcritas:** siguen sin recuadro. En el fragmento de texto que se muestra, la fecha o el plazo van con `<mark>`.
- **Desplazamiento:** la vista se centra en la primera marca, si la hay.

## Reglas que no cambian

- Se marca lo que dice el documento, no se busca el valor que capturó la IA. Si la IA se equivocó, el usuario debe ver la diferencia, no una confirmación falsa.
- Nunca un marcador sobre un lugar equivocado. Una marca sale solo si su texto está dentro del recuadro y `matchRects` la ubica en tramos contiguos. Si no, esa marca no se dibuja y queda solo el recuadro.
- Sin cambios en Go, la base de datos ni el análisis de la IA.

## Enfoques considerados

1. **Marcar fechas y plazos dentro del recuadro ya encontrado** (elegido). Se reutilizan `matchRects` y la búsqueda de la 1.4.0, y se respeta la regla de mostrar la fuente.
2. **Buscar en el documento el valor capturado por la IA**, en todas sus formas de escritura. Descartado: si la IA tomó una fecha equivocada, la vista la resaltaría como si fuera la correcta. Ya se había rechazado en el diseño de la 1.4.0.
3. **Achicar el recuadro hasta la fecha y quitar el contexto.** Descartado: sin la oración alrededor no se sabe si esa fecha es la de la firma o la de otro acto.

## Para quien lo construya

- `revision.js`: una función pura `findMarks(index, start, end, kind)`, con `kind` igual a `'firma'` o `'vigencia'`. Devuelve tramos `{start, end}` del texto normalizado dentro de `[start, end)`, en orden y sin traslapes; las fechas tienen prioridad sobre los plazos.
- `app.js`: `reviewCandidate` agrega `marks`, los rectángulos de cada marca, y `snippetParts` para las transcripciones. `drawReviewPage` dibuja las marcas y centra la vista en la primera.
- `styles.css`: `.review-mark` es el marcador. Con marcas, el recuadro baja de intensidad.
- La versión sube a 1.4.1, con su entrada en CHANGELOG.
