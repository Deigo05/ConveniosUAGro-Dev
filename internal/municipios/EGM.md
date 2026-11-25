# municipios

## Overview
Unifica los nombres de municipio que se escriben de varias formas. La IA a veces devuelve el nombre corto («Acapulco») en lugar del oficial («Acapulco de Juárez»), y cada forma terminaba como una etiqueta de ciudad distinta. El paquete compara contra un catálogo pequeño de municipios de Guerrero, sin tomar en cuenta acentos, mayúsculas ni signos. Lo usan `internal/ai` y `internal/store`.

## Interface
- `func Oficial(nombre, estado string) (oficial, estadoOficial string, ok bool)`: devuelve el nombre oficial y el estado del municipio que nombra `nombre`. `estado` es el estado conocido de la etiqueta o de la sugerencia y puede ir vacío. Si algo falla, devuelve `("", "", false)` [municipios.go:Oficial]
- Es el único símbolo exportado. El catálogo (`catalogo`), las marcas de estado (`marcasEstado`), el índice (`porClave`) y el normalizador (`clave`) son internos [municipios.go:catalogo] [municipios.go:marcasEstado] [municipios.go:porClave] [municipios.go:clave]

## Invariants
- Toda comparación pasa por `clave`: aplica NFD, quita las marcas diacríticas (`unicode.Mn`), pasa las letras a minúsculas, conserva los dígitos, cambia cualquier otro carácter por un espacio y colapsa los espacios. Por ejemplo, «Acapulco, Gro.» queda como `acapulco gro` [municipios.go:clave]
- La búsqueda es exacta sobre la clave normalizada, sin coincidencias parciales ni aproximadas. Un nombre que queda vacío al normalizarse devuelve `("", "", false)` [municipios.go:Oficial]
- El catálogo tiene 8 municipios, todos de Guerrero: Acapulco de Juárez, Chilpancingo de los Bravo, Iguala de la Independencia, Taxco de Alarcón, Zihuatanejo de Azueta, Tlapa de Comonfort, Tixtla de Guerrero y Ayutla de los Libres, cada uno con sus alias cortos [municipios.go:catalogo]
- Si la clave completa no aparece, se prueba quitar una sola marca de estado como última palabra (`guerrero` o `gro`). Así «Acapulco, Gro.» y «Acapulco Guerrero» se reconocen, pero «Gro. Acapulco» no [municipios.go:Oficial] [municipios.go:marcasEstado]
- El argumento `estado` tiene prioridad sobre el estado que venga pegado al nombre. Si el estado conocido no coincide (por clave) con el del catálogo, el nombre no se toca y la función devuelve `false`. `estado` no pasa por `marcasEstado`, así que `estado="Gro"` no coincide con «Guerrero» [municipios.go:Oficial]
- Los municipios con `Ambiguo=true` (hoy solo Ayutla de los Libres, porque también existe Ayutla en Jalisco) solo se unifican si consta el estado o si el texto ya es el nombre oficial: «Ayutla» sin estado devuelve `false` [municipios.go:municipio] [municipios.go:Oficial]
- `estadoOficial` siempre sale del catálogo, nunca de cómo lo escribió quien llama [municipios.go:Oficial]
- Las claves de `porClave` deben ser únicas en todo el catálogo: si dos entradas generaran la misma, la última sobrescribiría a la anterior sin aviso [municipios.go:porClave]

## Side Effects
- Al inicializarse el paquete, `porClave` se construye en memoria a partir de `catalogo`, sin E/S [municipios.go:porClave]
- `Oficial` es una función pura: no hace E/S y no modifica el estado del paquete, así que se puede llamar de forma concurrente [municipios.go:Oficial]

## Verification
- Run: `go test ./internal/municipios/...`
