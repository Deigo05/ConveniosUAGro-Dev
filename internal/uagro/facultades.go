// Package uagro contiene el catálogo de facultades de la UAGro y su ciudad.
// Las etiquetas de facultad nunca deben tomar la ciudad de la contraparte.
package uagro

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Faculty es una unidad académica oficial de la UAGro.
type Faculty struct {
	Name    string
	City    string
	Aliases []string
}

// Faculties es un catálogo de las facultades de nivel superior de la UAGro.
// Los nombres se normalizan en Resolve; no hace falta que coincidan
// literalmente con el documento.
var Faculties = []Faculty{
	{Name: "Facultad de Turismo", City: "Acapulco", Aliases: []string{"turismo"}},
	{Name: "Facultad de Derecho", City: "Acapulco", Aliases: []string{"derecho"}},
	{Name: "Facultad de Derecho", City: "Chilpancingo", Aliases: []string{"derecho"}},
	{Name: "Facultad de Medicina", City: "Acapulco", Aliases: []string{"medicina"}},
	{Name: "Facultad de Odontología", City: "Acapulco", Aliases: []string{"odontologia"}},
	{Name: "Facultad de Psicología", City: "Acapulco", Aliases: []string{"psicologia"}},
	{Name: "Facultad de Sociología", City: "Acapulco", Aliases: []string{"sociologia"}},
	{Name: "Facultad de Economía", City: "Acapulco", Aliases: []string{"economia"}},
	{Name: "Facultad de Ecología Marina", City: "Acapulco", Aliases: []string{"ecologia marina"}},
	{Name: "Facultad de Contaduría y Administración", City: "Acapulco", Aliases: []string{"contaduria y administracion", "contaduria", "administracion"}},
	{Name: "Facultad de Lenguas Extranjeras", City: "Acapulco", Aliases: []string{"lenguas extranjeras"}},
	{Name: "Facultad de Ciencias y Tecnologías de la Información", City: "Acapulco", Aliases: []string{"ciencias y tecnologias de la informacion", "tecnologias de la informacion"}},
	{Name: "Facultad de Enfermería 02", City: "Acapulco", Aliases: []string{"enfermeria", "enfermeria 2"}},
	{Name: "Facultad de Ciencias de la Educación 02", City: "Acapulco", Aliases: []string{"ciencias de la educacion", "ciencias de la educacion 2"}},
	{Name: "Facultad de Ingeniería", City: "Chilpancingo", Aliases: []string{"ingenieria"}},
	{Name: "Facultad de Arquitectura y Urbanismo", City: "Chilpancingo", Aliases: []string{"arquitectura y urbanismo", "arquitectura"}},
	{Name: "Facultad de Ciencias Naturales", City: "Chilpancingo", Aliases: []string{"ciencias naturales"}},
	{Name: "Facultad de Ciencias Químico Biológicas", City: "Chilpancingo", Aliases: []string{"ciencias quimico biologicas", "quimico biologicas"}},
	{Name: "Facultad de Comunicación y Mercadotecnia", City: "Chilpancingo", Aliases: []string{"comunicacion y mercadotecnia"}},
	{Name: "Facultad de Filosofía y Letras", City: "Chilpancingo", Aliases: []string{"filosofia y letras"}},
	{Name: "Facultad de Gobierno y Gestión Pública", City: "Chilpancingo", Aliases: []string{"gobierno y gestion publica"}},
	{Name: "Facultad de Matemáticas", City: "Chilpancingo", Aliases: []string{"matematicas"}},
	{Name: "Facultad de Matemáticas 02", City: "Cd. Altamirano", Aliases: []string{"matematicas 2"}},
	{Name: "Facultad de Ciencias de la Educación", City: "Chilpancingo", Aliases: []string{"ciencias de la educacion"}},
	{Name: "Facultad de Ciencias Agropecuarias y Ambientales", City: "Iguala", Aliases: []string{"ciencias agropecuarias y ambientales", "agropecuarias"}},
	{Name: "Facultad de Medicina Veterinaria y Zootecnia 01", City: "Cd. Altamirano", Aliases: []string{"medicina veterinaria y zootecnia", "veterinaria"}},
	{Name: "Facultad de Medicina Veterinaria y Zootecnia 02", City: "Cuajinicuilapa", Aliases: []string{"medicina veterinaria y zootecnia 2", "veterinaria 2"}},
}

var campusAliases = map[string]string{
	"acapulco":                   "Acapulco",
	"acapulco de juarez":         "Acapulco",
	"chilpancingo":               "Chilpancingo",
	"chilpancingo de los bravo":  "Chilpancingo",
	"chilpancingo de los bravos": "Chilpancingo",
	"iguala":                     "Iguala",
	"taxco":                      "Taxco",
	"taxco de alarcon":           "Taxco",
	"taxco el viejo":             "Taxco",
	"zihuatanejo":                "Zihuatanejo",
	"ometepec":                   "Ometepec",
	"cd altamirano":              "Cd. Altamirano",
	"ciudad altamirano":          "Cd. Altamirano",
	"cuajinicuilapa":             "Cuajinicuilapa",
	"tecpan":                     "Tecpan",
	"tierra colorada":            "Tierra Colorada",
	"cruz grande":                "Cruz Grande",
	"huamuxtitlan":               "Huamuxtitlán",
	"tlapa":                      "Tlapa",
	"tlapehuala":                 "Tlapehuala",
	"arcelia":                    "Arcelia",
	"teloloapan":                 "Teloloapan",
	"ajuchitlan":                 "Ajuchitlán",
	"azoyu":                      "Azoyú",
	"la union":                   "La Unión",
	"las vigas":                  "Las Vigas",
	"ayutla":                     "Ayutla",
	"ayutla de los libres":       "Ayutla",
	"el paraiso":                 "El Paraíso",
	"cocula":                     "Cocula",
	"tixtla":                     "Tixtla",
	"marquelia":                  "Marquelia",
	"san luis acatlan":           "San Luis Acatlán",
	"san marcos":                 "San Marcos",
	"atoyac":                     "Atoyac",
	"petatlan":                   "Petatlán",
	"chilapa":                    "Chilapa",
	"zumpango":                   "Zumpango",
	"olinala":                    "Olinalá",
	"buena vista de cuellar":     "Buena Vista de Cuéllar",
	"cutzamala de pinzon":        "Cutzamala de Pinzón",
	"coyuca de benitez":          "Coyuca de Benítez",
	"coyuca de catalan":          "Coyuca de Catalán",
	"apaxtla":                    "Apaxtla",
	"san jeronimo":               "San Jerónimo",
}

func matchFaculty(core string) []Faculty {
	matches := make([]Faculty, 0, 2)
	for _, f := range Faculties {
		for _, alias := range append([]string{coreName(f.Name)}, f.Aliases...) {
			if normalize(alias) == core {
				matches = append(matches, f)
				break
			}
		}
	}
	return matches
}

// Resolve devuelve el nombre canónico y la ciudad de la facultad. Si no hay
// evidencia suficiente, devuelve ok=false para no crear etiquetas inventadas.
// La ciudad de la dependencia nunca se usa como respaldo.
func Resolve(facultad, city string) (string, string, bool) {
	raw := strings.TrimSpace(facultad)
	if raw == "" {
		return "", "", false
	}
	cityCanon, cityOK := CanonicalCity(city)
	core, embeddedCity, embeddedOK := splitFacultyCity(raw)
	if !cityOK && embeddedOK {
		cityCanon, cityOK = embeddedCity, true
	}
	if core == "" {
		return "", "", false
	}
	matches := matchFaculty(core)
	if len(matches) == 0 {
		// Tolera una ciudad pegada al nombre aunque no sea una sede conocida,
		// por ejemplo "Turismo Tijuana". Se prueba quitando palabras finales.
		candidate := core
		for i := 0; i < 3; i++ {
			pos := strings.LastIndex(candidate, " ")
			if pos < 0 {
				break
			}
			candidate = strings.TrimSpace(candidate[:pos])
			if matches = matchFaculty(candidate); len(matches) > 0 {
				break
			}
		}
	}
	if len(matches) > 0 {
		if cityOK {
			for _, f := range matches {
				if f.City == cityCanon {
					return f.Name, f.City, true
				}
			}
		}
		if len(matches) == 1 {
			// Una facultad única manda sobre una ciudad errónea, por ejemplo
			// "Turismo Tijuana": Turismo solo existe en Acapulco.
			return matches[0].Name, matches[0].City, true
		}
		return "", "", false
	}
	// Facultad no catalogada: solo se acepta si el documento dio una ciudad
	// que corresponde a una sede de la UAGro. Nunca se usa la ciudad de la
	// dependencia.
	if !cityOK {
		return "", "", false
	}
	name := displayName(raw)
	if name == "" {
		return "", "", false
	}
	return name, cityCanon, true
}

// CanonicalCity normaliza una ciudad y comprueba que sea una sede de la UAGro.
func CanonicalCity(city string) (string, bool) {
	n := normalize(city)
	if n == "" {
		return "", false
	}
	if c, ok := campusAliases[n]; ok {
		return c, true
	}
	return "", false
}

func splitFacultyCity(facultad string) (core, city string, cityOK bool) {
	n := normalize(facultad)
	// Busca la ciudad al final, de la más larga a la más corta.
	longest := ""
	for alias, canonical := range campusAliases {
		if strings.HasSuffix(n, " "+alias) && len(alias) > len(longest) {
			longest, city = alias, canonical
		}
	}
	if longest != "" {
		n = strings.TrimSpace(strings.TrimSuffix(n, " "+longest))
		cityOK = true
	}
	n = stripFacultyPrefix(n)
	return n, city, cityOK
}

func coreName(name string) string {
	n := normalize(name)
	n = stripFacultyPrefix(n)
	return n
}

func stripFacultyPrefix(s string) string {
	for _, p := range []string{"facultad de ", "facultad ", "escuela superior de ", "escuela de ", "escuela "} {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(strings.TrimPrefix(s, p))
		}
	}
	return s
}

func displayName(facultad string) string {
	n := strings.Join(strings.Fields(facultad), " ")
	// Quita una ciudad pegada al final, si la hubiera.
	low := normalize(n)
	for alias := range campusAliases {
		if strings.HasSuffix(low, " "+alias) {
			n = strings.TrimSpace(n[:len(n)-len(alias)])
			break
		}
	}
	low = normalize(n)
	if strings.HasPrefix(low, "facultad de ") {
		return "Facultad de " + strings.TrimSpace(n[len("facultad de "):])
	}
	if strings.HasPrefix(low, "facultad ") {
		return "Facultad de " + strings.TrimSpace(n[len("facultad "):])
	}
	return "Facultad de " + n
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = norm.NFD.String(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.' || r == ',':
			b.WriteRune(' ')
		}
	}
	fields := strings.Fields(b.String())
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		switch f {
		case "num", "numero", "no":
			continue
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}
