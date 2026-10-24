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
	// Primary marca la sede principal de una facultad con varias sedes; se usa
	// cuando el documento no dice la ciudad.
	Primary bool
}

// Faculties es un catálogo de las unidades académicas de nivel superior de la
// UAGro. Los nombres se normalizan en Resolve: "Unidad Académica de Turismo",
// "Escuela Superior de Turismo" o "UAGro-FT" llevan a la misma facultad.
var Faculties = []Faculty{
	{Name: "Facultad de Turismo", City: "Acapulco", Aliases: []string{"turismo"}, Primary: true},
	{Name: "Facultad de Turismo", City: "Zihuatanejo", Aliases: []string{"turismo"}},
	{Name: "Facultad de Derecho", City: "Acapulco", Aliases: []string{"derecho"}},
	{Name: "Facultad de Derecho", City: "Chilpancingo", Aliases: []string{"derecho"}},
	{Name: "Facultad de Medicina", City: "Acapulco", Aliases: []string{"medicina"}},
	{Name: "Facultad de Odontología", City: "Acapulco", Aliases: []string{"odontologia"}},
	{Name: "Facultad de Psicología", City: "Acapulco", Aliases: []string{"psicologia"}},
	{Name: "Facultad de Sociología", City: "Acapulco", Aliases: []string{"sociologia"}},
	{Name: "Facultad de Ciencias Sociales", City: "Acapulco", Aliases: []string{"ciencias sociales"}},
	{Name: "Facultad de Economía", City: "Acapulco", Aliases: []string{"economia", "ciencias economicas"}},
	{Name: "Facultad de Ecología Marina", City: "Acapulco", Aliases: []string{"ecologia marina"}},
	{Name: "Facultad de Ciencias Ambientales", City: "Acapulco", Aliases: []string{"ciencias ambientales"}},
	{Name: "Facultad de Contaduría y Administración", City: "Acapulco", Aliases: []string{"contaduria y administracion", "contaduria", "administracion"}},
	{Name: "Facultad de Lenguas Extranjeras", City: "Acapulco", Aliases: []string{"lenguas extranjeras"}},
	{Name: "Facultad de Ciencias y Tecnologías de la Información", City: "Acapulco", Aliases: []string{"ciencias y tecnologias de la informacion", "tecnologias de la informacion"}},
	{Name: "Facultad de Enfermería 02", City: "Acapulco", Aliases: []string{"enfermeria 2"}},
	{Name: "Facultad de Ciencias de la Educación 02", City: "Acapulco", Aliases: []string{"ciencias de la educacion 2"}},
	{Name: "Facultad de Matemáticas", City: "Acapulco", Aliases: []string{"matematicas"}},
	{Name: "Escuela Superior de Nutrición y Ciencia de los Alimentos", City: "Acapulco", Aliases: []string{"nutricion y ciencia de los alimentos", "nutricion y ciencias de los alimentos", "nutricion"}},
	{Name: "Facultad de Ingeniería", City: "Chilpancingo", Aliases: []string{"ingenieria"}},
	{Name: "Facultad de Arquitectura y Urbanismo", City: "Chilpancingo", Aliases: []string{"arquitectura y urbanismo", "arquitectura"}},
	{Name: "Facultad de Ciencias Naturales", City: "Chilpancingo", Aliases: []string{"ciencias naturales"}},
	{Name: "Facultad de Ciencias Químico Biológicas", City: "Chilpancingo", Aliases: []string{"ciencias quimico biologicas", "quimico biologicas"}},
	{Name: "Facultad de Comunicación y Mercadotecnia", City: "Chilpancingo", Aliases: []string{"comunicacion y mercadotecnia", "ciencias de la comunicacion"}},
	{Name: "Facultad de Filosofía y Letras", City: "Chilpancingo", Aliases: []string{"filosofia y letras"}},
	{Name: "Facultad de Gobierno y Gestión Pública", City: "Chilpancingo", Aliases: []string{"gobierno y gestion publica"}},
	{Name: "Facultad de Matemáticas", City: "Chilpancingo", Aliases: []string{"matematicas"}, Primary: true},
	{Name: "Facultad de Matemáticas 02", City: "Cd. Altamirano", Aliases: []string{"matematicas 2"}},
	{Name: "Facultad de Ciencias de la Educación", City: "Chilpancingo", Aliases: []string{"ciencias de la educacion"}, Primary: true},
	{Name: "Facultad de Enfermería 01", City: "Chilpancingo", Aliases: []string{"enfermeria 1"}},
	{Name: "Facultad de Antropología Social", City: "Tixtla", Aliases: []string{"antropologia social", "antropologia"}},
	{Name: "Facultad de Ciencias Agropecuarias y Ambientales", City: "Iguala", Aliases: []string{"ciencias agropecuarias y ambientales", "agropecuarias"}},
	{Name: "Facultad de Medicina Veterinaria y Zootecnia 01", City: "Cd. Altamirano", Aliases: []string{"medicina veterinaria y zootecnia 1", "veterinaria 1"}},
	{Name: "Facultad de Medicina Veterinaria y Zootecnia 02", City: "Cuajinicuilapa", Aliases: []string{"medicina veterinaria y zootecnia 2", "veterinaria 2"}},
	{Name: "Escuela Superior de Medicina Veterinaria y Zootecnia 03", City: "Tecpan de Galeana", Aliases: []string{"medicina veterinaria y zootecnia 3", "veterinaria 3"}},
}

var campusAliases = map[string]string{
	"acapulco":                   "Acapulco",
	"acapulco de juarez":         "Acapulco",
	"llano largo":                "Acapulco",
	"chilpancingo":               "Chilpancingo",
	"chilpancingo de los bravo":  "Chilpancingo",
	"chilpancingo de los bravos": "Chilpancingo",
	"petaquillas":                "Chilpancingo",
	"iguala":                     "Iguala",
	"iguala de la independencia": "Iguala",
	"taxco":                      "Taxco",
	"taxco de alarcon":           "Taxco",
	"taxco el viejo":             "Taxco",
	"zihuatanejo":                "Zihuatanejo",
	"zihuatanejo de azueta":      "Zihuatanejo",
	"ometepec":                   "Ometepec",
	"cd altamirano":              "Cd. Altamirano",
	"ciudad altamirano":          "Cd. Altamirano",
	"cuajinicuilapa":             "Cuajinicuilapa",
	"tecpan":                     "Tecpan de Galeana",
	"tecpan de galeana":          "Tecpan de Galeana",
	"tierra colorada":            "Tierra Colorada",
	"cruz grande":                "Cruz Grande",
	"huamuxtitlan":               "Huamuxtitlán",
	"tlapa":                      "Tlapa",
	"tlapa de comonfort":         "Tlapa",
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
	"tixtla de guerrero":         "Tixtla",
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

// Prefijos de unidad académica que se ignoran al comparar con el catálogo.
var unitPrefixes = []string{
	"unidad academica de la facultad de", "unidad academica de", "unidad academica",
	"escuela superior de", "escuela superior", "facultad de", "facultad",
	"escuela de", "escuela", "la", "de", "ua",
}

// Siglas frecuentes en los documentos, por ejemplo "EMVZ No. 3".
var unitAbbreviations = map[string]string{
	"mvz": "medicina veterinaria y zootecnia", "emvz": "medicina veterinaria y zootecnia",
	"esmvz": "medicina veterinaria y zootecnia", "fmvz": "medicina veterinaria y zootecnia",
	"uamvz": "medicina veterinaria y zootecnia", "fca": "contaduria y administracion",
	"fcqb": "ciencias quimico biologicas", "fcti": "ciencias y tecnologias de la informacion",
}

// Palabras con las que empieza el nombre de una unidad académica. Si el
// campo facultad no empieza así ni está en el catálogo (por ejemplo,
// "Rectoría"), no se considera una facultad.
var unitKinds = []string{"facultad", "escuela", "unidad academica", "instituto", "centro"}

// key separa el nombre normalizado y sin prefijos de su número de unidad:
// "Facultad de Enfermería No. 2" → ("enfermeria", "2").
func key(s string) (base, num string) {
	n := stripPrefixes(normalize(s))
	fields := strings.Fields(n)
	if len(fields) > 0 {
		if exp, ok := unitAbbreviations[fields[0]]; ok {
			fields[0] = exp
		}
	}
	if len(fields) > 0 && isNumber(fields[len(fields)-1]) {
		num = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " "), num
}

func stripPrefixes(n string) string {
	for changed := true; changed; {
		changed = false
		for _, p := range unitPrefixes {
			if n == p {
				return ""
			}
			if strings.HasPrefix(n, p+" ") {
				n, changed = strings.TrimSpace(n[len(p):]), true
				break
			}
		}
	}
	for _, suf := range []string{" de la universidad autonoma de guerrero", " de la uagro", " uagro"} {
		n = strings.TrimSuffix(n, suf)
	}
	return n
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// candidates devuelve las facultades con ese nombre. Si el documento no da
// número de unidad ("Enfermería"), sirven todas las numeradas.
func candidates(base, num string) []Faculty {
	var out []Faculty
	for _, f := range Faculties {
		for _, alias := range append([]string{f.Name}, f.Aliases...) {
			b, n := key(alias)
			if b == base && (n == num || num == "") {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// Resolve devuelve el nombre canónico y la sede de la facultad. ok=false
// significa que el texto no nombra una unidad académica (el convenio es
// abierto). city puede quedar vacía si la facultad tiene varias sedes y el
// documento no dice cuál; la ciudad de la contraparte nunca se usa.
func Resolve(facultad, city string) (name, sede string, ok bool) {
	raw := strings.TrimSpace(facultad)
	if raw == "" {
		return "", "", false
	}
	cityCanon, cityOK := CanonicalCity(city)
	rest, embeddedCity, embeddedOK := splitCity(raw)
	if !cityOK && embeddedOK {
		cityCanon, cityOK = embeddedCity, true
	}
	base, num := key(rest)
	if base == "" {
		return "", "", false
	}
	matches := candidates(base, num)
	if len(matches) == 0 {
		// Tolera palabras de más al final, por ejemplo "Turismo Tijuana".
		words := strings.Fields(base)
		for i := len(words) - 1; i > 0 && len(matches) == 0 && len(words)-i <= 3; i-- {
			matches = candidates(strings.Join(words[:i], " "), num)
		}
	}
	if len(matches) > 0 {
		return pick(matches, cityCanon, cityOK)
	}
	n := normalize(raw)
	isUnit := false
	for _, kind := range unitKinds {
		if strings.HasPrefix(n, kind+" ") {
			isUnit = true
			break
		}
	}
	if !isUnit {
		return "", "", false
	}
	if !cityOK {
		cityCanon = ""
	}
	return displayName(rest), cityCanon, true
}

// pick elige entre las sedes posibles de una facultad del catálogo.
func pick(matches []Faculty, city string, cityOK bool) (string, string, bool) {
	if cityOK {
		for _, f := range matches {
			if f.City == city {
				return f.Name, f.City, true
			}
		}
	}
	if len(matches) == 1 {
		// Una facultad con una sola sede manda sobre una ciudad errónea, por
		// ejemplo "Turismo Tijuana" o la ciudad de la contraparte.
		return matches[0].Name, matches[0].City, true
	}
	for _, f := range matches {
		if f.Primary {
			return f.Name, f.City, true
		}
	}
	name := matches[0].Name
	for _, f := range matches[1:] {
		if f.Name != name {
			// Varias unidades numeradas ("Enfermería 01", "02"…) y ninguna
			// pista: se usa el nombre común sin número ni sede.
			return withoutNumber(name), "", true
		}
	}
	return name, "", true
}

func withoutNumber(name string) string {
	words := strings.Fields(name)
	if len(words) > 1 && isNumber(words[len(words)-1]) {
		words = words[:len(words)-1]
	}
	return strings.Join(words, " ")
}

// CanonicalCity normaliza una ciudad y comprueba que sea una sede de la UAGro.
func CanonicalCity(city string) (string, bool) {
	n := normalize(city)
	for _, suf := range []string{" guerrero", " gro"} {
		n = strings.TrimSuffix(n, suf)
	}
	if n == "" {
		return "", false
	}
	if c, ok := campusAliases[n]; ok {
		return c, true
	}
	return "", false
}

// splitCity quita una sede pegada al final del nombre ("Turismo Acapulco").
func splitCity(facultad string) (rest, city string, ok bool) {
	n := normalize(facultad)
	longest := ""
	for alias, canonical := range campusAliases {
		if strings.HasSuffix(n, " "+alias) && len(alias) > len(longest) {
			longest, city = alias, canonical
		}
	}
	if longest == "" {
		return facultad, "", false
	}
	// Recorta el texto original por palabras para conservar acentos.
	words := strings.Fields(facultad)
	cut := len(strings.Fields(longest))
	if cut >= len(words) {
		return facultad, "", false
	}
	return strings.Join(words[:len(words)-cut], " "), city, true
}

// displayName da formato a una unidad que no está en el catálogo: conserva
// "Escuela Superior", "Instituto"…, convierte "Unidad Académica de X" en
// "Facultad de X" (su nombre actual) y escribe el número como "03".
func displayName(s string) string {
	words := strings.Fields(strings.TrimSpace(s))
	out := make([]string, 0, len(words))
	for i, w := range words {
		n := simplify(w)
		switch {
		case n == "no" || n == "num" || n == "numero" || n == "n" || n == "nro" || n == "n0":
			continue
		case isNumber(n) && i == len(words)-1:
			n = strings.TrimLeft(n, "0")
			if len(n) < 2 {
				n = "0" + n
			}
			out = append(out, n)
		default:
			out = append(out, w)
		}
	}
	name := strings.Join(out, " ")
	low := normalize(name)
	for _, p := range []string{"unidad academica de ", "unidad academica "} {
		if strings.HasPrefix(low, p) {
			cut := len(strings.Fields(p))
			return "Facultad de " + strings.Join(strings.Fields(name)[cut:], " ")
		}
	}
	for _, kind := range unitKinds {
		if low == kind || strings.HasPrefix(low, kind+" ") {
			return strings.ToUpper(name[:1]) + name[1:]
		}
	}
	return "Facultad de " + name
}

// simplify pasa a minúsculas sin acentos y cambia la puntuación por espacios.
func simplify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = norm.NFD.String(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.' || r == ',' || r == '"' || r == '\'':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// normalize es simplify sin las marcas de número ("No.", "Núm.").
func normalize(s string) string {
	fields := strings.Fields(simplify(s))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		switch f {
		case "num", "numero", "no", "n", "nro", "n0":
			continue
		}
		if isNumber(f) {
			f = strings.TrimLeft(f, "0")
			if f == "" {
				f = "0"
			}
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}

// CatalogText resume el catálogo para el modelo: "Turismo (Acapulco,
// Zihuatanejo); Derecho (Acapulco, Chilpancingo)…". Va en el mensaje de
// sistema, que no cambia, para aprovechar la caché del proveedor.
func CatalogText() string {
	var order []string
	cities := map[string][]string{}
	for _, f := range Faculties {
		if _, ok := cities[f.Name]; !ok {
			order = append(order, f.Name)
		}
		cities[f.Name] = append(cities[f.Name], f.City)
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, name+" ("+strings.Join(cities[name], ", ")+")")
	}
	return strings.Join(parts, "; ")
}
