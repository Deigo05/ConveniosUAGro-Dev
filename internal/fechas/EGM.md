# fechas

## Overview
Comprueba, sin llamar a la IA, si la fecha de firma y la fecha de vigencia que capturó la IA coinciden con lo que dice el texto del convenio. Sirve sobre todo para los PDF escaneados: la IA puede leer mal una fecha impresa bajo un sello, con poco contraste o en una página chueca. Cuando la fecha no coincide con el texto, o el texto la marcó como dudosa, el resultado deja la fecha «por confirmar». No marca lo que no puede comprobar. Lo usa `internal/store` (`RefreshDateChecks`).

## Interface
- Constantes `Firma = "firma"` y `Vigencia = "vigencia"`: los dos campos que se comprueban [fechas.go:Firma]
- `type Date` (rango original `Start`/`End`, `ISO` AAAA-MM-DD, `Texto` original, `Conflicto`) y `FindDates(texto) []Date`: las fechas completas (día, mes y año) del texto, en orden [fechas.go:Date] [fechas.go:FindDates]
- `type Input` (texto, fechas guardadas, cláusula, indefinida, calculada), `type Result` (`campo`, `motivo`, `propuesta`, `cita`) y `Check(Input) []Result`, con como máximo un resultado por campo [fechas.go:Input] [fechas.go:Result] [fechas.go:Check]

## Invariants
- **Normalización.** El texto se compara en minúsculas, sin acentos (NFKD sin marcas `Mn`), con los signos cambiados por un espacio y espacios simples. Cada byte normalizado guarda su posición en el original, así que `Texto` y `cita` son el fragmento tal como está escrito [fechas.go:normalize]
- **Fechas.**
  - Se reconocen el día en cifras («25», «1o.») o en letra («veinticinco», «treinta y uno», «primero»), con su número entre paréntesis si lo trae.
  - Luego «días del mes de» o «de», el mes y «de», «del» o «del año», seguido del año en cifras o en letra («dos mil veinticinco», «mil novecientos noventa y nueve»), también con paréntesis opcional.
  - También se aceptan fechas en cifras separadas, como 25/11/2025 (día, mes, año).
  - Una fecha sin día o sin año no cuenta.
  - El número entre paréntesis manda. Si no coincide con el de letra, la fecha queda con `Conflicto` y sin `ISO`. Lo mismo pasa con una fecha que no existe (31 de junio) o con un año fuera de 1950-2100 [fechas.go:findDates] [fechas.go:iso]
- **Cierre.**
  - La fecha de firma se toma del último lugar donde aparece una fórmula de firma («lo firman», «días del mes de», «firmado en»…), la misma lista que usa `revision.js`.
  - Se usa la primera fecha completa hasta 400 caracteres después de la fórmula o, si no hay, la más cercana antes de ella, hasta 200 caracteres.
  - Sin fórmula de firma no se marca nada [fechas.go:lastClosing] [fechas.go:closingDate]
- **Firma por confirmar**, en este orden:
  1. `[?]` o `[ilegible]` a 40 caracteres o menos de la fecha del cierre.
  2. Conflicto entre la letra y el número, o fecha imposible.
  3. Firma vacía con fecha en el cierre (propone esa fecha).
  4. Firma distinta de la del cierre (propone la del cierre).
  5. Cierre sin fecha completa, cuando cumple tres condiciones: la fórmula está en el 60 % final del texto, hay rastro de una fecha (un mes, «días del mes», `[?]`, `[ilegible]` o `[Sello`) y la IA sí puso fecha. Una simple mención como «el firmado en 2021» no cuenta [fechas.go:checkFirma]
- **Vigencia por confirmar.**
  - Nunca se marca si es indefinida.
  - Si es calculada, se marca solo cuando la firma está por confirmar, porque de ella sale el cálculo.
  - Si no es calculada, solo cuentan las fechas de la cláusula precedidas por «hasta», «al», «concluirá», «terminará», «vencerá»… (las fechas de término, no las de inicio). Se marca si alguna tiene `[?]`, si ninguna coincide con la vigencia guardada (propone la última) o si la última tiene conflicto.
  - También se marca una vigencia anterior a la firma [fechas.go:checkVigencia] [fechas.go:endDates]
- La `propuesta` solo existe cuando el texto da una sola fecha legible y sin conflicto. Nunca se inventa una fecha [fechas.go:checkFirma] [fechas.go:checkVigencia]

## Side Effects
- Ninguno: funciones puras, sin E/S ni estado; las expresiones regulares se compilan al iniciar el paquete [fechas.go:init]

## Verification
- Run: `go test ./internal/fechas/...`
