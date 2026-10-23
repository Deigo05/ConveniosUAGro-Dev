// Package ai extrae datos de un convenio usando un servicio compatible con OpenAI.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const maxTextChars = 45000

// Tag is one item of the local catalog sent to the model.
type Tag struct {
	Nombre string
	Tipo   string
}

// Suggestion is a tag proposed by the model.
type Suggestion struct {
	Tipo   string `json:"tipo"`
	Nombre string `json:"nombre"`
	Nueva  bool   `json:"nueva"`
}

// Extraction is the validated result ready to save.
type Extraction struct {
	Dependencia        string
	FechaFirma         string
	FechaVigencia      string
	VigenciaIndefinida bool
	VigenciaCalculada  bool
	VigenciaTexto      string
	Notas              string
	Estado             string
	Sugerencias        []Suggestion
}

// Client calls the configured OpenAI-compatible endpoint.
type Client struct {
	APIKey      string
	BaseURL     string
	Model       string
	VisionModel string
	HTTP        *http.Client
}

func New(apiKey, baseURL, model, visionModel string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.deepseek.com"
	}
	if strings.TrimSpace(model) == "" {
		model = "deepseek-flash"
	}
	if strings.TrimSpace(visionModel) == "" {
		visionModel = model
	}
	return &Client{
		APIKey:      strings.TrimSpace(apiKey),
		BaseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:       strings.TrimSpace(model),
		VisionModel: strings.TrimSpace(visionModel),
		HTTP:        &http.Client{Timeout: 300 * time.Second},
	}
}

// chat hace una petición y devuelve el contenido y el motivo de finalización.
// Permite reintentar con más tokens cuando el modelo de razonamiento se corta.
func (c *Client) chat(ctx context.Context, body map[string]interface{}) (string, string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", "", errors.New("el servicio de IA tardó demasiado en responder")
		}
		return "", "", errors.New("no se pudo conectar con el servicio de IA")
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", "", errors.New("la clave de IA es inválida")
		case http.StatusTooManyRequests:
			return "", "", errors.New("se alcanzó el límite de uso de la IA; intente más tarde")
		case http.StatusBadRequest:
			return "", "", errors.New("el servicio rechazó la solicitud; si es una imagen, revise el modelo de visión en Ajustes")
		default:
			return "", "", fmt.Errorf("el servicio de IA respondió con error (%d)", resp.StatusCode)
		}
	}
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		return "", "", errors.New("el servicio de IA devolvió una respuesta inesperada")
	}
	return apiResp.Choices[0].Message.Content, apiResp.Choices[0].FinishReason, nil
}

const systemPrompt = `Eres un asistente que extrae datos de convenios firmados por la Universidad Autónoma de Guerrero (UAGro), México.
Responde ÚNICAMENTE con un objeto JSON válido, sin texto adicional ni bloques de código, con exactamente estas claves:
{
  "dependencia": string|null,
  "fecha_firma": "YYYY-MM-DD"|null,
  "fecha_firma_texto": string|null,
  "vigencia_tipo": "fecha_exacta"|"duracion"|"indefinida"|"no_encontrada",
  "fecha_vigencia": "YYYY-MM-DD"|null,
  "vigencia_duracion": {"anios": int, "meses": int, "dias": int}|null,
  "vigencia_desde_firma": boolean,
  "vigencia_texto": string|null,
  "confianza": "alta"|"media"|"baja",
  "etiquetas": [{"tipo": "ambito"|"estado"|"ciudad"|"tema", "nombre": string, "nueva": boolean}],
  "notas": string|null
}
Reglas:
- "dependencia": nombre oficial completo de la institución, dependencia u organismo contraparte con quien la UAGro celebra el convenio. NUNCA la propia UAGro ni sus facultades. Si hay varias contrapartes, sepáralas con "; ".
- "fecha_firma": fecha de firma. Convierte fechas escritas en palabras o con meses en español a YYYY-MM-DD. "fecha_firma_texto": fragmento literal.
- Vigencia:
  - Fecha de término concreta: vigencia_tipo="fecha_exacta" y "fecha_vigencia".
  - Duración ("tres años a partir de su firma"): vigencia_tipo="duracion", "vigencia_duracion" y "vigencia_desde_firma"=true solo si cuenta desde la firma. Deja "fecha_vigencia" en null; el sistema la calcula.
  - Indefinida o por tiempo indeterminado: vigencia_tipo="indefinida".
  - "vigencia_texto": copia literal (máximo 300 caracteres) de la cláusula.
- "etiquetas": hasta 8 etiquetas relevantes.
  - ambito: 0 o 1, exactamente "Nacional" o "Internacional".
  - estado: 0 o 1, solo un nombre de la lista de estados existentes.
  - ciudad: 0 a 3. Usa el nombre existente si aparece; si no y hay certeza, propón una nueva con "nueva": true.
  - tema: 0 a 4, solo nombres de temas existentes. No inventes temas.
  - No repitas etiquetas. Si no hay certeza, no la incluyas.
- No inventes datos. Si falta un dato, usa null y explícalo brevemente en "notas".`

// Extract llama al modelo y devuelve los campos ya validados.
func (c *Client) Extract(ctx context.Context, texto string, catalogo []Tag) (Extraction, error) {
	var out Extraction
	if c.APIKey == "" {
		return out, errors.New("no hay una clave de IA configurada")
	}
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return out, errors.New("el PDF no tiene texto seleccionable; capture los datos manualmente")
	}
	if len([]rune(texto)) > maxTextChars {
		runes := []rune(texto)
		texto = string(runes[:maxTextChars])
	}
	prompt := catalogText(catalogo) + "\n\nTexto del convenio:\n<<<\n" + texto + "\n>>>"
	body := map[string]interface{}{
		"model":            c.Model,
		"temperature":      0.1,
		"max_tokens":       16000,
		"reasoning_effort": "high",
		"response_format":  map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
	}
	var content, finish string
	var err error
	for _, limit := range []int{16000, 32000} {
		body["max_tokens"] = limit
		content, finish, err = c.chat(ctx, body)
		if err != nil {
			return out, err
		}
		if finish != "length" {
			raw, err := parseJSON(content)
			if err != nil {
				return out, err
			}
			return interpret(raw, catalogo)
		}
	}
	return out, errors.New("la extracción con IA se cortó por longitud incluso con más tokens; intente con un PDF más corto")
}

const answerPrompt = `Eres un asistente documental de la Universidad Autónoma de Guerrero.
Responde la pregunta usando ÚNICAMENTE los fragmentos de convenios que recibirás.
Reglas:
- Responde en español, de forma clara y breve.
- Cada afirmación debe llevar una cita con el formato [Convenio #N, página P].
- Si los fragmentos no alcanzan para responder, dilo explícitamente y no inventes.
- No des asesoría legal definitiva; presenta la información como apoyo para revisión humana.
- No repitas los fragmentos completos; sintetiza.`

// Answer responde una pregunta con base en fragmentos recuperados.
func (c *Client) Answer(ctx context.Context, question, contexto string) (string, error) {
	if c.APIKey == "" {
		return "", errors.New("no hay una clave de IA configurada")
	}
	if strings.TrimSpace(contexto) == "" {
		return "", errors.New("no se encontraron fragmentos relevantes")
	}
	body := map[string]interface{}{
		"model":       c.Model,
		"temperature": 0.1,
		"max_tokens":  1600,
		"messages": []map[string]string{
			{"role": "system", "content": answerPrompt},
			{"role": "user", "content": "Pregunta:\n" + question + "\n\nFragmentos:\n<<<\n" + contexto + "\n>>>"},
		},
	}
	var content, finish string
	var err error
	for _, limit := range []int{4000, 8000} {
		body["max_tokens"] = limit
		content, finish, err = c.chat(ctx, body)
		if err != nil {
			return "", err
		}
		if finish != "length" {
			texto := strings.TrimSpace(content)
			if texto == "" {
				return "", errors.New("el servicio de IA devolvió una respuesta vacía")
			}
			return texto, nil
		}
	}
	return "", errors.New("la respuesta se cortó por longitud; intenta una pregunta más específica")
}

const transcriptionPrompt = `Eres un transcriptor de documentos. La imagen es una página escaneada de un convenio de la Universidad Autónoma de Guerrero (UAGro), México.
Transcribe de forma literal y completa todo el texto legible, en orden de lectura, conservando párrafos, títulos y la numeración de cláusulas.
Reglas:
- No resumas, no traduzcas, no corrijas la ortografía y no agregues comentarios ni explicaciones.
- Marca las palabras que no se puedan leer como [ilegible].
- Indica firmas, sellos y logotipos brevemente entre corchetes, por ejemplo: [Firma], [Sello].
- Si la página no contiene texto, responde exactamente: [Página sin texto]
Responde únicamente con la transcripción en texto plano, sin bloques de código.`

// TranscribeImage envía una página renderizada al modelo de visión y devuelve
// el texto literal. La imagen debe venir en base64, sin el prefijo data:image/...
func (c *Client) TranscribeImage(ctx context.Context, imageBase64 string, page int) (string, error) {
	if c.APIKey == "" {
		return "", errors.New("no hay una clave de IA configurada")
	}
	if strings.TrimSpace(imageBase64) == "" {
		return "", errors.New("la página no tiene imagen para transcribir")
	}
	body := map[string]interface{}{
		"model":            c.VisionModel,
		"temperature":      0.0,
		"max_tokens":       16000,
		"reasoning_effort": "low",
		"messages": []map[string]interface{}{
			{"role": "system", "content": transcriptionPrompt},
			{"role": "user", "content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("Página %d del convenio.", page)},
				{"type": "image_url", "image_url": map[string]string{
					"url": "data:image/jpeg;base64," + imageBase64,
				}},
			}},
		},
	}
	var content, finish string
	var err error
	for _, limit := range []int{16000, 32000} {
		body["max_tokens"] = limit
		content, finish, err = c.chat(ctx, body)
		if err != nil {
			return "", err
		}
		if finish != "length" {
			texto := strings.TrimSpace(content)
			texto = strings.TrimPrefix(texto, "```text")
			texto = strings.TrimPrefix(texto, "```")
			texto = strings.TrimSuffix(texto, "```")
			texto = strings.TrimSpace(texto)
			if texto == "" {
				return "", errors.New("el servicio de transcripción devolvió texto vacío")
			}
			return texto, nil
		}
	}
	return "", errors.New("la transcripción se cortó por longitud incluso con más tokens; intente de nuevo")
}

func catalogText(tags []Tag) string {
	var b strings.Builder
	b.WriteString("Etiquetas existentes (usa los nombres exactos):\n")
	groups := []struct{ tipo, titulo string }{
		{"ambito", "Ámbito"}, {"estado", "Estado"}, {"ciudad", "Ciudad"}, {"tema", "Tema / Rubro"},
	}
	for _, g := range groups {
		var names []string
		for _, t := range tags {
			if t.Tipo == g.tipo {
				names = append(names, t.Nombre)
			}
		}
		if len(names) == 0 {
			names = []string{"(ninguna)"}
		}
		b.WriteString("- " + g.titulo + ": " + strings.Join(names, ", ") + "\n")
	}
	return b.String()
}

func parseJSON(s string) (map[string]interface{}, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, errors.New("el servicio de IA devolvió un formato inválido")
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(s[start:end+1]), &raw); err != nil {
		return nil, errors.New("el servicio de IA devolvió un formato inválido")
	}
	return raw, nil
}

func interpret(raw map[string]interface{}, catalogo []Tag) (Extraction, error) {
	var out Extraction
	get := func(k string) string {
		if v, ok := raw[k]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return ""
	}
	out.Dependencia = get("dependencia")
	out.VigenciaTexto = truncate(get("vigencia_texto"), 500)
	notas := []string{}
	if n := get("notas"); n != "" {
		notas = append(notas, n)
	}

	firma := parseDate(get("fecha_firma"))
	if firma == "" {
		firma = parseSpanishDate(get("fecha_firma_texto"))
	}
	if firma == "" {
		firma = parseSpanishDate(get("fecha_firma"))
	}
	out.FechaFirma = firma

	tipo := strings.ToLower(get("vigencia_tipo"))
	switch tipo {
	case "fecha_exacta":
		out.FechaVigencia = parseDate(get("fecha_vigencia"))
		if out.FechaVigencia == "" {
			out.FechaVigencia = parseSpanishDate(get("vigencia_texto"))
		}
		if out.FechaVigencia == "" {
			notas = append(notas, "No se pudo leer la fecha exacta de vigencia.")
		}
	case "duracion":
		if dur, ok := raw["vigencia_duracion"].(map[string]interface{}); ok && firma != "" && asBool(raw["vigencia_desde_firma"]) {
			y := asInt(dur["anios"])
			m := asInt(dur["meses"])
			d := asInt(dur["dias"])
			if y+m+d > 0 {
				if t, err := time.Parse("2006-01-02", firma); err == nil {
					end := t.AddDate(y, m, d)
					if end.Year() >= 1950 && end.Year() <= 2100 {
						out.FechaVigencia = end.Format("2006-01-02")
						out.VigenciaCalculada = true
					}
				}
			}
		}
		if out.FechaVigencia == "" {
			notas = append(notas, "La vigencia es una duración que no se pudo convertir en fecha; revise la cláusula.")
		}
	case "indefinida":
		out.VigenciaIndefinida = true
	default:
		// no_encontrada: se deja sin fecha.
	}
	if firma != "" && out.FechaVigencia != "" && out.FechaVigencia < firma {
		notas = append(notas, "La fecha de vigencia es anterior a la fecha de firma; revise los datos.")
	}
	out.Sugerencias = validateSuggestions(raw["etiquetas"], catalogo)
	out.Notas = strings.TrimSpace(strings.Join(notas, " "))
	if out.Dependencia != "" && (out.FechaVigencia != "" || out.VigenciaIndefinida) {
		out.Estado = "ok"
	} else {
		out.Estado = "parcial"
	}
	return out, nil
}

func validateSuggestions(v interface{}, catalogo []Tag) []Suggestion {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}
	byType := map[string]map[string]string{}
	for _, t := range catalogo {
		if byType[t.Tipo] == nil {
			byType[t.Tipo] = map[string]string{}
		}
		byType[t.Tipo][key(t.Nombre)] = t.Nombre
	}
	aliasTipo := map[string]string{
		"ámbito": "ambito", "ambito": "ambito", "estados": "estado", "estado": "estado",
		"ciudades": "ciudad", "ciudad": "ciudad", "temas": "tema", "tema": "tema", "rubro": "tema",
	}
	aliasEstado := map[string]string{
		"michoacan de ocampo": "Michoacán", "coahuila de zaragoza": "Coahuila",
		"veracruz de ignacio de la llave": "Veracruz", "mexico": "Estado de México",
		"edomex": "Estado de México", "cdmx": "Ciudad de México", "distrito federal": "Ciudad de México",
	}
	var out []Suggestion
	seen := map[string]bool{}
	exclusive := map[string]bool{}
	cities, themes := 0, 0
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		tipo := aliasTipo[strings.ToLower(strings.TrimSpace(fmt.Sprint(m["tipo"])))]
		if tipo == "" {
			continue
		}
		nombre := truncate(strings.TrimSpace(fmt.Sprint(m["nombre"])), 80)
		if nombre == "" || strings.ContainsAny(nombre, "\x00\r\n\t") {
			continue
		}
		nueva := false
		switch tipo {
		case "ciudad":
			if cities >= 3 {
				continue
			}
			cities++
			if real, ok := byType[tipo][key(nombre)]; ok {
				nombre = real
			} else {
				nueva = true
			}
		case "tema":
			real, ok := byType[tipo][key(nombre)]
			if !ok || themes >= 4 {
				continue
			}
			themes++
			nombre = real
		default:
			if tipo == "estado" {
				if real, ok := aliasEstado[key(nombre)]; ok {
					nombre = real
				}
			}
			real, ok := byType[tipo][key(nombre)]
			if !ok || exclusive[tipo] {
				continue
			}
			exclusive[tipo] = true
			nombre = real
		}
		k := tipo + "|" + key(nombre)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, Suggestion{Tipo: tipo, Nombre: nombre, Nueva: nueva})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func key(s string) string {
	s = norm.NFD.String(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func asBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.ToLower(strings.TrimSpace(x))
		return x == "true" || x == "si" || x == "sí" || x == "1"
	case float64:
		return x != 0
	}
	return false
}

func asInt(v interface{}) int {
	switch x := v.(type) {
	case float64:
		if x < 0 {
			return 0
		}
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		if n < 0 {
			return 0
		}
		return n
	}
	return 0
}

var (
	reISODate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reDMY     = regexp.MustCompile(`\b(\d{1,2})[\s/.-]+(?:de\s+)?([a-zA-ZáéíóúüÁÉÍÓÚÜ]+|\d{1,2})[\s/.,-]+(?:de\s+|del\s+)?(\d{4})\b`)
)

var meses = map[string]int{
	"enero": 1, "febrero": 2, "marzo": 3, "abril": 4, "mayo": 5, "junio": 6,
	"julio": 7, "agosto": 8, "septiembre": 9, "setiembre": 9, "octubre": 10,
	"noviembre": 11, "diciembre": 12,
}

func parseDate(s string) string {
	s = strings.TrimSpace(s)
	if !reISODate.MatchString(s) {
		return ""
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.Year() < 1950 || t.Year() > 2100 {
		return ""
	}
	return t.Format("2006-01-02")
}

func parseSpanishDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if d := parseDate(s); d != "" {
		return d
	}
	m := reDMY.FindStringSubmatch(s)
	if len(m) != 4 {
		return ""
	}
	day, _ := strconv.Atoi(m[1])
	month := 0
	if n, err := strconv.Atoi(m[2]); err == nil {
		month = n
	} else {
		month = meses[key(m[2])]
	}
	year, _ := strconv.Atoi(m[3])
	if day < 1 || day > 31 || month < 1 || month > 12 || year < 1950 || year > 2100 {
		return ""
	}
	d := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if d.Year() != year || int(d.Month()) != month || d.Day() != day {
		return ""
	}
	return d.Format("2006-01-02")
}
