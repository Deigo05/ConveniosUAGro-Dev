// Package municipios unifica los nombres de municipio que se escriben de
// varias formas. La IA a veces devuelve el nombre corto («Acapulco») en lugar
// del oficial («Acapulco de Juárez»), y cada forma acababa como una etiqueta
// de ciudad distinta.
package municipios

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type municipio struct {
	Oficial string
	Estado  string
	// Ambiguo marca los nombres cortos que también son de un municipio de
	// otro estado (Ayutla, Jalisco): solo se unifican si consta el estado.
	Ambiguo bool
	Alias   []string
}

// catalogo lista el nombre oficial de cada municipio y las formas que se
// reconocen como el mismo. Las formas se comparan sin acentos ni mayúsculas.
var catalogo = []municipio{
	{Oficial: "Acapulco de Juárez", Estado: "Guerrero", Alias: []string{"acapulco"}},
	{Oficial: "Chilpancingo de los Bravo", Estado: "Guerrero", Alias: []string{"chilpancingo", "chilpancingo de los bravos"}},
	{Oficial: "Iguala de la Independencia", Estado: "Guerrero", Alias: []string{"iguala"}},
	{Oficial: "Taxco de Alarcón", Estado: "Guerrero", Alias: []string{"taxco"}},
	{Oficial: "Zihuatanejo de Azueta", Estado: "Guerrero", Alias: []string{"zihuatanejo"}},
	{Oficial: "Tlapa de Comonfort", Estado: "Guerrero", Alias: []string{"tlapa"}},
	{Oficial: "Tixtla de Guerrero", Estado: "Guerrero", Alias: []string{"tixtla"}},
	{Oficial: "Ayutla de los Libres", Estado: "Guerrero", Ambiguo: true, Alias: []string{"ayutla"}},
}

// marcasEstado son las formas en que un nombre puede traer pegado su estado:
// «Acapulco, Gro.», «Acapulco Guerrero».
var marcasEstado = map[string]string{"guerrero": "Guerrero", "gro": "Guerrero"}

var porClave = func() map[string]*municipio {
	m := map[string]*municipio{}
	for i := range catalogo {
		mu := &catalogo[i]
		m[clave(mu.Oficial)] = mu
		for _, a := range mu.Alias {
			m[clave(a)] = mu
		}
	}
	return m
}()

// clave quita acentos, mayúsculas y signos: «Acapulco, Gro.» → «acapulco gro».
func clave(s string) string {
	s = norm.NFD.String(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Oficial devuelve el nombre oficial y el estado del municipio que nombra
// nombre. estado es el estado conocido de la etiqueta o de la sugerencia (puede
// ir vacío); si es de otro estado, el nombre no se toca. ok es false si el
// municipio no está en el catálogo.
func Oficial(nombre, estado string) (oficial, estadoOficial string, ok bool) {
	k := clave(nombre)
	if k == "" {
		return "", "", false
	}
	mu, found := porClave[k]
	estadoEnNombre := ""
	if !found {
		// «Acapulco, Gro.»: el estado viene pegado al nombre.
		words := strings.Fields(k)
		if len(words) > 1 {
			if est, esMarca := marcasEstado[words[len(words)-1]]; esMarca {
				if mu, found = porClave[strings.Join(words[:len(words)-1], " ")]; found {
					estadoEnNombre = est
				}
			}
		}
	}
	if !found {
		return "", "", false
	}
	conocido := estado
	if conocido == "" {
		conocido = estadoEnNombre
	}
	if conocido != "" && clave(conocido) != clave(mu.Estado) {
		return "", "", false
	}
	if mu.Ambiguo && conocido == "" && clave(nombre) != clave(mu.Oficial) {
		return "", "", false
	}
	return mu.Oficial, mu.Estado, true
}
