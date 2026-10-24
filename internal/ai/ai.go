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

	"convenios-uagro-desktop/internal/uagro"

	"golang.org/x/text/unicode/norm"
)

// Límites del texto enviado al modelo. Un convenio típico cabe completo; en los
// largos se conserva el proemio, el cierre con firmas y los párrafos clave.
const (
	maxTextChars  = 32000
	headChars     = 16000
	tailChars     = 9000
	keywordsChars = 7000
)

// Tag is one item of the local catalog sent to the model.
type Tag struct {
	Nombre string
	Tipo   string
	// Estado acompaña a las ciudades para que el modelo no las mezcle.
	Estado string
}

// Suggestion is a tag proposed by the model.
type Suggestion struct {
	Tipo   string `json:"tipo"`
	Nombre string `json:"nombre"`
	Nueva  bool   `json:"nueva"`
}

// Extraction is the validated result ready to save.
type Extraction struct {
	Dependencia         string
	DependenciaCorta    string
	TipoDocumento       string
	FirmanteDependencia string
	CargoDependencia    string
	FirmanteUAGro       string
	CargoUAGro          string
	Facultad            string
	FacultadCiudad      string
	FechaFirma          string
	FechaVigencia       string
	VigenciaIndefinida  bool
	VigenciaCalculada   bool
	VigenciaTexto       string
	Notas               string
	Estado              string
	Sugerencias         []Suggestion
	Correcciones        []Correction
	// Renovacion indica que el documento dice renovar o sustituir un convenio
	// anterior con la misma contraparte; FechaAnterior es la firma de ese
	// convenio si el texto la menciona.
	Renovacion    bool
	FechaAnterior string
	// EsConvenio es false si el archivo no es un convenio (un acta, un
	// oficio, una constancia…); el motivo va en Notas.
	EsConvenio bool
	// Domicilio es el de la contraparte tal como aparece; ayuda a buscar su
	// ciudad en la web, igual que LugarFirma cuando no consta el domicilio.
	Domicilio  string
	LugarFirma string
	// Dudas son contradicciones de lectura en páginas transcritas que el texto
	// no basta para resolver; se revisan después con la imagen de la página.
	Dudas []Doubt
}

// Doubt es un dato que la transcripción leyó de formas distintas (un nombre
// en el proemio y otro bajo la firma, dos fechas de firma).
type Doubt struct {
	Paginas  []int    `json:"paginas"`
	Dato     string   `json:"dato"`
	Lecturas []string `json:"lecturas"`
}

// Correction es un nombre que el modelo considera mal leído en una página
// transcrita; Original y Correcto aparecen tal cual en el texto.
type Correction struct {
	Original string `json:"original"`
	Correcto string `json:"correcto"`
}

// Niveles de razonamiento admitidos para la extracción.
const (
	EffortLow    = "low"
	EffortMedium = "medium"
	EffortHigh   = "high"
)

// NormalizeEffort devuelve un nivel válido; el predeterminado es el bajo,
// que en las pruebas extrajo los mismos datos que el medio en un tercio del
// tiempo.
func NormalizeEffort(e string) string {
	switch strings.ToLower(strings.TrimSpace(e)) {
	case EffortMedium:
		return EffortMedium
	case EffortHigh:
		return EffortHigh
	default:
		return EffortLow
	}
}

// apiEffort traduce el nivel de la aplicación al valor que entiende el
// proveedor. DeepSeek solo distingue low, high y max ("medium" equivale a
// high), así que cada nivel de la aplicación es uno distinto del servicio.
func (c *Client) apiEffort(level string) string {
	level = NormalizeEffort(level)
	if !strings.Contains(strings.ToLower(c.BaseURL), "deepseek") {
		return level
	}
	switch level {
	case EffortMedium:
		return "high"
	case EffortHigh:
		return "max"
	default:
		return "low"
	}
}

// Client calls the configured OpenAI-compatible endpoint.
type Client struct {
	APIKey      string
	BaseURL     string
	Model       string
	VisionModel string
	WebSearch   bool
	// Effort es el nivel de razonamiento de la extracción (low, medium, high).
	Effort string
	// ReferenceDate (AAAA-MM-DD) es la fecha en que se registró el documento:
	// una fecha de firma posterior es un error de lectura.
	ReferenceDate string
	HTTP          *http.Client
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
		WebSearch:   true,
		Effort:      EffortLow,
		HTTP:        sharedHTTP,
	}
}

// sharedHTTP reutiliza las conexiones entre peticiones simultáneas (varias
// páginas y documentos a la vez) en lugar de abrir una por petición.
var sharedHTTP = &http.Client{
	Timeout: 300 * time.Second,
	Transport: func() http.RoundTripper {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.MaxIdleConnsPerHost = 16
		return t
	}(),
}

// Reintentos ante errores pasajeros (servicio saturado, red caída): la
// espera crece en cada intento.
var retryDelays = []time.Duration{3 * time.Second, 10 * time.Second}

type httpStatusError int

func (e httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", int(e)) }

func transient(err error) bool {
	var st httpStatusError
	if errors.As(err, &st) {
		return st == http.StatusTooManyRequests || st >= 500
	}
	return err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// chat hace una petición y devuelve el contenido y el motivo de finalización.
// Permite reintentar con más tokens cuando el modelo de razonamiento se corta.
// Los errores pasajeros (429, 5xx, red) se reintentan con espera creciente.
func (c *Client) chat(ctx context.Context, body map[string]interface{}) (string, string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	var data []byte
	status := 0
	for attempt := 0; ; attempt++ {
		data, status, err = c.post(ctx, payload)
		if err == nil && status >= 200 && status < 300 {
			break
		}
		if err == nil {
			err = httpStatusError(status)
		}
		if attempt >= len(retryDelays) || !transient(err) {
			break
		}
		select {
		case <-ctx.Done():
			return "", "", errors.New("el servicio de IA tardó demasiado en responder")
		case <-time.After(retryDelays[attempt]):
		}
	}
	if err != nil && status == 0 {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", "", errors.New("el servicio de IA tardó demasiado en responder")
		}
		return "", "", errors.New("no se pudo conectar con el servicio de IA")
	}
	if status < 200 || status >= 300 {
		switch status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", "", errors.New("la clave de IA es inválida")
		case http.StatusTooManyRequests:
			return "", "", errors.New("se alcanzó el límite de uso de la IA; intente más tarde")
		case http.StatusBadRequest:
			return "", "", errors.New("el servicio rechazó la solicitud; si es una imagen, revise el modelo de visión en Ajustes")
		default:
			return "", "", fmt.Errorf("el servicio de IA respondió con error (%d)", status)
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

func (c *Client) post(ctx context.Context, payload []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	// Con el servicio saturado, DeepSeek envía líneas vacías mientras espera;
	// json.Unmarshal las ignora como espacio en blanco.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, 0, err
	}
	return data, resp.StatusCode, nil
}

// systemPrompt es breve a propósito: cada regla evita un error observado en
// convenios reales (fechas de nombramiento, testigos, facultad inventada…).
var systemPrompt = `Extraes datos de convenios de la Universidad Autónoma de Guerrero (UAGro), México. Responde solo con un objeto JSON con estas claves:
{"es_convenio":b,"tipo_documento":"Convenio General"|"Convenio Específico"|"Convenio Marco"|"Convenio de Colaboración"|"Convenio Modificatorio"|"Carta de Intención"|"Contrato"|"Acuerdo"|"Otro"|null,"dependencia":s|null,"dependencia_corta":s|null,"domicilio":s|null,"firmante_dependencia":s|null,"cargo_dependencia":s|null,"firmante_uagro":s|null,"cargo_uagro":s|null,"facultad":s|null,"facultad_ciudad":s|null,"fecha_firma":"AAAA-MM-DD"|null,"fecha_firma_texto":s|null,"lugar_firma":s|null,"vigencia_tipo":"fecha_exacta"|"duracion"|"indefinida"|"no_encontrada","fecha_vigencia":"AAAA-MM-DD"|null,"vigencia_duracion":{"anios":n,"meses":n,"dias":n}|null,"vigencia_inicio":"firma"|"AAAA-MM-DD"|null,"vigencia_texto":s|null,"etiquetas":[{"tipo":"ambito"|"estado"|"ciudad"|"tema","nombre":s}],"correcciones":[{"original":s,"correcto":s}],"dudas":[{"paginas":[n],"dato":s,"lecturas":[s]}],"renovacion":{"es":b,"fecha_anterior":"AAAA-MM-DD"|null},"notas":s|null}
Reglas:
0. "es_convenio": false si el archivo no es un convenio, acuerdo, contrato o carta de intención entre la UAGro y otra parte (por ejemplo: acta constitutiva, poder notarial, oficio, constancia, certificado, identificación, factura); di qué es en "notas" y llena solo lo que conste.
1. Partes: búscalas en el proemio ("CONVENIO ... QUE CELEBRAN ...") y en las declaraciones. "dependencia" es la contraparte con su nombre oficial completo; nunca la UAGro ni sus facultades. Varias contrapartes: sepáralas con "; ". "dependencia_corta": sus siglas oficiales si aparecen en el texto ("IMSS", "SEG") o un nombre corto de máximo 40 caracteres ("Ayuntamiento de Acapulco"). "tipo_documento": según el título del documento; "Convenio Modificatorio" incluye addenda. "domicilio": el de la contraparte tal como aparece (máximo 150 caracteres).
2. Firmantes: nombre y cargo de quien firma por cada parte, sin grados ni tratamientos (Dr., Mtra., Lic., C.). Tómalos del bloque de firmas final y compáralos con quien representa a esa parte en el proemio y las declaraciones; si no se leen, usa las declaraciones. Testigos, testigos de honor y notarios no son firmantes.
3. "facultad": solo si el texto dice expresamente que una facultad, escuela o unidad académica de la UAGro celebra, suscribe o participa (o firma su director). Si firma la Rectoría o la universidad en general sin nombrar una unidad académica, usa null. Escribe el nombre oficial del catálogo, sin ciudad: los nombres anteriores o abreviados son la misma unidad ("Unidad Académica de Turismo", "UAGro-FT" → "Facultad de Turismo"; "EMVZ No. 3" → "Escuela Superior de Medicina Veterinaria y Zootecnia 03"). Si no está en el catálogo, cópiala tal como aparece. "facultad_ciudad": sede de la facultad según el catálogo o el domicilio de la UAGro en el documento, nunca la ciudad de la contraparte; null si no consta. Catálogo (nombre y sedes): ` + uagro.CatalogText() + `.
4. "fecha_firma": la del cierre ("firmado en ... a los X días del mes de ... de ..."); nunca fechas de nombramientos, decretos, actas o publicaciones, ni una fecha posterior al registro del documento. Lo marcado [Tachado: ...] no vale. Si el cierre y una anotación de firmas ("las firmas que anteceden corresponden al acuerdo ... en fecha ...") no coinciden, usa la de la anotación (suele ser la que se actualizó al firmar, y el cierre puede estar tachado) y dilo en "notas". "fecha_firma_texto": ese fragmento literal. "lugar_firma": ciudad y estado donde se firma, tal como aparecen. "renovacion.es": true solo si el documento dice que renueva, sustituye o da continuidad a un convenio anterior con la misma contraparte (no basta con citarlo en antecedentes); "fecha_anterior": la firma de ese convenio anterior si consta.
5. Vigencia: fecha de término explícita → "fecha_exacta". Plazo ("tres años") → "duracion" con "vigencia_duracion" y "vigencia_inicio" ("firma" si cuenta desde la firma, o la fecha de inicio indicada); "fecha_vigencia" queda null porque el sistema la calcula. Tiempo indefinido o indeterminado → "indefinida". "vigencia_texto": cita literal de la cláusula, máximo 300 caracteres.
6. "etiquetas" con nombres exactos del catálogo: ambito siempre 1 ("Nacional" si la contraparte es de México, "Internacional" si es de otro país); estado 0-1 (de la contraparte); ciudad 0-1: municipio de la contraparte (nunca el de la UAGro) que pertenezca a ese estado; puede ser nueva. Escribe el nombre oficial del municipio con acentos y corrige errores de lectura evidentes de un municipio que conozcas ("Huachinango, Pue." → "Huauchinango"); si no estás seguro, omítela. tema 0-4 (solo existentes). No incluyas facultades.
7. No inventes: dato ausente o dudoso → null. "notas": máximo una frase con lo que el usuario deba revisar en el documento: un dato clave que no consta (contraparte, fecha de firma o vigencia) o una contradicción que no pudiste resolver. No menciones datos secundarios que falten (domicilio, siglas, testigos) ni errores de lectura que ya resolviste (lo que pongas en "correcciones" no va en "notas"); si no hay nada que revisar, null.`

// correctionsRule se agrega solo cuando el texto incluye páginas transcritas
// por el modelo de visión, para no gastar tokens en PDF con texto nativo.
const correctionsRule = `
8. El texto incluye páginas transcritas desde imágenes ([Página N]) que pueden traer errores de lectura; "[?]" marca palabras dudosas. Lo que peor se lee son los bloques de firmas, los sellos y las anotaciones al margen, porque las firmas y los sellos tapan letras; el proemio y las declaraciones suelen leerse mejor. Una diferencia entre dos lecturas del mismo dato suele ser un error de lectura; solo es una incongruencia del documento si ambas lecturas son claras.
- Nombres: compara cada nombre propio (personas, instituciones, lugares) en todas sus apariciones. Si dos variantes son la misma persona o entidad (la misma parte con el mismo cargo, o el mismo contexto), es un error de lectura aunque difieran muchas letras ("María Elena Ríos Vega" en el proemio y "Mario Elías Ruiz Vega" bajo la firma de la misma parte; "García"/"Garay"; "rn"/"m"): usa en tus campos la variante mejor respaldada (más repetida, del proemio o las declaraciones, o del texto nativo) y agrégala a "correcciones" con el nombre completo (2 o más palabras) tal como aparece: {"original": variante errónea, "correcto": variante buena}. Máximo 10. Si son personas distintas (otra parte u otro cargo), no corrijas.
- Fechas: si la firma aparece con dos fechas distintas, descarta la tachada o la imposible y aplica la regla 4. Los meses y los años escritos con letra se confunden al leer ("septiembre"/"noviembre", "veinticinco"/"veintiséis").
- "dudas": si corregiste un nombre cuyas variantes difieren en más que letras de forma parecida, o elegiste entre dos fechas, agrega {"paginas":[página o páginas donde está la lectura dudosa],"dato":qué hay que verificar (en fechas, también si alguna está tachada),"lecturas":[las variantes tal como aparecen]} para revisarlo con la imagen. Máximo 3.`

const noCorrectionsRule = `
8. "correcciones": [] y "dudas": [].`

// Extract llama al modelo y devuelve los campos ya validados. transcrito indica
// que el texto incluye páginas leídas por visión y activa la revisión de nombres.
func (c *Client) Extract(ctx context.Context, texto string, catalogo []Tag, transcrito bool) (Extraction, error) {
	var out Extraction
	if c.APIKey == "" {
		return out, errors.New("no hay una clave de IA configurada")
	}
	texto = CompactText(texto, maxTextChars)
	if texto == "" {
		return out, errors.New("el PDF no tiene texto seleccionable; capture los datos manualmente")
	}
	system := systemPrompt + noCorrectionsRule
	if transcrito {
		system = systemPrompt + correctionsRule
	}
	prompt := catalogText(catalogo) + "\nTexto del convenio:\n<<<\n" + texto + "\n>>>"
	if c.ReferenceDate != "" {
		// Va en el mensaje del usuario para no romper la caché del prompt fijo.
		prompt = "Fecha de registro del documento: " + c.ReferenceDate + "\n" + prompt
	}
	body := map[string]interface{}{
		"model":            c.Model,
		"temperature":      0.1,
		"reasoning_effort": c.apiEffort(c.Effort),
		"response_format":  map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": prompt},
		},
	}
	limits := []int{12000, 24000}
	if NormalizeEffort(c.Effort) != EffortLow {
		limits = []int{24000, 48000}
	}
	for _, limit := range limits {
		body["max_tokens"] = limit
		content, finish, err := c.chat(ctx, body)
		if err != nil {
			return out, err
		}
		if finish == "length" {
			continue
		}
		raw, err := parseJSON(content)
		if err != nil {
			return out, err
		}
		ext, err := interpret(raw, catalogo)
		if err != nil {
			return out, err
		}
		if transcrito {
			ext.Correcciones = parseCorrections(raw["correcciones"])
			ext.Dudas = parseDoubts(raw["dudas"])
		}
		c.checkSignatureDate(&ext)
		if c.WebSearch {
			c.enrichLocation(ctx, &ext, catalogo)
		}
		return ext, nil
	}
	return out, errors.New("la extracción con IA se cortó por longitud incluso con más tokens; intente con un PDF más corto")
}

// checkSignatureDate avisa cuando la fecha de firma es posterior al registro
// del documento: nadie sube un convenio antes de firmarlo, así que es un error
// de lectura (por ejemplo, «noviembre» en lugar de «septiembre»).
func (c *Client) checkSignatureDate(ext *Extraction) {
	if c.ReferenceDate == "" || ext.FechaFirma == "" || ext.FechaFirma <= c.ReferenceDate {
		return
	}
	shown := ext.FechaFirma
	if t, err := time.Parse("2006-01-02", ext.FechaFirma); err == nil {
		shown = t.Format("02/01/2006")
	}
	note := "La fecha de firma leída (" + shown + ") es posterior al registro del documento; revísela."
	ext.Notas = strings.TrimSpace(ext.Notas + " " + note)
}

func parseDoubts(v interface{}) []Doubt {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var out []Doubt
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		var d Doubt
		d.Dato = truncate(strings.TrimSpace(fmt.Sprint(m["dato"])), 200)
		if pages, ok := m["paginas"].([]interface{}); ok {
			for _, p := range pages {
				if n := asInt(p); n > 0 && len(d.Paginas) < 3 {
					d.Paginas = append(d.Paginas, n)
				}
			}
		}
		if lecturas, ok := m["lecturas"].([]interface{}); ok {
			for _, l := range lecturas {
				if s, ok := l.(string); ok && strings.TrimSpace(s) != "" && len(d.Lecturas) < 4 {
					d.Lecturas = append(d.Lecturas, truncate(strings.TrimSpace(s), 160))
				}
			}
		}
		if d.Dato == "" || d.Dato == "<nil>" || len(d.Paginas) == 0 {
			continue
		}
		out = append(out, d)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func parseCorrections(v interface{}) []Correction {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var out []Correction
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		o, _ := m["original"].(string)
		c, _ := m["correcto"].(string)
		o, c = strings.TrimSpace(o), strings.TrimSpace(c)
		if o == "" || c == "" || o == c || len([]rune(o)) > 120 || len([]rune(c)) > 120 {
			continue
		}
		out = append(out, Correction{Original: o, Correcto: c})
		if len(out) >= 10 {
			break
		}
	}
	return out
}

var (
	reSpaces    = regexp.MustCompile(`[ \t\x{00A0}]+`)
	reBlank     = regexp.MustCompile(`\n{3,}`)
	reKeyClause = regexp.MustCompile(`(?i)vigen|duraci|t[eé]rmino|plazo|firma|facultad|escuela|unidad acad|representad|celebran|domicilio`)
)

// CompactText reduce espacios (que cuestan tokens sin aportar) y, si el texto
// sigue siendo largo, conserva el inicio, el final y los párrafos con palabras
// clave de vigencia, firma y facultad.
func CompactText(texto string, limit int) string {
	lines := strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(reSpaces.ReplaceAllString(l, " "))
	}
	texto = strings.TrimSpace(reBlank.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
	runes := []rune(texto)
	if len(runes) <= limit {
		return texto
	}
	head := string(runes[:headChars])
	tail := string(runes[len(runes)-tailChars:])
	middle := string(runes[headChars : len(runes)-tailChars])
	var keep []string
	used := 0
	for _, p := range strings.Split(middle, "\n\n") {
		p = strings.TrimSpace(p)
		if p == "" || !reKeyClause.MatchString(p) {
			continue
		}
		if r := []rune(p); len(r) > 1500 {
			p = string(r[:1500]) + "…"
		}
		n := len([]rune(p))
		if used+n > keywordsChars {
			break
		}
		keep = append(keep, p)
		used += n
	}
	parts := []string{head, "[…]"}
	if len(keep) > 0 {
		parts = append(parts, strings.Join(keep, "\n\n"), "[…]")
	}
	parts = append(parts, tail)
	return strings.Join(parts, "\n\n")
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

// transcriptionPrompt pide marcar las dudas con [?] para que la extracción
// posterior pueda unificar nombres leídos de forma distinta en otras páginas.
const transcriptionPrompt = `Transcribe la página escaneada de un convenio de la UAGro (México). Devuelve solo el texto en orden de lectura, sin comentarios ni bloques de código.
Reglas:
- Literal y completo: no resumas, no traduzcas ni corrijas ortografía. Conserva títulos, párrafos, numeración de cláusulas y acentos.
- Nombres propios, cargos, cifras y fechas: léelos letra por letra y no los cambies por palabras o apellidos más comunes. Si dudas de una palabra, escribe tu mejor lectura seguida de [?], por ejemplo: Garay[?].
- Ilegible: [ilegible]. Firmas y logotipos: solo [Firma] o [Logotipo], sin describirlos. Sellos: [Sello] o, si su texto se lee, [Sello: texto].
- Texto tachado o cruzado por una línea: [Tachado: texto]. Anotaciones a mano: [Manuscrito: texto].
- Bajo una firma o un sello, el nombre impreso suele estar tapado: léelo con cuidado y, si dudas, márcalo con [?]; no lo sustituyas por otro nombre.
- Tablas: una fila por línea con celdas separadas por " | ".
- Página sin texto: responde exactamente [Página sin texto]`

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
		"reasoning_effort": EffortLow,
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
	// Una página densa usa unos 6000 tokens entre razonamiento y texto; con
	// 12000 casi nunca hace falta el segundo intento, que repite todo.
	for _, limit := range []int{12000, 24000} {
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

// verificationPrompt pide una segunda lectura enfocada en los datos que la
// transcripción leyó de formas distintas.
const verificationPrompt = `Verificas la lectura de una página escaneada de un convenio de la UAGro (México). Una transcripción automática leyó algunos datos de formas distintas en el documento; en esta página pudieron leerse mal (las firmas, sellos y rúbricas suelen tapar letras).
Para cada duda, mira la imagen con atención, letra por letra, en el lugar donde aparece el dato, y fíjate también si ese texto está tachado (cruzado por una línea horizontal) o corregido a mano. Responde solo JSON: {"correcciones":[{"original":s,"correcto":s}]}
- Corrige solo los fragmentos de las dudas. No cambies nada más (ni ortografía, ni gramática, ni otras palabras), aunque veas otros errores.
- "original": el fragmento tal como está en la transcripción de esta página, copiado exacto (de 1 a 12 palabras).
- "correcto": lo que dice la imagen en ese mismo lugar; si ese texto está tachado, "[Tachado: texto]".
- Si la transcripción coincide con la imagen o no se distingue, devuelve {"correcciones":[]}.`

// VerifyPage vuelve a mirar una página con las dudas de lectura del documento
// y devuelve las correcciones que la imagen confirma.
func (c *Client) VerifyPage(ctx context.Context, imageBase64 string, page int, transcripcion string, dudas []Doubt) ([]Correction, error) {
	if c.APIKey == "" {
		return nil, errors.New("no hay una clave de IA configurada")
	}
	if strings.TrimSpace(imageBase64) == "" || len(dudas) == 0 {
		return nil, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Página %d.\nDudas:\n", page)
	for _, d := range dudas {
		b.WriteString("- " + d.Dato)
		if len(d.Lecturas) > 0 {
			b.WriteString(": se leyó «" + strings.Join(d.Lecturas, "» y «") + "»")
		}
		b.WriteString("\n")
	}
	b.WriteString("Transcripción actual de esta página:\n<<<\n" + truncate(transcripcion, 12000) + "\n>>>")
	body := map[string]interface{}{
		"model":            c.VisionModel,
		"temperature":      0.0,
		"reasoning_effort": EffortLow,
		"response_format":  map[string]string{"type": "json_object"},
		"messages": []map[string]interface{}{
			{"role": "system", "content": verificationPrompt},
			{"role": "user", "content": []map[string]interface{}{
				{"type": "text", "text": b.String()},
				{"type": "image_url", "image_url": map[string]string{
					"url": "data:image/jpeg;base64," + imageBase64,
				}},
			}},
		},
	}
	for _, limit := range []int{8000, 16000} {
		body["max_tokens"] = limit
		content, finish, err := c.chat(ctx, body)
		if err != nil {
			return nil, err
		}
		if finish == "length" {
			continue
		}
		raw, err := parseJSON(content)
		if err != nil {
			return nil, err
		}
		return relevantCorrections(parseCorrections(raw["correcciones"]), dudas), nil
	}
	return nil, errors.New("la verificación con la imagen se cortó por longitud")
}

// relevantCorrections descarta lo que el modelo corrigió fuera de las dudas
// (por ejemplo, la gramática del documento): el fragmento original debe
// compartir una palabra significativa con alguna de las lecturas en duda.
func relevantCorrections(cors []Correction, dudas []Doubt) []Correction {
	words := map[string]bool{}
	for _, d := range dudas {
		for _, l := range d.Lecturas {
			for _, w := range significantWords(l) {
				words[w] = true
			}
		}
	}
	var out []Correction
	for _, c := range cors {
		for _, w := range significantWords(c.Original) {
			if words[w] {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// significantWords devuelve las palabras de 4 letras o más, sin acentos ni
// signos, más los números ("4", "2026").
func significantWords(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(key(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) >= 4 || (w[0] >= '0' && w[0] <= '9') {
			out = append(out, w)
		}
	}
	return out
}

// catalogText lista las etiquetas que el modelo puede usar. Las facultades no
// se envían: el sistema crea su etiqueta a partir del campo "facultad".
func catalogText(tags []Tag) string {
	var b strings.Builder
	b.WriteString("Catálogo de etiquetas:\n")
	groups := []struct{ tipo, titulo string }{
		{"ambito", "Ámbito"}, {"estado", "Estado"}, {"ciudad", "Ciudad"}, {"tema", "Tema"},
	}
	for _, g := range groups {
		if g.tipo == "ciudad" {
			b.WriteString("- Ciudad (por estado): " + citiesByState(tags) + "\n")
			continue
		}
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

// citiesByState agrupa las ciudades del catálogo: "Guerrero: Acapulco de
// Juárez, Ometepec; Puebla: Huauchinango".
func citiesByState(tags []Tag) string {
	var states []string
	byState := map[string][]string{}
	for _, t := range tags {
		if t.Tipo != "ciudad" {
			continue
		}
		st := t.Estado
		if st == "" {
			st = "sin estado"
		}
		if _, ok := byState[st]; !ok {
			states = append(states, st)
		}
		byState[st] = append(byState[st], t.Nombre)
	}
	if len(states) == 0 {
		return "(ninguna)"
	}
	parts := make([]string, 0, len(states))
	for _, st := range states {
		parts = append(parts, st+": "+strings.Join(byState[st], ", "))
	}
	return strings.Join(parts, "; ")
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
	out.Dependencia = truncate(get("dependencia"), 500)
	out.DependenciaCorta = truncate(get("dependencia_corta"), 60)
	out.TipoDocumento = truncate(get("tipo_documento"), 80)
	out.Domicilio = truncate(get("domicilio"), 200)
	out.LugarFirma = truncate(get("lugar_firma"), 120)
	// Si el modelo no lo dice, se asume que es un convenio.
	out.EsConvenio = true
	if v, ok := raw["es_convenio"]; ok && v != nil {
		out.EsConvenio = asBool(v)
	}
	if ren, ok := raw["renovacion"].(map[string]interface{}); ok {
		out.Renovacion = asBool(ren["es"])
		if v, ok := ren["fecha_anterior"].(string); ok {
			out.FechaAnterior = parseDate(v)
		}
	}
	out.FirmanteDependencia = truncate(get("firmante_dependencia"), 200)
	out.CargoDependencia = truncate(get("cargo_dependencia"), 200)
	out.FirmanteUAGro = truncate(get("firmante_uagro"), 200)
	out.CargoUAGro = truncate(get("cargo_uagro"), 200)
	out.Facultad = truncate(get("facultad"), 200)
	out.FacultadCiudad = truncate(get("facultad_ciudad"), 100)
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
		// El plazo cuenta desde la firma o desde una fecha de inicio explícita.
		inicio := parseDate(get("vigencia_inicio"))
		if v := strings.ToLower(get("vigencia_inicio")); v == "firma" || (v == "" && asBool(raw["vigencia_desde_firma"])) {
			inicio = firma
		}
		if dur, ok := raw["vigencia_duracion"].(map[string]interface{}); ok && inicio != "" {
			y := asInt(dur["anios"])
			m := asInt(dur["meses"])
			d := asInt(dur["dias"])
			if y+m+d > 0 {
				if t, err := time.Parse("2006-01-02", inicio); err == nil {
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
		"ciudades": "ciudad", "ciudad": "ciudad", "facultades": "facultad", "facultad": "facultad", "temas": "tema", "tema": "tema", "rubro": "tema",
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
		if tipo == "" || tipo == "facultad" {
			// La etiqueta de facultad la decide el sistema con el campo "facultad".
			continue
		}
		nombre := truncate(strings.TrimSpace(fmt.Sprint(m["nombre"])), 80)
		if nombre == "" || strings.ContainsAny(nombre, "\x00\r\n\t") {
			continue
		}
		nueva := false
		switch tipo {
		case "ciudad":
			if cities >= 1 {
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
