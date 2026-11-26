# Fechas de convenios escaneados: leer mejor y avisar cuando no se puede

* **Author:** Deigo05
* **Date:** 2026-11-26

## Overview

Todos los convenios que entran a la app son PDF escaneados. Las fechas de firma y de vigencia siempre van impresas. En una parte considerable de los casos salen equivocadas, o vacías («—»), cuando la fecha está bajo un sello, sobre un fondo de poco contraste, en una página chueca o en un escaneo malo. Hoy el error solo se descubre por casualidad: en Duplicados, dos copias del mismo convenio muestran fechas distintas aunque el papel diga lo mismo.

El objetivo tiene dos partes:

1. Que la app lea mejor esas fechas.
2. Más importante: que una fecha que no se pudo comprobar se vea como «Fecha por confirmar», en lugar de quedar guardada como si fuera cierta.

La regla del usuario es que es preferible un «no sé» a una respuesta equivocada que parezca segura.

## Analysis

### Correcciones a lo que creíamos

- **La imagen sale con menos resolución de la que dice el código.** La escala se calcula como `min(1.45, 1600 / lado mayor)` (app.js:1522), y en una hoja carta siempre gana el tope de 1.45.
  - Cada página escaneada llega a la IA a unos 888 × 1149 píxeles (cerca de 104 DPI), en JPEG al 82 %.
  - La segunda revisión, que debería ir «a 2400 px», en realidad va a unos 1347 × 1743.
  - Una fecha de 10 u 11 puntos bajo un sello, a 104 DPI, coincide con la falla que se ve. La documentación (frontend/dist/EGM.md:154) dice «1600 px» y oculta el problema.
- **El guion «—» también lo produce el propio prompt.** La regla 7 dice «dato ausente o dudoso → null» (ai.go:297). Una fecha que la transcripción marcó como `[?]`, `[ilegible]` o `[Sello]` termina vacía sin generar ninguna duda.
- **Una fecha equivocada queda guardada como cierta antes de cualquier revisión.** La extracción guarda el resultado (app.go:672) y después la interfaz lanza la revisión de dudas. Las dudas viven en memoria: si la app se cierra o la revisión falla, se pierden.
- **La revisión de dudas actual no sirve para fechas.**
  - Le muestra al modelo la lectura actual y le pide confirmarla. Eso lo empuja a dar por buena la primera lectura.
  - No puede llenar un campo vacío.
  - Tiene límites de tamaño que una fecha escrita en letra (unas 12 palabras) apenas cumple.
- **Leer dos veces la misma imagen no confirma nada.** Visión corre a temperatura 0 (ai.go:592, 659): dos lecturas iguales repiten el mismo error. La segunda opinión tiene que venir de una entrada distinta: otra resolución, otro recorte, la imagen sin el color del sello u otra copia del convenio.
- **El error de fecha no se queda en la fecha.** Una firma mal leída arrastra todo esto:
  - la vigencia calculada por plazo, que se muestra como aritmética confiable;
  - el estado Vigente, Por vencer o Vencido y los contadores;
  - el nombre del PDF descargado;
  - Duplicados: con la misma contraparte y fechas distintas, el par se clasifica como «renovación» antes de revisar si el texto es casi idéntico (duplicates.go:176-178);
  - la vinculación automática de renovaciones, que ordena por fecha de firma (app.go:1423-1445). Una fecha mal leída puede mandar la copia correcta al historial.
- **Volver a analizar borra las correcciones del usuario.** El análisis sobrescribe todos los campos, incluidas las notas. Por eso cualquier medición sobre los convenios reales debe correr sobre una copia de la carpeta de datos.
- **Probar otro modelo de visión no es «cero código»** si el modelo es de otro proveedor. El modelo de análisis y el de visión comparten la misma dirección y la misma clave.

### No sabemos todavía dónde nace el error

La extracción solo ve el texto transcrito, nunca la imagen. Una fecha mal puede venir de cuatro lados:

- **(a)** La transcripción leyó mal la imagen. Solo en este caso ayuda mejorar la imagen.
- **(b)** La transcripción está bien, pero el extractor eligió otra fecha.
- **(c)** El texto se recortó en un documento largo. Es poco probable.
- **(d)** La regla 7 vació a propósito una fecha marcada como dudosa.

Abrir la transcripción guardada de los pares conocidos lo decide sin código y sin gastar API. Si domina (b), trabajar la imagen sería la solución equivocada.

### La contradicción de fondo y cómo se resuelve

La app quiere llenar la fecha sola y, al mismo tiempo, nunca mostrar como cierta una fecha no comprobada. Se resuelve separando el valor de la confianza. La IA siempre propone un valor. La confianza es otro dato, y solo la dan una comprobación independiente o el usuario.

El recurso gratuito que nadie usa es la redundancia del propio documento:

- estos convenios escriben la fecha dos veces, en letra y en número («veinticinco (25) días… dos mil veinticinco (2025)»);
- la vigencia suele citar la firma;
- la fecha de registro acota la de firma;
- las copias repiten el documento entero.

El documento se puede comprobar a sí mismo sin llamar a la IA.

**Resultado ideal:** cada fecha guardada está respaldada por dos fuentes independientes que ya existen, o aparece como «Fecha por confirmar» junto con la imagen que leyó la IA. Las páginas limpias no cuestan nada extra; el costo crece con la duda, no con el número de convenios.

### Opciones de implementación

| # | Opción | Costo de API | Esfuerzo | Punto más débil |
|---|---|---|---|---|
| A | **Corregir la resolución real**: que el límite de 1600 px funcione de verdad (unos 1800-2000 px, calidad 0.88) y que la segunda revisión llegue de verdad a 2400 px o más. | Unas 2 veces los píxeles por página, sin llamadas extra. | Minutos. | Si el escaneo de origen ya es de baja resolución no aporta. Toca todas las páginas, así que se mide antes de adoptarlo. |
| B | **Señales de duda gratuitas**, revisadas en Go sobre la transcripción ya guardada (lista abajo). | 0 | 1-2 días | No detecta una lectura limpia, consistente y equivocada («noviembre» por «septiembre» en letra y en número). |
| C | **Lector de fecha a ciegas con dos vistas**: recorta la zona de la fecha y la manda dos veces, a color y con el sello aclarado. El modelo no ve la lectura anterior, responde en un formato fijo y puede decir «no legible». | 2 a 4 llamadas pequeñas por convenio (por medir). | 3-5 días | El mismo modelo puede equivocarse igual en ambas vistas, y depende de encontrar la página correcta. |
| D | **Estado guardado por campo**: «por confirmar», «confirmada por el usuario» o normal, en una tabla nueva sin subir la versión del esquema. | 0 | 3-5 días | Mucha interfaz sin pruebas automáticas. Hay que decidir cómo se ve en cada lugar. |
| E | **Confirmar contra la imagen**: para una fecha por confirmar, la revisión muestra el recorte exacto que leyó la IA, con el rótulo «Imagen que leyó la IA», y las lecturas como botones iguales ([15/09/2021] [15/11/2021] [Otra]), sin resaltar ninguna. | 0, porque se vuelve a dibujar en la PC. | ~2 días | Si el recorte cae en otro párrafo se nota de inmediato, pero hay que presentarlo como acercamiento, nunca como «aquí está la fecha». |
| F | **Usar la otra copia como segundo lector**: si el convenio tiene un duplicado, comparar con su transcripción, que ya está pagada. | 0 | 1-2 días | Solo cubre los convenios con copia, pero sirve de fuente de casos para medir. |
| G | **Probar otro modelo de visión.** | Depende del modelo; volver a transcribir cuesta el precio completo. | De nada a poco. | Baja los errores pero no crea el «no sé». Las imágenes con firmas irían a otro proveedor, y eso lo decide el usuario. |

**Señales de la opción B (todas gratuitas):**

- `[?]`, `[ilegible]` o `[Sello` cerca de la fecha en la transcripción.
- La letra no coincide con el número: «veinticinco (26)».
- La fecha guardada no coincide con la cita literal de la fecha que la IA ya devuelve (`fecha_firma_texto`), que hoy se descarta.
- Fecha vacía en un documento que sí tiene cierre o cláusula de vigencia.
- Firma posterior al registro, vigencia anterior a la firma, o un plazo que no cuadra con la fecha de término escrita.
- Un día imposible, como el 31 de un mes de 30 días.
- En Duplicados: misma contraparte, texto casi idéntico y fechas distintas. Se muestra como «fechas distintas: ¿misma copia o renovación?» antes de suponer renovación.

**Preparar la imagen en la PC sin dependencias nuevas sí se puede.** El canvas de la WebView permite leer y modificar los píxeles.

- Escala de grises y más contraste: triviales.
- Quitar el color del sello tomando el canal más claro de cada píxel (`max(R,G,B)`): la tinta azul, morada o roja se aclara y el texto negro se queda negro.
- Límites:
  - no sirve con sellos negros ni con escaneos que ya están en grises;
  - también borra el texto del sello, por eso solo puede ser una segunda vista, nunca la imagen de la transcripción principal;
  - enderezar la página es posible en unas 60 líneas, pero va al final;
  - convertir a blanco y negro puro borra dígitos tenues, así que no se hace.

## Goals vs. Non-Goals

* **In-Scope Goals:**
  - Medir primero, con los convenios que ya hay, dónde nace el error (transcripción o extracción) y cuántos errores silenciosos existen.
  - Marcar como «Fecha por confirmar» toda fecha que no pasó una comprobación, de modo que se vea en la lista, en la ficha, en el estado de vigencia y en Duplicados.
  - Separar los tres casos que hoy son el mismo «—»: «No consta en el documento», «No se pudo leer» y «Sin analizar».
  - Que una fecha por confirmar cuente como desconocida en las decisiones automáticas: vincular renovaciones, clasificar duplicados y calcular la vigencia por plazo.
  - Leer mejor la fecha solo donde hace falta (resolución corregida, lector a ciegas, sello aclarado), si la medición lo justifica.
  - Registrar los tokens usados por llamada para poder medir el costo real.
* **Out-of-Scope Non-Goals:**
  - Dibujar recuadros sobre la imagen escaneada a partir de coordenadas que dé el modelo.
  - Mostrar porcentajes de confianza.
  - Guardar la marca dentro de «Notas».
  - Llamar «dudosa» a la fecha: en lenguaje administrativo suena a documento irregular.
  - Sobrescribir fechas que el usuario editó o confirmó, o deshacer solos los vínculos de versión ya creados (solo listarlos).
  - Volver a procesar todo el archivo de forma automática o al arrancar. Siempre a pedido, con el costo a la vista.
  - Preprocesar la imagen de la transcripción principal, o enderezar y pasar a blanco y negro todas las páginas.
  - Dependencias nuevas (OpenCV, Tesseract, OCR en el navegador).
  - Fechas manuscritas.
  - Subir la versión del esquema de la base en las dos primeras entregas.

## Alternatives Weighed

- **Leer dos veces la misma imagen y comparar.** Descartada: con temperatura 0, dos lecturas iguales repiten el mismo error y se paga doble.
- **Reusar la revisión de dudas actual (VerifyAI) tal como está.** Descartada: le enseña al modelo la lectura previa, no puede llenar vacíos y vuelve a correr la extracción completa, que sobrescribe la ficha. Sí se reusa su tubería para volver a dibujar las páginas.
- **Guardar la duda en «Notas» y dejar la fecha vacía.** Descartada: las notas son texto libre del usuario, no se pueden filtrar y la siguiente extracción las reemplaza.
- **Agregar columnas con un esquema v8.** Descartada para las primeras entregas: la actualización no tiene vuelta atrás y una versión anterior de la app dejaría de abrir la base. Una tabla nueva con `CREATE TABLE IF NOT EXISTS` no exige subir la versión (store.go:100-118).
- **Subir la resolución de todas las páginas como única solución.** Descartada como solución única: si el escaneo de origen es pobre no agrega información, y encarece páginas sin fechas. Queda como experimento medido.
- **Cambiar de modelo de visión como única solución.** Descartada como solución única: puede bajar los errores, pero no crea el «no sé». Queda como experimento si el usuario acepta mandar imágenes a otro proveedor.
- **Resaltar en la imagen escaneada dónde cree la IA que está la fecha.** Descartada: rompe la regla de no señalar como seguro algo que puede estar mal. Se invierte la idea y se muestra qué miró la IA (opción E).

## Key Decisions & Staging for Next Steps

**Paso 0, triaje (sin código y sin API, medio día):**
- Juntar como conjunto de prueba:
  - los pares de Duplicados con la misma contraparte y fechas distintas;
  - los escaneados «Sin fecha»;
  - unos 20 escaneados al azar.
- El usuario anota desde el papel la fecha real y la causa visual (sello, color, inclinación, calidad) en un CSV fuera de la base.
- Para cada caso, abrir la transcripción guardada y decidir si el error es de lectura (a), de extracción (b), de recorte (c) o de la regla 7 (d).
- Guardar una captura de los pares actuales de Duplicados antes de cambiar nada.

**Entrega 1, hacer visibles los errores sin gastar API:**
- Registrar tokens por llamada.
- Implementar las señales de la opción B con pruebas en Go, más la regla «fechas distintas» en Duplicados.
- Guardar la marca en la tabla nueva de la opción D. Se invalida sola si la fecha cambia.
- Que una fecha por confirmar cuente como desconocida en renovaciones, duplicados y vigencia calculada.
- Interfaz mínima:
  - insignia «Fecha por confirmar»;
  - filtro con el número de fechas por confirmar;
  - botón «La fecha es correcta»;
  - los tres tipos de vacío separados.
- Una pasada única sobre las transcripciones ya guardadas que solo escribe marcas, nunca fechas.

**En paralelo:** medir la opción A (y la G, si se quiere) re-transcribiendo solo las páginas de fecha del conjunto de prueba, sobre una copia de los datos.

**Entrega 2, solo si la 1 demuestra que la marca atrapa los casos:**
- El lector a ciegas con dos vistas (C).
- Confirmar contra la imagen (E).
- Una acción «Volver a leer fechas» que solo toca fechas y muestra el costo antes de correr.
- Antes, en un cambio aparte, unificar las tres secuencias repetidas que corren después del análisis en la interfaz.

**Cómo se mide el éxito:**
- **Métrica principal:** fechas equivocadas y sin marca (errores silenciosos), con meta cercana a 0 en el conjunto de prueba.
- **Métricas secundarias:**
  - de las fechas equivocadas, cuántas quedaron marcadas;
  - de las marcadas, cuántas estaban realmente mal;
  - porcentaje de convenios marcados, con un techo cercano al 15 % para que el usuario no deje de mirarlas;
  - tokens por convenio.
- Con 10 a 20 casos malos, los resultados se revisan uno por uno, no como estadística.

**Decisión pendiente del usuario:** cuando una extracción nueva no puede comprobar la fecha, ¿se guarda la fecha propuesta con la marca «por confirmar», o se deja vacía con las lecturas candidatas aparte?
- Dejarla vacía es más seguro en todas partes (contadores, CSV, nombre del archivo), pero sube el número de «Sin fecha».
- Guardarla con marca obliga a que cada pantalla muestre la marca.
- Regla mixta propuesta: vacía en extracciones nuevas; solo marca en la pasada sobre los convenios existentes.
