// Package fechas comprueba, sin llamar a la IA, si las fechas que capturó la
// IA coinciden con lo que dice el texto del convenio. Busca la fecha del
// cierre («…lo firman… a los quince días del mes de…») y la fecha de término
// de la cláusula de vigencia, las convierte a AAAA-MM-DD y las compara con las
// fechas guardadas. Cuando no coinciden, cuando la transcripción marcó la
// fecha como dudosa o cuando la fecha en letra y la fecha en número no son la
// misma, devuelve un Result: la fecha queda «por confirmar».
//
// Solo afirma lo que lee: si no encuentra el cierre, no marca nada.
package fechas

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Campos que se comprueban.
const (
	Firma    = "firma"
	Vigencia = "vigencia"
)

// Date es una fecha completa (día, mes y año) encontrada en un texto.
type Date struct {
	// Start y End delimitan la fecha en el texto original (bytes).
	Start, End int
	// ISO es la fecha como AAAA-MM-DD; vacía si la fecha no existe (31 de junio).
	ISO string
	// Texto es el fragmento original, tal como está escrito.
	Texto string
	// Conflicto indica que la fecha en letra y su número entre paréntesis no
	// coinciden («veinticinco (26)»), o que la fecha no existe.
	Conflicto bool
	// posiciones en el texto normalizado
	ns, ne int
}

// Input son los datos de un convenio que se comprueban.
type Input struct {
	Texto              string
	FechaFirma         string
	FechaVigencia      string
	VigenciaTexto      string
	VigenciaIndefinida bool
	VigenciaCalculada  bool
}

// Result es una fecha que conviene confirmar con el documento.
type Result struct {
	Campo string `json:"campo"`
	// Motivo explica, en una frase para el usuario, por qué.
	Motivo string `json:"motivo"`
	// Propuesta es la fecha que dice el documento (AAAA-MM-DD), si se pudo leer.
	Propuesta string `json:"propuesta,omitempty"`
	// Cita es el fragmento del documento en que se basa.
	Cita string `json:"cita,omitempty"`
}

/* ---------- Normalización ---------- */

// normText es el texto en minúsculas, sin acentos ni signos y con espacios
// simples, con la posición original de cada byte.
type normText struct {
	s     string
	start []int // inicio en el original del carácter que produjo cada byte
	end   []int // fin en el original de ese carácter
}

func normalize(src string) normText {
	var b strings.Builder
	var start, end []int
	lastSpace := true
	for i, r := range src {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		for _, d := range norm.NFKD.String(string(r)) {
			if unicode.Is(unicode.Mn, d) {
				continue
			}
			d = unicode.ToLower(d)
			if unicode.IsLetter(d) || unicode.IsDigit(d) {
				if d >= utf8.RuneSelf {
					d = 'x'
				}
				b.WriteByte(byte(d))
				start = append(start, i)
				end = append(end, i+size)
				lastSpace = false
			} else if !lastSpace {
				b.WriteByte(' ')
				start = append(start, i)
				end = append(end, i+size)
				lastSpace = true
			}
		}
	}
	return normText{s: b.String(), start: start, end: end}
}

// original devuelve el rango original de s[a:b].
func (n normText) original(a, b int) (int, int) {
	return n.start[a], n.end[b-1]
}

/* ---------- Fechas ---------- */

var numberWords = map[string]int{
	"un": 1, "uno": 1, "una": 1, "primero": 1, "primer": 1, "dos": 2, "tres": 3, "cuatro": 4,
	"cinco": 5, "seis": 6, "siete": 7, "ocho": 8, "nueve": 9, "diez": 10, "once": 11, "doce": 12,
	"trece": 13, "catorce": 14, "quince": 15, "dieciseis": 16, "diecisiete": 17, "dieciocho": 18,
	"diecinueve": 19, "veinte": 20, "veintiun": 21, "veintiuno": 21, "veintiuna": 21, "veintidos": 22,
	"veintitres": 23, "veinticuatro": 24, "veinticinco": 25, "veintiseis": 26, "veintisiete": 27,
	"veintiocho": 28, "veintinueve": 29, "treinta": 30, "cuarenta": 40, "cincuenta": 50,
	"sesenta": 60, "setenta": 70, "ochenta": 80, "noventa": 90,
}

var months = map[string]time.Month{
	"enero": 1, "febrero": 2, "marzo": 3, "abril": 4, "mayo": 5, "junio": 6, "julio": 7,
	"agosto": 8, "septiembre": 9, "setiembre": 9, "octubre": 10, "noviembre": 11, "diciembre": 12,
}

func alternation(m map[string]int) string {
	words := make([]string, 0, len(m))
	for w := range m {
		words = append(words, w)
	}
	sort.Slice(words, func(i, j int) bool { return len(words[i]) > len(words[j]) || (len(words[i]) == len(words[j]) && words[i] < words[j]) })
	return strings.Join(words, "|")
}

var (
	reDate    *regexp.Regexp
	reNumeric = regexp.MustCompile(`\b(\d{1,2}) (\d{1,2}) (\d{4})\b`)
	// Marcas de la transcripción: palabra dudosa o ilegible.
	reDoubt = regexp.MustCompile(`(?i)\[\?\]|\[ilegible\]`)
	reDoubtOrStamp = regexp.MustCompile(`(?i)\[\?\]|\[ilegible\]|\[sello`)
	// Rastro de una fecha en un cierre: un mes o «días del mes».
	reDateTrace = regexp.MustCompile(`\b(dias? del mes|enero|febrero|marzo|abril|mayo|junio|julio|agosto|septiembre|setiembre|octubre|noviembre|diciembre)\b`)
	// Palabras que anuncian la fecha de término en la cláusula de vigencia.
	reEndWord = regexp.MustCompile(`\b(hasta|al|concluira|concluye|terminara|termina|vencera|vence|finalizara|fenecera|fenece)\b`)
)

func init() {
	w := "(?:" + alternation(numberWords) + ")"
	wn := w + "(?: y " + w + ")?"
	ms := make(map[string]int, len(months))
	for k, v := range months {
		ms[k] = int(v)
	}
	reDate = regexp.MustCompile(`\b(?:(?P<dw>` + wn + `)|(?P<dd>\d{1,2})o?)(?: (?P<dp>\d{1,2}))?(?: dias?)?(?: del mes)? de (?P<m>` +
		alternation(ms) + `) (?:de|del)(?: ano)? (?:(?P<yw>(?:dos mil|mil novecientos)(?: ` + wn + `)?)|(?P<yd>\d{4}))(?: (?P<yp>\d{4}))?\b`)
}

// wordsValue convierte «treinta y uno» en 31.
func wordsValue(s string) int {
	total := 0
	for _, p := range strings.Split(s, " y ") {
		total += numberWords[strings.TrimSpace(p)]
	}
	return total
}

func yearWords(s string) int {
	switch {
	case strings.HasPrefix(s, "dos mil"):
		return 2000 + wordsValue(strings.TrimSpace(strings.TrimPrefix(s, "dos mil")))
	case strings.HasPrefix(s, "mil novecientos"):
		return 1900 + wordsValue(strings.TrimSpace(strings.TrimPrefix(s, "mil novecientos")))
	}
	return 0
}

func iso(y int, m time.Month, d int) string {
	if y < 1950 || y > 2100 || d < 1 {
		return ""
	}
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d || t.Month() != m {
		return ""
	}
	return t.Format("2006-01-02")
}

func findDates(n normText, src string) []Date {
	var out []Date
	names := reDate.SubexpNames()
	for _, m := range reDate.FindAllStringSubmatchIndex(n.s, -1) {
		g := map[string]string{}
		for i, name := range names {
			if name != "" && m[2*i] >= 0 {
				g[name] = n.s[m[2*i]:m[2*i+1]]
			}
		}
		conflict := false
		day := 0
		switch {
		case g["dd"] != "":
			day, _ = strconv.Atoi(g["dd"])
		case g["dw"] != "":
			day = wordsValue(g["dw"])
		}
		if g["dp"] != "" {
			p, _ := strconv.Atoi(g["dp"])
			conflict = conflict || p != day
			day = p
		}
		year := 0
		switch {
		case g["yd"] != "":
			year, _ = strconv.Atoi(g["yd"])
		case g["yw"] != "":
			year = yearWords(g["yw"])
		}
		if g["yp"] != "" {
			p, _ := strconv.Atoi(g["yp"])
			conflict = conflict || p != year
			year = p
		}
		out = append(out, makeDate(n, src, m[0], m[1], year, months[g["m"]], day, conflict))
	}
	for _, m := range reNumeric.FindAllStringSubmatchIndex(n.s, -1) {
		overlaps := false
		for _, d := range out {
			if m[0] < d.ne && d.ns < m[1] {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		d, _ := strconv.Atoi(n.s[m[2]:m[3]])
		mo, _ := strconv.Atoi(n.s[m[4]:m[5]])
		y, _ := strconv.Atoi(n.s[m[6]:m[7]])
		if mo < 1 || mo > 12 || iso(y, time.Month(mo), d) == "" {
			continue // una serie de números que no es fecha
		}
		out = append(out, makeDate(n, src, m[0], m[1], y, time.Month(mo), d, false))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ns < out[j].ns })
	return out
}

func makeDate(n normText, src string, a, b, y int, m time.Month, d int, conflict bool) Date {
	s, e := n.original(a, b)
	date := Date{Start: s, End: e, Texto: strings.TrimSpace(src[s:e]), ns: a, ne: b, Conflicto: conflict}
	date.ISO = iso(y, m, d)
	if date.ISO == "" {
		date.Conflicto = true
	}
	if date.Conflicto {
		date.ISO = ""
	}
	return date
}

// FindDates devuelve las fechas completas del texto, en orden.
func FindDates(src string) []Date {
	return findDates(normalize(src), src)
}

/* ---------- Comprobación ---------- */

// Fórmulas con que se anuncia la firma, ya normalizadas (las mismas que usa la
// revisión puntual en frontend/dist/revision.js).
var closingFormulas = []string{
	"dias del mes de", "dia del mes de", "lo firman", "la firman", "firman por",
	"firman de conformidad", "firmado en", "firmada en", "se firma", "suscriben",
	"para constancia", "firmas que anteceden",
}

func occurrences(s, phrase string) []int {
	var out []int
	for i := strings.Index(s, phrase); i >= 0; {
		e := i + len(phrase)
		if (i == 0 || s[i-1] == ' ') && (e == len(s) || s[e] == ' ') {
			out = append(out, i)
		}
		next := strings.Index(s[i+1:], phrase)
		if next < 0 {
			break
		}
		i += 1 + next
	}
	return out
}

// lastClosing es la posición (normalizada) de la última fórmula de firma, o -1.
func lastClosing(s string) int {
	last := -1
	for _, f := range closingFormulas {
		for _, p := range occurrences(s, f) {
			if p > last {
				last = p
			}
		}
	}
	return last
}

// closingDate elige la fecha del cierre: la primera completa después de la
// última fórmula de firma (hasta 400 caracteres), o la más cercana antes de
// ella (hasta 200).
func closingDate(dates []Date, p int) (Date, bool) {
	for _, d := range dates {
		if d.ns >= p && d.ns <= p+400 {
			return d, true
		}
	}
	for i := len(dates) - 1; i >= 0; i-- {
		d := dates[i]
		if d.ns < p && d.ne >= p-200 {
			return d, true
		}
	}
	return Date{}, false
}

func hasDoubt(src string, d Date) bool {
	a := max(0, d.Start-40)
	b := min(len(src), d.End+40)
	return reDoubt.MatchString(src[a:b])
}

func quote(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		s = string(r[:160]) + "…"
	}
	return s
}

func checkFirma(in Input) (Result, bool) {
	n := normalize(in.Texto)
	p := lastClosing(n.s)
	if p < 0 {
		return Result{}, false
	}
	r := Result{Campo: Firma}
	d, ok := closingDate(findDates(n, in.Texto), p)
	if !ok {
		// Hay cierre pero sin fecha legible. Solo cuenta si el cierre está en la
		// parte final del documento y la IA de todos modos puso una fecha.
		if in.FechaFirma == "" || p < len(n.s)*2/5 {
			return Result{}, false
		}
		// Debe haber rastro de una fecha que no se pudo leer completa: un mes,
		// «días del mes» o una marca de la transcripción. Una simple mención
		// («el firmado en 2021») no es un cierre ilegible.
		s, _ := n.original(p, min(len(n.s), p+1))
		window := n.s[p:min(len(n.s), p+300)]
		if !reDateTrace.MatchString(window) && !reDoubtOrStamp.MatchString(in.Texto[s:min(len(in.Texto), s+300)]) {
			return Result{}, false
		}
		r.Cita = quote(in.Texto[s:min(len(in.Texto), s+220)])
		r.Motivo = "No se encontró una fecha legible en el cierre del documento; compare la fecha con el PDF."
		return r, true
	}
	r.Cita = quote(d.Texto)
	switch {
	case hasDoubt(in.Texto, d):
		r.Motivo = fmt.Sprintf("En la transcripción, la fecha del cierre está marcada como difícil de leer: «%s».", r.Cita)
	case d.Conflicto:
		r.Motivo = fmt.Sprintf("En el cierre, la fecha en letra y en número no coinciden o no existe: «%s».", r.Cita)
	case in.FechaFirma == "":
		r.Motivo = fmt.Sprintf("No se capturó la fecha de firma; el cierre dice «%s».", r.Cita)
		r.Propuesta = d.ISO
	case in.FechaFirma != d.ISO:
		r.Motivo = fmt.Sprintf("El cierre dice «%s», distinto de la fecha capturada.", r.Cita)
		r.Propuesta = d.ISO
	default:
		return Result{}, false
	}
	return r, true
}

// endDates son las fechas de la cláusula que van después de «hasta», «al»,
// «concluirá»…: las fechas de término, no las de inicio.
func endDates(clause string) []Date {
	n := normalize(clause)
	var out []Date
	for _, d := range findDates(n, clause) {
		before := n.s[max(0, d.ns-40):d.ns]
		if reEndWord.MatchString(before) {
			out = append(out, d)
		}
	}
	return out
}

func checkVigencia(in Input, firmaPorConfirmar bool) (Result, bool) {
	if in.VigenciaIndefinida {
		return Result{}, false
	}
	r := Result{Campo: Vigencia}
	if in.VigenciaCalculada {
		if firmaPorConfirmar && in.FechaVigencia != "" {
			r.Motivo = "Se calculó a partir de la fecha de firma, que está por confirmar."
			return r, true
		}
	} else if ends := endDates(in.VigenciaTexto); len(ends) > 0 {
		match := false
		for _, d := range ends {
			if d.ISO != "" && d.ISO == in.FechaVigencia {
				match = true
			}
		}
		d := ends[len(ends)-1]
		r.Cita = quote(d.Texto)
		switch {
		case hasDoubt(in.VigenciaTexto, d):
			r.Motivo = fmt.Sprintf("En la transcripción, la fecha de término está marcada como difícil de leer: «%s».", r.Cita)
			return r, true
		case match:
		case d.Conflicto:
			r.Motivo = fmt.Sprintf("En la cláusula, la fecha en letra y en número no coinciden o no existe: «%s».", r.Cita)
			return r, true
		case in.FechaVigencia == "":
			r.Motivo = fmt.Sprintf("No se capturó la fecha de vigencia; la cláusula dice «%s».", r.Cita)
			r.Propuesta = d.ISO
			return r, true
		default:
			r.Motivo = fmt.Sprintf("La cláusula dice «%s», distinto de la fecha capturada.", r.Cita)
			r.Propuesta = d.ISO
			return r, true
		}
	}
	if in.FechaFirma != "" && in.FechaVigencia != "" && in.FechaVigencia < in.FechaFirma {
		return Result{Campo: Vigencia, Motivo: "La fecha de vigencia es anterior a la fecha de firma."}, true
	}
	return Result{}, false
}

// Check compara las fechas del convenio con su texto y devuelve las que
// conviene confirmar (como máximo una por campo).
func Check(in Input) []Result {
	var out []Result
	firma, okFirma := checkFirma(in)
	if okFirma {
		out = append(out, firma)
	}
	if v, ok := checkVigencia(in, okFirma); ok {
		out = append(out, v)
	}
	return out
}
