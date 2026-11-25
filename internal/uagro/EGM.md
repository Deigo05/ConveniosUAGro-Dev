# uagro

## Overview
Catálogo de las facultades (unidades académicas de nivel superior) de la UAGro y su sede, más las reglas para reconocer cómo aparecen escritas en los convenios: con o sin «Unidad Académica», «Escuela Superior», siglas, «No. 02» o la ciudad pegada al final. Regla del paquete: la etiqueta de facultad nunca debe tomar la ciudad de la contraparte. También produce el resumen del catálogo que va en el mensaje de sistema del modelo. Lo usan `app.go` e `internal/ai`.

## Interface
- `type Faculty`: `Name` (nombre oficial, puede terminar en número de unidad, como «Facultad de Enfermería 02»), `City` (sede canónica), `Aliases` (formas normalizadas, en minúsculas y sin acentos) y `Primary` (sede principal de una facultad con varias sedes) [facultades.go:Faculty]
- `var Faculties []Faculty`: catálogo exportado de 35 entradas [facultades.go:Faculties]
- `func Resolve(facultad, city string) (name, sede string, ok bool)`: nombre canónico y sede de la facultad. `ok=false` significa que el texto no nombra una unidad académica (convenio abierto), y entonces `name` y `sede` vienen vacíos. `sede` puede quedar vacía si la facultad tiene varias sedes y el documento no dice cuál [facultades.go:Resolve]
- `func CanonicalCity(city string) (string, bool)`: normaliza una ciudad y comprueba que sea sede de la UAGro [facultades.go:CanonicalCity]
- `func CatalogText() string`: resumen del catálogo para el modelo, por ejemplo `Facultad de Turismo (Acapulco, Zihuatanejo); Facultad de Derecho (Acapulco, Chilpancingo)…` [facultades.go:CatalogText]

## Invariants
- La ciudad de la contraparte nunca se usa como sede. El código lo cumple de dos formas: una facultad con una sola sede ignora una ciudad que no coincide, y quien llama no debe pasar la ciudad de la contraparte en `city` [facultades.go:Resolve] [facultades.go:pick]
- La normalización pasa a minúsculas, quita acentos (NFD), conserva solo `[a-z0-9]`, cambia `- _ . , " '` por espacios y borra los demás caracteres. Luego descarta las marcas de número (`no`, `num`, `numero`, `n`, `nro`, `n0`) y quita los ceros a la izquierda, así que «No. 02», «Núm. 2» y «2» son equivalentes [facultades.go:simplify] [facultades.go:normalize]
- `key` quita repetidamente los prefijos de `unitPrefixes` (de «unidad academica de la facultad de» hasta «la», «de» y «ua»), quita los sufijos « de la universidad autonoma de guerrero», « de la uagro» y « uagro», expande la sigla de la primera palabra según `unitAbbreviations` (`mvz`, `emvz`, `esmvz`, `fmvz`, `uamvz`, `fca`, `fcqb`, `fcti`) y separa como número de unidad la última palabra si es numérica [facultades.go:key] [facultades.go:stripPrefixes] [facultades.go:unitAbbreviations]
- Una ciudad explícita y reconocida en `city` tiene prioridad. Si no la hay, se usa la sede pegada al final de `facultad` (el alias más largo de `campusAliases`, recortado por palabras para conservar los acentos). La sede pegada siempre se quita antes de comparar [facultades.go:Resolve] [facultades.go:splitCity]
- `CanonicalCity` quita al final « guerrero» y luego « gro» antes de buscar en `campusAliases`. Una ciudad vacía o desconocida da `("", false)`. `splitCity` no quita «Gro.» ni «Guerrero» [facultades.go:CanonicalCity] [facultades.go:splitCity]
- Una facultad coincide si la base de su nombre o de algún alias es igual a la del texto y además coincide el número de unidad, o el documento no trae número (en ese caso sirven todas las variantes numeradas) [facultades.go:candidates]
- Si no hay coincidencia exacta, se reintenta quitando al final de 1 a 3 palabras, sin quitarlas todas: «Turismo Tijuana» lleva a Turismo [facultades.go:Resolve]
- Orden para elegir sede: (1) la entrada cuya `City` coincide con la ciudad reconocida; (2) si solo hay una entrada, esa con su sede del catálogo; (3) la entrada con `Primary`; (4) si los nombres difieren (unidades numeradas sin pista), el nombre común sin número y la sede vacía, como «Facultad de Enfermería»; (5) si todas comparten nombre (Derecho sin ciudad), ese nombre con la sede vacía [facultades.go:pick] [facultades.go:withoutNumber]
- Para una unidad fuera del catálogo, `ok=true` solo si el texto empieza con una palabra de `unitKinds` (`facultad`, `escuela`, `unidad academica`, `instituto`, `centro`). Entonces el nombre lo da `displayName`, que convierte «Unidad Académica de X» en «Facultad de X» y escribe el número final con al menos dos dígitos («03»). Cualquier otro texto («Rectoría») da `("", "", false)` [facultades.go:Resolve] [facultades.go:unitKinds] [facultades.go:displayName]
- Las siglas como «UAGro-FT» no se reconocen: `Resolve("UAGro-FT", "")` devuelve `("", "", false)`, porque `ft` no está en `unitAbbreviations` (el comentario de `Faculties` lo dice así desde la 1.3.1) [facultades.go:Faculties] [facultades.go:unitAbbreviations]
- `CatalogText` agrupa por `Name` exacto en orden de aparición, así que su salida es estable mientras no cambie el orden de `Faculties`. Va en el mensaje de sistema, que no cambia, para aprovechar la caché del proveedor [facultades.go:CatalogText]
- `Faculties` es una variable exportada y modificable, y `Resolve` y `CatalogText` la leen en cada llamada. Modificarla desde fuera cambia sus resultados [facultades.go:Faculties]

## Side Effects
- None

## Verification
- Run: `go test ./internal/uagro/...`
