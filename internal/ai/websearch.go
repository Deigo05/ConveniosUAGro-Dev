package ai

import (
	"context"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	reResult  = regexp.MustCompile(`(?is)<a[^>]*class="result__a"[^>]*>(.*?)</a>`)
	reSnippet = regexp.MustCompile(`(?is)<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	reTags    = regexp.MustCompile(`(?is)<[^>]+>`)
	reDoubt   = regexp.MustCompile(`\[\?\]|\[ilegible\]`)
)

// DuckDuckGo responde con un reto anti-robots si recibe muchas búsquedas
// seguidas. Por eso las búsquedas van de una en una, con una pausa mínima, se
// guardan en caché y, si aparece el reto, se dejan de intentar un rato.
const (
	searchGap     = 2500 * time.Millisecond
	searchBackoff = 15 * time.Minute
)

var search struct {
	sync.Mutex
	last         time.Time
	blockedUntil time.Time
	cache        map[string]string
}

// searchWeb consulta DuckDuckGo HTML y devuelve títulos y fragmentos. No requiere
// clave. ok es false si no se pudo consultar (sin conexión o con el reto
// anti-robots), para distinguirlo de una búsqueda sin resultados.
func (c *Client) searchWeb(ctx context.Context, query string) (results string, ok bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", false
	}
	search.Lock()
	defer search.Unlock()
	if r, ok := search.cache[query]; ok {
		return r, true
	}
	now := time.Now()
	if now.Before(search.blockedUntil) {
		return "", false
	}
	if wait := searchGap - now.Sub(search.last); wait > 0 {
		select {
		case <-ctx.Done():
			return "", false
		case <-time.After(wait):
		}
	}
	search.last = time.Now()
	body, ok := fetchSearch(ctx, query)
	if !ok {
		return "", false
	}
	if strings.Contains(body, "anomaly-modal") || strings.Contains(body, "bots use DuckDuckGo") {
		log.Printf("la búsqueda web pidió verificación anti-robots; se pausa %s", searchBackoff)
		search.blockedUntil = time.Now().Add(searchBackoff)
		return "", false
	}
	results = parseResults(body)
	if search.cache == nil {
		search.cache = map[string]string{}
	}
	search.cache[query] = results
	return results, true
}

func fetchSearch(ctx context.Context, query string) (string, bool) {
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ConveniosUAGro/1.0")
	req.Header.Set("Accept-Language", "es-MX,es;q=0.9")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(data), true
}

func parseResults(body string) string {
	titles := reResult.FindAllStringSubmatch(body, 5)
	snippets := reSnippet.FindAllStringSubmatch(body, 5)
	var b strings.Builder
	for i := 0; i < len(titles) && i < 5; i++ {
		title := cleanHTML(titles[i][1])
		snippet := ""
		if i < len(snippets) {
			snippet = cleanHTML(snippets[i][1])
		}
		if title == "" && snippet == "" {
			continue
		}
		b.WriteString("- " + title)
		if snippet != "" {
			b.WriteString(": " + snippet)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// locationCatalog envía solo ámbitos, estados y ciudades: los temas no hacen
// falta para ubicar a la contraparte y ahorran tokens.
func locationCatalog(tags []Tag) string {
	var loc []Tag
	for _, t := range tags {
		if t.Tipo == "ambito" || t.Tipo == "estado" || t.Tipo == "ciudad" {
			loc = append(loc, t)
		}
	}
	return catalogText(loc)
}

func cleanHTML(s string) string {
	s = reTags.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

// locationQuery arma la búsqueda con el nombre (las siglas si el nombre es
// muy largo) y el domicilio del documento: "C.B.T.i.s. No. 86 Huauchinango".
// Sin domicilio usa el lugar de firma, que evita confundir a la contraparte
// con otra del mismo nombre en otro estado.
func locationQuery(ext *Extraction) string {
	name := strings.TrimSpace(strings.Split(ext.Dependencia, ";")[0])
	if short := strings.TrimSpace(ext.DependenciaCorta); short != "" && len([]rune(name)) > 70 {
		name = short
	}
	if name == "" {
		return ""
	}
	dom := strings.Join(strings.Fields(reDoubt.ReplaceAllString(ext.Domicilio, " ")), " ")
	if dom != "" {
		return truncate(name+" "+dom, 220)
	}
	if lugar := strings.TrimSpace(ext.LugarFirma); lugar != "" {
		return truncate(name+" "+lugar, 220)
	}
	return name + " dirección ciudad estado México"
}

// brandQuery busca por los nombres comerciales con el domicilio o el lugar de
// firma: "PREGO Salads & Wings Acapulco de Juárez, Guerrero". Se usa cuando la
// razón social no aparece en internet.
func brandQuery(ext *Extraction) string {
	var marcas []string
	for _, m := range strings.FieldsFunc(ext.Marcas, func(r rune) bool { return r == ';' || r == ',' }) {
		if m = strings.TrimSpace(m); m != "" && len(marcas) < 3 {
			marcas = append(marcas, m)
		}
	}
	if len(marcas) == 0 {
		return ""
	}
	q := strings.Join(marcas, " ")
	if dom := strings.Join(strings.Fields(reDoubt.ReplaceAllString(ext.Domicilio, " ")), " "); dom != "" {
		q += " " + dom
	} else if lugar := strings.TrimSpace(ext.LugarFirma); lugar != "" {
		q += " " + lugar
	}
	return truncate(q, 220)
}

// locationOf devuelve el estado y la ciudad sugeridos.
func locationOf(list []Suggestion) (estado, ciudad string) {
	for _, s := range list {
		switch s.Tipo {
		case "estado":
			estado = s.Nombre
		case "ciudad":
			ciudad = s.Nombre
		}
	}
	return estado, ciudad
}

// enrichLocation busca en la web si falta la ciudad o el estado, si la ciudad
// sería nueva o si el domicilio se leyó con dudas, y pide al modelo que
// confirme la ubicación con los resultados. Busca primero por la razón social
// y, si no se confirma, por las marcas. Nunca inventa si no hay evidencia, y
// si la ubicación no viene del documento lo dice en las notas. Devuelve
// unavailable si hacía falta buscar y no se pudo consultar internet.
func (c *Client) enrichLocation(ctx context.Context, ext *Extraction, catalogo []Tag) (unavailable bool) {
	if !ext.EsConvenio {
		return false
	}
	hasState, hasCity, uncertainCity := false, false, false
	for _, s := range ext.Sugerencias {
		switch s.Tipo {
		case "estado":
			hasState = true
		case "ciudad":
			hasCity = true
			if s.Nueva {
				uncertainCity = true
			}
		}
	}
	if hasState && hasCity && !uncertainCity && !reDoubt.MatchString(ext.Domicilio) {
		return false
	}
	var refined []Suggestion
	tried := map[string]bool{}
	consulted := false
	for _, query := range []string{locationQuery(ext), brandQuery(ext)} {
		if query == "" || tried[query] {
			continue
		}
		tried[query] = true
		results, ok := c.searchWeb(ctx, query)
		consulted = consulted || ok
		if results == "" {
			continue
		}
		if refined = c.confirmLocation(ctx, ext, results, catalogo); len(refined) > 0 {
			break
		}
	}
	if len(refined) == 0 {
		return !consulted
	}
	antesEstado, antesCiudad := locationOf(ext.Sugerencias)
	// Conserva etiquetas no ubicacionales y reemplaza las de ubicación por las refinadas.
	var merged []Suggestion
	for _, s := range ext.Sugerencias {
		if s.Tipo != "ambito" && s.Tipo != "estado" && s.Tipo != "ciudad" {
			merged = append(merged, s)
		}
	}
	seen := map[string]bool{}
	for _, s := range refined {
		if s.Tipo != "ambito" && s.Tipo != "estado" && s.Tipo != "ciudad" {
			continue
		}
		k := s.Tipo + "|" + strings.ToLower(s.Nombre)
		if seen[k] {
			continue
		}
		seen[k] = true
		merged = append(merged, s)
	}
	// Si la web solo confirmó parte de la ubicación, conserva lo demás.
	for _, s := range ext.Sugerencias {
		if (s.Tipo == "ambito" || s.Tipo == "estado") && !hasType(merged, s.Tipo) {
			merged = append(merged, s)
		}
	}
	ext.Sugerencias = merged
	// Avisa cuando la ubicación salió de internet y no del documento: sin
	// domicilio que la respalde, o con otro estado que el que se leyó.
	estado, ciudad := locationOf(merged)
	cambio := !strings.EqualFold(estado, antesEstado) || !strings.EqualFold(ciudad, antesCiudad)
	if cambio && (ext.Domicilio == "" || (antesEstado != "" && !strings.EqualFold(estado, antesEstado))) {
		lugar := estado
		if ciudad != "" && estado != "" {
			lugar = ciudad + ", " + estado
		} else if ciudad != "" {
			lugar = ciudad
		}
		if lugar != "" {
			ext.Notas = strings.TrimSpace(ext.Notas + " Ubicación de la contraparte tomada de internet (no consta en el documento): " + lugar + "; revísela.")
		}
	}
	return false
}

// confirmLocation pide al modelo la ubicación de la contraparte según los
// resultados web, solo si hablan claramente de la misma entidad.
func (c *Client) confirmLocation(ctx context.Context, ext *Extraction, results string, catalogo []Tag) []Suggestion {
	datos := []string{"Contraparte: " + ext.Dependencia}
	if ext.Marcas != "" {
		datos = append(datos, "Marcas: "+ext.Marcas)
	}
	if ext.FirmanteDependencia != "" {
		datos = append(datos, "Representante: "+ext.FirmanteDependencia)
	}
	domicilio := ext.Domicilio
	if domicilio == "" {
		domicilio = "(no consta)"
	}
	datos = append(datos, "Domicilio en el documento (puede traer errores de lectura): "+domicilio)
	if ext.LugarFirma != "" {
		datos = append(datos, "Lugar de firma: "+ext.LugarFirma)
	}
	prompt := fmt.Sprintf(`%s
Resultados web:
%s

Devuelve {"etiquetas":[{"tipo":"ambito"|"estado"|"ciudad","nombre":s}]} con la ubicación de la contraparte (nunca la de la UAGro): máximo una de cada tipo, nombres exactos del catálogo. La ciudad es el municipio con su nombre oficial y acentos, debe pertenecer al estado y puede ser nueva.
Un resultado vale solo si es claramente la misma entidad: coincide su razón social o una de sus marcas y, si es un nombre que pueden tener varios negocios ("Jardín de Eventos Lua", "Hotel Las Palmas"), también otro dato del documento (domicilio, representante o una ubicación compatible con el lugar de firma). Directorios genéricos, otra entidad con un nombre parecido o resultados que no la ubican no valen: devuelve {"etiquetas":[]}.
%s`, strings.Join(datos, "\n"), results, locationCatalog(catalogo))
	body := map[string]interface{}{
		"model":            c.Model,
		"temperature":      0.0,
		"max_tokens":       4000,
		"reasoning_effort": c.apiEffort(EffortLow),
		"response_format":  map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": "Ubicas geográficamente a la contraparte de un convenio. Responde solo JSON."},
			{"role": "user", "content": prompt},
		},
	}
	content, finish, err := c.chat(ctx, body)
	if err != nil || finish == "length" {
		return nil
	}
	raw, err := parseJSON(content)
	if err != nil {
		return nil
	}
	return validateSuggestions(raw["etiquetas"], catalogo)
}

func hasType(list []Suggestion, tipo string) bool {
	for _, s := range list {
		if s.Tipo == tipo {
			return true
		}
	}
	return false
}
