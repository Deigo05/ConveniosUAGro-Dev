package store

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TiposDocumento es el catálogo de tipos de documento que usa el nombre de
// archivo. La IA elige uno y el usuario puede cambiarlo en la ficha.
var TiposDocumento = []string{
	"Convenio General",
	"Convenio Específico",
	"Convenio Marco",
	"Convenio de Colaboración",
	"Convenio Modificatorio",
	"Carta de Intención",
	"Contrato",
	"Acuerdo",
	"Otro",
}

// NormalizeTipoDocumento devuelve el tipo del catálogo que corresponde a s,
// aceptando formas largas como "Convenio General de Colaboración". Devuelve
// "" si no reconoce el tipo.
func NormalizeTipoDocumento(s string) string {
	k := strings.Join(strings.Fields(NormalizeKey(s)), " ")
	if k == "" {
		return ""
	}
	for _, t := range TiposDocumento {
		if NormalizeKey(t) == k {
			return t
		}
	}
	switch {
	case strings.Contains(k, "modificatorio"), strings.Contains(k, "addendum"), strings.Contains(k, "adendum"), strings.Contains(k, "adenda"):
		return "Convenio Modificatorio"
	case strings.Contains(k, "carta de intencion"):
		return "Carta de Intención"
	case strings.HasPrefix(k, "contrato"):
		return "Contrato"
	case strings.HasPrefix(k, "acuerdo"):
		return "Acuerdo"
	case strings.Contains(k, "especifico"):
		return "Convenio Específico"
	case strings.Contains(k, "marco"):
		return "Convenio Marco"
	case strings.Contains(k, "general"):
		return "Convenio General"
	case strings.Contains(k, "colaboracion"), strings.Contains(k, "convenio"):
		return "Convenio de Colaboración"
	}
	return ""
}

const (
	maxFileStem        = 150
	maxCounterpartName = 70
)

// FileName arma el nombre con el que se descarga el PDF, por ejemplo
// "2024-03-15 Convenio Específico - IMSS - Facultad de Medicina Acapulco.pdf".
// La fecha va primero para que una carpeta de descargas quede en orden
// cronológico. Mientras no se conoce la contraparte se usa el nombre original.
func FileName(c ConventionSummary) string {
	contraparte := strings.TrimSpace(c.DependenciaCorta)
	if contraparte == "" {
		contraparte = shortCounterpart(c.Dependencia)
	}
	if contraparte == "" {
		return withExt(cleanFileName(c.ArchivoNombreOriginal), ".pdf")
	}
	fecha := c.FechaFirma
	if fecha == "" {
		fecha = "Sin fecha"
	}
	tipo := c.TipoDocumento
	if tipo == "" || tipo == "Otro" {
		tipo = "Convenio"
	}
	parts := []string{fecha + " " + tipo, contraparte}
	if unidad := academicUnit(c); unidad != "" {
		parts = append(parts, unidad)
	}
	return withExt(cutRunes(cleanFileName(strings.Join(parts, " - ")), maxFileStem), ".pdf")
}

// academicUnit usa la etiqueta de facultad (que ya incluye la sede o dice
// "Convenio Abierto") y, si no la hay, la facultad capturada.
func academicUnit(c ConventionSummary) string {
	for _, t := range c.Etiquetas {
		if t.Tipo == "facultad" {
			return t.Nombre
		}
	}
	return strings.TrimSpace(strings.TrimSpace(c.Facultad) + " " + strings.TrimSpace(c.FacultadCiudad))
}

// shortCounterpart toma la primera contraparte y la recorta en una palabra
// completa para que el nombre del archivo no sea interminable.
func shortCounterpart(dep string) string {
	dep = strings.Join(strings.Fields(dep), " ")
	extra := ""
	if i := strings.Index(dep, ";"); i >= 0 {
		dep, extra = strings.TrimSpace(dep[:i]), " y otras"
	}
	if utf8.RuneCountInString(dep) > maxCounterpartName {
		dep = cutRunes(dep, maxCounterpartName)
		if i := strings.LastIndex(dep, " "); i > maxCounterpartName/2 {
			dep = dep[:i]
		}
		// No terminar en un conector: "Secretaría de" → "Secretaría".
		for {
			i := strings.LastIndex(dep, " ")
			if i < 0 || !connectors[NormalizeKey(dep[i+1:])] {
				break
			}
			dep = dep[:i]
		}
		dep = strings.TrimRight(dep, " ,.-")
	}
	if dep == "" {
		return ""
	}
	return dep + extra
}

var connectors = map[string]bool{
	"de": true, "del": true, "la": true, "las": true, "el": true, "los": true,
	"y": true, "e": true, "en": true, "para": true, "por": true, "a": true, "al": true,
}

// cleanFileName quita los caracteres que Windows no admite en un nombre de
// archivo y deja un solo espacio entre palabras.
func cleanFileName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '"' || r == '?' || r == '*' || r == '<' || r == '>':
			continue
		case r == '/' || r == '\\' || r == '|' || r == ':':
			b.WriteRune('-')
		case unicode.IsControl(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Trim(strings.Join(strings.Fields(b.String()), " "), " .")
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:n]), " .-")
}

func withExt(stem, ext string) string {
	if stem == "" {
		stem = "convenio"
	}
	if strings.EqualFold(strings.TrimSpace(stem[max(0, len(stem)-len(ext)):]), ext) {
		return stem
	}
	return stem + ext
}

// UniqueFileName agrega " (2)", " (3)"… si el nombre ya se usó (sin distinguir
// mayúsculas, como Windows) y lo registra en used.
func UniqueFileName(name string, used map[string]bool) string {
	ext := ""
	if i := strings.LastIndex(name, "."); i > 0 {
		ext = name[i:]
	}
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for n := 2; used[strings.ToLower(candidate)]; n++ {
		candidate = stem + " (" + strconv.Itoa(n) + ")" + ext
	}
	used[strings.ToLower(candidate)] = true
	return candidate
}
