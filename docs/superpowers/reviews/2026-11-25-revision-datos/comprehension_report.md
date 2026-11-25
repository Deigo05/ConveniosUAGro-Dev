# Etapa 6: prueba de comprensión

Documento: `docs/superpowers/specs/2026-11-25-revision-puntual-fechas-design.md` (142 líneas). Revisor: `egm-comprehension`, sin estándar y sin contexto: solo leyó el documento.

**What this proposes**

Junto a la etiqueta de cada dato que la IA extrae y pone en la ficha va un ícono discreto, «Ver en el documento». Esos datos son contraparte, UAGro, fecha de firma, fecha de vigencia y cláusula de vigencia. Al pulsarlo, una vista hecha con PDF.js reemplaza por un rato el visor de Edge en la columna «Documento original». La vista dibuja la página, encierra en un recuadro el fragmento encontrado, permite pasar de una aparición a otra y dice cómo se encontró o que el dato «no aparece así». Lo que se busca es el valor que tiene el campo en ese momento, normalizado y con reglas distintas según el campo. Para la firma se usa además una cita nueva, `fecha_firma_texto`, que se guardará con la migración al esquema 8. En las páginas escaneadas se usa la transcripción guardada por página y no hay resaltado. Todo corre localmente, solo al pulsar el botón y sin volver a analizar los convenios existentes. La combinación recomendada, «B con 1», sigue pendiente de que el usuario la elija.

**How the existing system works, per this document**

Al importar un convenio, DeepSeek analiza el texto y llena la ficha. Para dos datos devuelve también la cita literal. `vigencia_texto` se guarda y el usuario puede editarla. `fecha_firma_texto` solo sirve para interpretar la fecha y luego se descarta. Antes de guardar los nombres, `ApplyNameCorrections` puede corregirlos usando la transcripción. La interfaz ya trae PDF.js y lo usa para dibujar páginas que luego se transcriben. La ficha muestra el PDF en un `iframe` con el visor de Edge (WebView2), que puede saltar de página pero no resaltar. El texto que Go extrae no marca dónde empieza cada página. El de las páginas escaneadas sí está guardado por página, en `transcripcion_paginas`. Cuando la vigencia es un plazo, el sistema calcula la fecha a partir de la firma, así que esa fecha no aparece en el documento.

## Hallazgos (sin Blockers: el revisor pudo parafrasear todas las secciones)

1. **Major. El ejemplo de la pantalla contradice las reglas** (líneas 104-120 contra 90 y 100). El campo «Dr. J. Saldaña» resaltado como «Dr. Javier Saldaña A.» con «texto exacto»: ninguna regla cubre iniciales. Además, «Firmante (UAGro)» aparece bajo «Contraparte».
2. **Major. No se define qué es una coincidencia «dudosa»** (línea 40 contra 90, 93-94, 97, 100). Las etiquetas describen el método, no la confianza. Faltan homónimos y una misma fecha que pertenece a otro acto. Recomendación: relacionar cada estrategia con un nivel de confianza y con el aviso que ve el usuario.
3. **Major. No se explica cómo pasar de la coincidencia en texto normalizado al recuadro en la página** (líneas 88-95, 137-141). Tampoco qué pasa si la coincidencia cruza fragmentos, renglones o páginas.
4. **Major. «Ciudad de la facultad» no tiene regla de búsqueda** (línea 22 contra 89-95 y 98). El mensaje «la eligió de una lista» no sirve para los datos deducidos, y no se define el catálogo de facultades.
5. **Major. Se da por conocido el proceso de transcripción** (líneas 51, 53, 96). No se dice quién transcribe, qué cuenta como página escaneada ni el orden de búsqueda en un PDF mixto.
6. **Major. No se definen los paquetes exportados ni `CloneAnalysis`** (línea 80). Tampoco qué pasa al importar un paquete anterior al esquema 8.
7. **Nit.** No se dice qué consecuencia tiene `ApplyNameCorrections` para la búsqueda (línea 50).
8. **Nit.** La línea 52 dice que el visor de Edge salta de página y la 62 lo pone en duda.
9. **Nit.** No hay controles para cambiar de página y buscar a mano (línea 98).
10. **Nit.** `MaxPages`, `SetAIResult` y «esquema 8» aparecen sin valor ni descripción (líneas 80, 134).
11. **Nit.** «No impedir la marca después» no dice cómo se cumple (línea 33).
12. **Nit.** «Proemio», «declaraciones» y «cierre» no se explican (líneas 19, 23), y la regla de empezar por la última fecha depende de saber que el cierre va al final.
