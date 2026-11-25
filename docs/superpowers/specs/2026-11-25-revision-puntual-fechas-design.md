# Revisión puntual de la vigencia en el documento

Estado: diseño para aprobación (2026-11-25). Revisado con EGM: [reporte consolidado](../reviews/2026-11-25-revision-datos/consolidated_review_report.md). Contexto del sistema: [EGM.md](../../../EGM.md), [interfaz](../../../frontend/dist/EGM.md), [IA](../../../internal/ai/EGM.md) y [persistencia](../../../internal/store/EGM.md).

## Intención

El análisis con IA (DeepSeek) a veces captura una fecha de firma o una vigencia que no coincide con el documento: el usuario lo ha visto varias veces. Hoy, para comprobarlo, tiene que abrir el PDF y buscar a mano entre varias páginas.

Se quiere que, desde la ficha, el usuario vea **la página y el fragmento del documento** de donde sale la vigencia, lo compare con lo capturado y lo corrija ahí mismo.

La app la usa una sola persona, en una PC, con unos 120 convenios hoy y una meta de al menos 1000.

## Objetivos

- Junto a tres campos de la sección Vigencia de la ficha, un botón «Ver en el documento»: **Fecha de firma**, **Fecha de vigencia** y **Cláusula de vigencia**.
- Al hacer clic se ve, sin salir de la ficha, la página del documento con un recuadro sobre el fragmento. Los campos siguen a la vista para corregirlos.
- Si hay varios fragmentos posibles, se puede pasar de uno a otro.
- Funciona con los convenios ya analizados, sin volver a analizarlos con IA y sin cambiar la base de datos.

## No objetivos

- Los demás campos de la ficha (dependencia, siglas, firmantes, cargos, tipo de documento, facultad, ciudad de la facultad). Quedan para una entrega posterior si esta funciona. Varios de ellos la IA los elige de una lista o los deduce, y buscarlos en el texto daría coincidencias engañosas («Acuerdo» en «de común acuerdo»).
- Las etiquetas y las notas de la IA.
- Revisión sistemática: no hay marca de «verificado» ni lista de pendientes. Esta vista no guarda ningún estado, así que no estorba agregar esa marca después.
- Mejorar la lectura de la IA (prompt o modelo).
- Confirmar que el dato capturado es correcto. La vista **enseña la fuente**; quien compara es el usuario.

## Lo que haría fracasar el cambio aunque funcione

- Que dibuje un recuadro sobre un lugar equivocado con aparente seguridad. Es peor que no dibujar nada, porque confirma un dato mal leído.
- Que para usarlo haya que volver a analizar con IA los convenios existentes.
- Que la ficha se vuelva más lenta de abrir: todo corre solo al hacer clic.
- Que la ficha se llene de botones: el botón es un ícono discreto junto a la etiqueta.
- Que el texto del PDF o de la transcripción se pinte sin escapar: todo pasa por `esc()` o `textContent`.

## Cómo es un convenio (lo que el diseño aprovecha)

- Un convenio tiene proemio (quiénes firman), declaraciones, cláusulas y **cierre**. El cierre va al final, antes de las firmas, y dice dónde y cuándo se firmó, casi siempre en palabras: «…lo firman por duplicado en Chilpancingo, Guerrero, a los quince días del mes de marzo de dos mil veinticuatro». Las fechas del principio suelen ser de nombramientos o publicaciones, que es justo lo que a veces confunde a la IA.
- La vigencia está en una cláusula propia. La IA ya devuelve su texto literal y la app lo guarda como «Cláusula de vigencia» (`vigencia_texto`, editable).
- Cuando la vigencia es un plazo («tres años»), la fecha de vigencia se calcula a partir de la firma y no aparece escrita en el documento.

## Situación actual

- La interfaz ya trae PDF.js (`vendor/pdf.min.js`) y lo usa en `transcribeConvention` y `renderPageJPEG`. PDF.js da el texto de cada página con la posición de cada fragmento.
- El PDF de la ficha se ve en un `iframe` (`#pdf-frame`) con el visor de Edge (WebView2), que no permite resaltar.
- Una página con menos de 30 caracteres en PDF.js se considera escaneada (`transcribeConvention`). Esas páginas, si el usuario las transcribió, tienen su texto guardado por página. `GetTranscription` lo devuelve unido, con bloques `[Página N]`.
- Ese texto transcrito lo escribió la misma IA, y `ApplyNameCorrections` y `ApplyPageCorrections` lo reescriben después con las lecturas que ella eligió. No es evidencia independiente.
- `normalize()` (`app.js`) ya quita acentos y pasa a minúsculas.

## Enfoques considerados

**Cómo se muestra.**
- A. Saltar a la página en el visor de Edge: no resalta, y no está comprobado que WebView2 respete `#page=N`.
- **B. Vista de revisión dibujada con PDF.js, con recuadro (elegida).** Es lo que se pidió y reutiliza PDF.js.
- C. Recorte del fragmento como imagen: pierde el contexto (una fecha tachada o una nota al margen quedan fuera).

**Qué se busca.**
- Buscar el valor capturado: si la IA tomó la fecha de un nombramiento, la búsqueda encuentra esa misma fecha y la resalta como buena. Confirma el error en vez de mostrarlo. Descartado.
- Pedir a la IA cita y página de cada dato: solo sirve para análisis nuevos, cuesta más, y la página que diga puede estar mal. Descartado.
- **Enseñar la fuente (elegido).** Para la firma se ubica el cierre; para la vigencia, la cláusula guardada. No depende de lo que capturó la IA y sirve para todos los convenios.

**Alcance.** El usuario eligió entregar primero solo la vigencia (2026-11-25). La opción B fue la recomendada y no se objetó.

## Diseño

### Qué busca cada botón

- **Fecha de firma → el cierre.** Se buscan las fórmulas del cierre («días del mes de», «lo firman», «firmado en», «se firma», «para constancia», y similares) y la anotación de firmas («las firmas que anteceden…»), recorriendo las páginas **desde la última hacia atrás**. Solo cuenta como candidato una oración que tenga la fórmula **y** una fecha (un mes escrito o una fecha en números). El primero que se muestra es el más cercano al final, y el contador («1 de 3») deja ver que hay otros. La etiqueta es «posible cierre». El recuadro abarca la oración desde la fórmula.
  - Una fórmula con fecha que no es el cierre (un nombramiento, un anexo pegado al final) puede salir como candidato. Es aceptable: es texto real con esa forma, está etiquetado como «posible» y el usuario ve la página entera. Por eso mismo, un cierre tachado se distingue en la imagen aunque PDF.js no vea el tachado.
- **Fecha de vigencia y Cláusula de vigencia → la cláusula guardada** (`vigencia_texto`). Las dos muestran lo mismo: si la vigencia es un plazo, ahí está el plazo; si es una fecha, ahí está la fecha. Se busca el texto completo de la cláusula en cada página. Como la IA la recorta (pide hasta 300 caracteres y Go guarda hasta 500) y la cláusula puede cruzar de página, lo normal será una coincidencia parcial: el tramo más largo de palabras seguidas que aparezca en una página, siempre que sea largo y único en el documento. Ese tramo **sí lleva recuadro**, solo sobre lo encontrado. Etiquetas: «cláusula completa» o «parte de la cláusula».
  - Si `vigencia_texto` está vacío: «No hay cláusula de vigencia guardada», y la vista abre en la primera página para buscar a mano.

### Normalización

El texto buscado y el de cada página se comparan normalizados del mismo modo: `normalize()` más espacios unificados, guiones de corte de línea unidos y signos de puntuación cambiados por espacio. Se compara por palabras completas.

### Dos estados, nada intermedio

- **Con recuadro:** lo buscado (la oración del cierre, la cláusula completa o su tramo largo y único) aparece entero y corresponde, sin ambigüedad, a fragmentos contiguos de PDF.js en una página con texto (30 caracteres o más). Si cruza un salto de línea, se dibuja un recuadro por renglón.
- **Sin recuadro:** todo lo demás. La vista muestra la página probable (o la primera) y un aviso. Ejemplos: «No se encontró la cláusula en el texto del documento»; «No se encontró el cierre en el texto del documento». Nunca un recuadro aproximado: un bloque que PDF.js entrega revuelto (por ejemplo, firmas a dos columnas) cae aquí.

### Páginas escaneadas

- Si la página no tiene texto en PDF.js pero sí transcripción, se busca en la transcripción de esa página (partiendo el texto de `GetTranscription` por los bloques `[Página N]`). Si aparece, se muestra la página **sin recuadro**, con el aviso «Según la transcripción de la IA; compare con la imagen» y, arriba de la página, la frase transcrita.
- Si la página no tiene texto ni transcripción, no se puede buscar en ella. Si eso impide el resultado, el aviso lo dice: «Hay N páginas escaneadas sin transcribir».
- El orden de búsqueda es el mismo para todas las páginas (desde el final para la firma, desde el principio para la cláusula), use cada una PDF.js o transcripción.

### Pantalla

```text
┌ Ficha del convenio ─────────────────────────────────────────────────┐
│ Documento original                  │ Datos del convenio            │
│ ┌─────────────────────────────────┐ │ Vigencia                      │
│ │ Revisando: Fecha de firma       │ │ Fecha de firma ⌕ ◂            │
│ │ Página 8 de 8 · posible cierre  │ │ [15/03/2024]                  │
│ │ ‹ anterior   1 de 2  siguiente ›│ │ Fecha de vigencia ⌕           │
│ │ ┌─────────────────────────────┐ │ │ [15/03/2027]                  │
│ │ │ … página dibujada …         │ │ │ Cláusula de vigencia ⌕        │
│ │ │ ┏━━━━━━━━━━━━━━━━━━━━━━━━━┓ │ │ │ [El presente convenio …]      │
│ │ │ ┃ …a los quince días del  ┃ │ │ │                               │
│ │ │ ┃ mes de marzo de dos mil ┃ │ │ │                               │
│ │ │ ┃ veinticuatro.           ┃ │ │ │                               │
│ │ │ ┗━━━━━━━━━━━━━━━━━━━━━━━━━┛ │ │ │                               │
│ │ └─────────────────────────────┘ │ │                               │
│ │ página ‹ 8 › · zoom − +         │ │ ⌕ = Ver en el documento       │
│ │ [Volver al PDF completo]        │ │ ◂ = campo en revisión         │
│ └─────────────────────────────────┘ │                               │
└─────────────────────────────────────────────────────────────────────┘
```

- La vista reemplaza temporalmente el visor de Edge en la columna «Documento original». El `iframe` se oculta, no se recarga. «Volver al PDF completo» lo muestra de nuevo.
- Encabezado: qué campo se revisa, página y etiqueta, y «anterior / siguiente» entre fragmentos candidatos («1 de 2»).
- Abajo: cambiar de página a mano (para buscar cuando no hubo resultado) y zoom (más / menos). La página abre ajustada al ancho de la columna y con el recuadro a la vista.
- El campo en revisión se marca en el formulario. Con la vista abierta, hacer clic en el ícono de otro campo cambia la búsqueda sin cerrarla.
- El botón es un `<button type="button">` con `title` y `aria-label` («Ver en el documento: Fecha de firma»), **fuera** del `<label>` del campo, para no cambiar el nombre accesible del campo ni enviar el formulario.

### Ciclo de vida

- El documento de PDF.js se abre al primer clic y el texto de cada página se lee una sola vez mientras la vista siga abierta. Mientras se lee, «Buscando…».
- La vista se cierra, y el documento de PDF.js se destruye, al hacer clic en «Volver al PDF completo», al abrir otro convenio o salir de la ficha, y cuando termina una transcripción o un análisis del mismo convenio (el texto pudo cambiar).
- Si la ficha se vuelve a dibujar sin cambiar de convenio (cualquier llamada a `renderDetail()` con el mismo convenio), la vista sigue abierta.

## Restricciones

- Sin red ni IA: todo se hace con el PDF y los textos ya guardados.
- Sin cambios en la base de datos ni en los métodos publicados de Go: solo `frontend/dist`. Se usan `GetTranscription` y el PDF que ya sirve `/pdf/<id>`.
- PDFs de hasta `MaxPages` (2000) páginas. Para el cierre, el recorrido empieza por el final y muestra el primer resultado en cuanto aparece. No se miden ni optimizan PDF muy largos en esta entrega: un convenio típico tiene unas decenas de páginas.

## Verificación

La interfaz no tiene pruebas automatizadas, así que se verifica a mano:

- Durante el desarrollo, en el entorno de prueba que ya se usó para las versiones anteriores: la interfaz servida localmente con los métodos de Go simulados (fuera del repositorio, solo para desarrollo). Hay que ampliarlo para que simule `GetTranscription` y sirva `/pdf/<id>`, y preparar PDFs de muestra (tampoco van al repositorio): con texto, escaneado transcrito, escaneado sin transcribir, mixto, cierre con fecha en palabras, nombramiento con fecha antes del cierre, cláusula con plazo, cláusula que cruza de página y sin cláusula guardada. Con capturas de playwright.
- En la PC del usuario, con unos 12 convenios reales de esos tipos.
- Criterio de aceptación: **cero recuadros sobre un lugar equivocado**. «Equivocado» es un recuadro sobre texto que no es lo que dice su etiqueta: «posible cierre» sobre algo que no tiene fórmula y fecha, o «parte de la cláusula» sobre otra cláusula. Un resultado sin recuadro es aceptable; un candidato real que no es el cierre, etiquetado como «posible», también.

## Lo que queda a quien implemente

- La lista exacta de fórmulas del cierre y el largo mínimo del tramo de la cláusula.
- Cómo se pasa de la coincidencia normalizada a los fragmentos de PDF.js y a los recuadros, respetando la regla de los dos estados.
- La forma del recuadro, del ícono y de los controles, siguiendo el estilo actual de la ficha.
