package ai

import "testing"

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
