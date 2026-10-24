package ai

import (
	"strings"
	"testing"
)

func TestParseSpanishDate(t *testing.T) {
	cases := map[string]string{
		"15 de marzo de 2024":     "2024-03-15",
		"15/03/2024":              "2024-03-15",
		"3 de septiembre de 2021": "2021-09-03",
		"2024-03-15":              "2024-03-15",
		"":                        "",
		"31 de febrero de 2024":   "",
	}
	for in, want := range cases {
		if got := parseSpanishDate(in); got != want {
			t.Errorf("parseSpanishDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateSuggestions(t *testing.T) {
	catalogo := []Tag{
		{Nombre: "Nacional", Tipo: "ambito"},
		{Nombre: "Guerrero", Tipo: "estado"},
		{Nombre: "Michoacán", Tipo: "estado"},
		{Nombre: "Medicina", Tipo: "tema"},
		{Nombre: "Acapulco", Tipo: "ciudad"},
	}
	items := []interface{}{
		map[string]interface{}{"tipo": "ambito", "nombre": "Nacional"},
		map[string]interface{}{"tipo": "ambito", "nombre": "Internacional"},
		map[string]interface{}{"tipo": "estado", "nombre": "Michoacán de Ocampo"},
		map[string]interface{}{"tipo": "tema", "nombre": "Medicina"},
		map[string]interface{}{"tipo": "tema", "nombre": "Inventado"},
		map[string]interface{}{"tipo": "ciudad", "nombre": "Acapulco"},
		map[string]interface{}{"tipo": "ciudad", "nombre": "Nueva Ciudad", "nueva": true},
	}
	got := validateSuggestions(items, catalogo)
	if len(got) != 4 {
		t.Fatalf("sugerencias = %+v, want 4", got)
	}
	if got[0].Nombre != "Nacional" || got[0].Tipo != "ambito" {
		t.Fatalf("primera sugerencia: %+v", got[0])
	}
	cities := 0
	for _, s := range got {
		if s.Tipo == "ciudad" {
			cities++
			if s.Nombre != "Acapulco" {
				t.Fatalf("ciudad inesperada: %+v", s)
			}
		}
	}
	if cities != 1 {
		t.Fatalf("se conservaron %d ciudades: %+v", cities, got)
	}
}

func TestInterpret(t *testing.T) {
	raw := map[string]interface{}{
		"dependencia":          "Ayuntamiento de Acapulco",
		"fecha_firma":          "2024-03-15",
		"vigencia_tipo":        "duracion",
		"vigencia_duracion":    map[string]interface{}{"anios": float64(3)},
		"vigencia_desde_firma": true,
		"notas":                "Documento de prueba",
	}
	got, err := interpret(raw, []Tag{{Nombre: "Nacional", Tipo: "ambito"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.FechaVigencia != "2027-03-15" || !got.VigenciaCalculada {
		t.Fatalf("vigencia = %+v", got)
	}
	if got.Estado != "ok" {
		t.Fatalf("estado = %s", got.Estado)
	}
}

func TestInterpretDurationFromExplicitStart(t *testing.T) {
	raw := map[string]interface{}{
		"dependencia":       "Secretaría de Salud",
		"fecha_firma":       "2024-03-15",
		"vigencia_tipo":     "duracion",
		"vigencia_duracion": map[string]interface{}{"anios": float64(2)},
		"vigencia_inicio":   "2024-06-01",
	}
	got, err := interpret(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.FechaVigencia != "2026-06-01" || !got.VigenciaCalculada {
		t.Fatalf("vigencia = %q calculada=%v", got.FechaVigencia, got.VigenciaCalculada)
	}
	raw["vigencia_inicio"] = "firma"
	got, _ = interpret(raw, nil)
	if got.FechaVigencia != "2026-03-15" {
		t.Fatalf("vigencia desde firma = %q", got.FechaVigencia)
	}
}

func TestFacultySuggestionsAreIgnored(t *testing.T) {
	catalogo := []Tag{{Nombre: "Facultad de Turismo Acapulco", Tipo: "facultad"}, {Nombre: "Nacional", Tipo: "ambito"}}
	got := validateSuggestions([]interface{}{
		map[string]interface{}{"tipo": "facultad", "nombre": "Facultad de Turismo Acapulco"},
		map[string]interface{}{"tipo": "ambito", "nombre": "Nacional"},
	}, catalogo)
	if len(got) != 1 || got[0].Tipo != "ambito" {
		t.Fatalf("sugerencias = %+v", got)
	}
}

func TestCompactTextKeepsKeyClauses(t *testing.T) {
	if got := CompactText("  Hola   mundo \r\n\r\n\r\n\r\nfin  ", 1000); got != "Hola mundo\n\nfin" {
		t.Fatalf("compactado = %q", got)
	}
	relleno := strings.Repeat("Texto de relleno sin datos relevantes. ", 600)
	texto := "PROEMIO " + relleno + "\n\nDÉCIMA. VIGENCIA. El presente convenio tendrá una duración de tres años.\n\n" + relleno + "\n\nFIRMAS"
	got := CompactText(texto, maxTextChars)
	if n := len([]rune(got)); n > maxTextChars+keywordsChars {
		t.Fatalf("texto demasiado largo: %d", n)
	}
	for _, want := range []string{"PROEMIO", "DÉCIMA. VIGENCIA", "FIRMAS"} {
		if !strings.Contains(got, want) {
			t.Fatalf("falta %q en el texto compactado", want)
		}
	}
}

func TestParseCorrections(t *testing.T) {
	got := parseCorrections([]interface{}{
		map[string]interface{}{"original": "Juan Ramón Garay", "correcto": "Juan Ramón García"},
		map[string]interface{}{"original": "igual", "correcto": "igual"},
		map[string]interface{}{"original": "", "correcto": "x"},
		"basura",
	})
	if len(got) != 1 || got[0].Correcto != "Juan Ramón García" {
		t.Fatalf("correcciones = %+v", got)
	}
}

func TestParseDoubts(t *testing.T) {
	got := parseDoubts([]interface{}{
		map[string]interface{}{"paginas": []interface{}{3.0, 4.0}, "dato": "fecha de firma", "lecturas": []interface{}{"4 de noviembre", "04 de septiembre"}},
		map[string]interface{}{"paginas": []interface{}{}, "dato": "sin página"},
		map[string]interface{}{"paginas": []interface{}{2.0}, "dato": nil},
		"basura",
	})
	if len(got) != 1 || len(got[0].Paginas) != 2 || got[0].Paginas[0] != 3 || len(got[0].Lecturas) != 2 {
		t.Fatalf("dudas = %+v", got)
	}
}

// Nadie sube un convenio antes de firmarlo: una firma posterior al registro
// es un error de lectura y se avisa en las notas.
func TestSignatureAfterRegistrationIsFlagged(t *testing.T) {
	c := &Client{ReferenceDate: "2026-10-24"}
	ext := Extraction{FechaFirma: "2026-11-04"}
	c.checkSignatureDate(&ext)
	if !strings.Contains(ext.Notas, "04/11/2026") {
		t.Fatalf("notas = %q", ext.Notas)
	}
	ok := Extraction{FechaFirma: "2026-09-04"}
	c.checkSignatureDate(&ok)
	if ok.Notas != "" {
		t.Fatalf("una fecha anterior no lleva nota: %q", ok.Notas)
	}
}

func TestLocationQueryUsesSigningPlace(t *testing.T) {
	q := locationQuery(&Extraction{Dependencia: "Jardín de Eventos Lúa", LugarFirma: "Acapulco de Juárez, Guerrero"})
	if q != "Jardín de Eventos Lúa Acapulco de Juárez, Guerrero" {
		t.Fatalf("consulta = %q", q)
	}
}

// La revisión con la imagen solo puede tocar lo que estaba en duda.
func TestRelevantCorrections(t *testing.T) {
	dudas := []Doubt{
		{Paginas: []int{3}, Dato: "firmante", Lecturas: []string{"Jacqueline Cerón Falcón (propietaria)", "Joaquín Zorro Falcón (propietario)"}},
		{Paginas: []int{3, 4}, Dato: "fecha de firma", Lecturas: []string{"4 de noviembre de 2026", "04 de septiembre de 2026"}},
	}
	got := relevantCorrections([]Correction{
		{Original: "Dr. Joaquín Zorro Falcón", Correcto: "Dra. Jacqueline Cerón Falcón"},
		{Original: "4 días del mes de noviembre del año dos mil veintiséis", Correcto: "[Tachado: 4 días del mes de noviembre del año dos mil veinticinco]"},
		{Original: "del acuerdo Modificatoria", Correcto: "del acuerdo Modificatorio"},
		{Original: "respetar las condición que", Correcto: "respetar la condición que"},
	}, dudas)
	if len(got) != 2 {
		t.Fatalf("correcciones = %+v", got)
	}
}
