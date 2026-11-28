# Cola de procesamiento con pausa y reanudación (versión 1.6.0)

Diseño aprobado por el usuario el 2026-11-27: enfoque **A** (cola guardada en la base) y opción **2** (al abrir no se procesa nada solo: el aviso ofrece «Continuar»).

## Intención

Poder importar hasta 200 convenios de una vez y dejar el procesamiento con IA (transcripción y extracción) para cuando haya tiempo: pausarlo, cerrar la aplicación y continuar después, sin perder lo ya hecho.

## Qué se construye

### 1. Límite de 200

- `MAX_IMPORT = 200` en `frontend/dist/app.js` y el mismo tope en `App.ImportPDFs` (`app.go`).
- La lista del diálogo de importación ya tiene scroll propio (`.import-list`, 250 px), así que 200 renglones no rompen el diálogo.

### 2. Cola guardada en una tabla nueva

```sql
CREATE TABLE IF NOT EXISTS cola_procesamiento (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  convenio_id INTEGER NOT NULL UNIQUE REFERENCES convenios(id) ON DELETE CASCADE,
  creado_en   TEXT NOT NULL DEFAULT (datetime('now'))
);
```

- Sin subir la versión del esquema: una versión anterior de la app ignora la tabla (mismo patrón que `revision_fechas`).
- El orden de la cola es por `id`.
- Borrar un convenio lo saca de la cola solo (`ON DELETE CASCADE`).
- En `store`: `EnqueueForAI(ids)`, `Dequeue(id)`, `QueuedIDs()`, `QueueCount()`, `PruneQueue()`.
  - `PruneQueue` quita las filas de convenios ya resueltos (`ia_estado` `ok` o `parcial`) o archivados. Corre al arrancar y antes de iniciar la cola.
- La cola **no** guarda el avance dentro de un archivo: eso ya existe en `transcripcion_paginas` y en el estado de la transcripción.

### 3. Cuándo se encola

- Al importar, cuando la IA está activa y con clave: cada archivo que se va a procesar (los no duplicados clonados) entra a la cola con `EnqueueForAI`. Equivale al `startAutoAI` de hoy.
- El botón «Analizar con IA» de la lista y la ficha también encola.
- Los reintentos por error siguen como hoy, pero **dejan de arrancar solos al abrir**: al pulsar «Continuar» se piden los candidatos con `RetryCandidates` y se suman a la cola.

### 4. Pausar y continuar

- El aviso de procesamiento (`#processing-banner`) gana dos botones: «Pausar» y «Continuar».
- «Pausar» es inmediato en el siguiente punto de control:
  - entre archivos, antes de tomarlos de la cola;
  - entre páginas, dentro de `transcribeConvention`.
  - La página en curso termina y se guarda. Una extracción en curso (una sola llamada, de segundos) también termina; después ya no se toma otro archivo.
- Pausar no es un error: no se llama a `FailTranscription` ni se suman rondas de reintento. El archivo que quedó a medias sigue en la cola y, al continuar, retoma con las páginas que ya tiene (lo que hoy hace `ResumeTranscription`).
- «Continuar» arranca la cola con los ids que devuelve el servidor, más los candidatos de reintento.

### 5. Al abrir la aplicación

- `app.startup` corre `PruneQueue`.
- `GetBootstrap` agrega `cola_pendientes` (número). Si es mayor que 0 y la IA está configurada, la lista muestra el aviso «N convenios en cola para procesar con IA · Continuar · Quitar».
  - **Continuar**: procesa la cola.
  - **Quitar**: vacía la cola (deja los convenios como están, sin procesar). Pide confirmación.
- El reintento automático al arrancar (`retryFailedOnStart`) desaparece; queda dentro de «Continuar», como se explicó.
- Si la IA no está configurada, no se muestra el aviso: los archivos quedan sin procesar, como hoy.

### 6. Final de cada archivo

- Si termina con datos (`ok` o `parcial`): se borra su fila de la cola.
- Si falla definitivamente tras los reintentos de la sesión: se borra su fila y queda `ia_estado='error'` en la ficha, con el botón de reintentar de siempre. No se queda en un ciclo que gaste sin fin.
- El contador del aviso sale de las filas de la cola, así que sobrevive a cerrar la app y a un apagón.

## Lo que no cambia

- `ExportConvenios` y los paquetes no exportan la cola: es trabajo pendiente de esta PC.
- La transcripción, la extracción y sus reintentos funcionan igual; solo dejan de arrancar solos al abrir la app.
- Nada de la versión 1.5.0 (fechas por confirmar, revisión puntual) cambia.

## Pruebas

- **Go (`internal/store`)**: encolar conserva el orden; una fila repetida no se duplica; `Dequeue` la quita; borrar el convenio la quita; `PruneQueue` quita resueltas y archivadas; la cola sobrevive a cerrar y abrir la base.
- **Go (raíz)**: `ImportPDFs` acepta 200; `GetBootstrap` informa `cola_pendientes`.
- **Interfaz (manual, playwright-cli con el stub)**: importar N, «Pausar» corta en una página, recargar muestra el aviso con N pendientes; «Continuar» procesa y vacía; «Quitar» vacía.

## Riesgos

- Con 200 convenios escaneados, la primera vez son muchos cientos de páginas: el costo de IA sube en proporción. El aviso dice cuántos hay antes de continuar, y por eso el arranque no es automático.
- Pausar entre páginas deja transcripciones incompletas a la vista («Transcripción incompleta»), que es el mismo estado que ya existe cuando falla una página.
