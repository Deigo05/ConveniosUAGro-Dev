package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFileNameUsesAgreementData(t *testing.T) {
	c := ConventionSummary{
		ArchivoNombreOriginal: "Escaner123515.pdf",
		Dependencia:           "Instituto Mexicano del Seguro Social",
		DependenciaCorta:      "IMSS",
		TipoDocumento:         "Convenio Específico",
		FechaFirma:            "2024-03-15",
		Etiquetas:             []Tag{{Tipo: "estado", Nombre: "Guerrero"}, {Tipo: "facultad", Nombre: "Facultad de Medicina Acapulco"}},
	}
	if got, want := FileName(c), "2024-03-15 Convenio Específico - IMSS - Facultad de Medicina Acapulco.pdf"; got != want {
		t.Fatalf("FileName = %q, want %q", got, want)
	}

	c.Etiquetas = []Tag{{Tipo: "facultad", Nombre: TagConvenioAbierto}}
	c.FechaFirma, c.TipoDocumento, c.DependenciaCorta = "", "", ""
	if got, want := FileName(c), "Sin fecha Convenio - Instituto Mexicano del Seguro Social - Convenio Abierto.pdf"; got != want {
		t.Fatalf("FileName = %q, want %q", got, want)
	}
}

func TestFileNameKeepsOriginalUntilIdentified(t *testing.T) {
	c := ConventionSummary{ArchivoNombreOriginal: "Escaner123515.PDF"}
	if got := FileName(c); got != "Escaner123515.PDF" {
		t.Fatalf("FileName = %q", got)
	}
	if got := FileName(ConventionSummary{}); got != "convenio.pdf" {
		t.Fatalf("FileName vacío = %q", got)
	}
}

func TestFileNameIsSafeAndShort(t *testing.T) {
	c := ConventionSummary{
		Dependencia: `Secretaría de Educación Pública del Gobierno del Estado de Guerrero: Dirección "General" de Educación Media Superior y Superior; Ayuntamiento`,
		FechaFirma:  "2023-01-02",
		Facultad:    "Facultad de Derecho",
	}
	got := FileName(c)
	if strings.ContainsAny(got, `<>:"/\|?*`) {
		t.Fatalf("caracteres no válidos: %q", got)
	}
	if !strings.HasPrefix(got, "2023-01-02 Convenio - Secretaría de Educación Pública") || !strings.HasSuffix(got, " y otras - Facultad de Derecho.pdf") {
		t.Fatalf("FileName = %q", got)
	}
	if n := utf8.RuneCountInString(got); n > maxFileStem+4 {
		t.Fatalf("nombre de %d caracteres: %q", n, got)
	}
	if strings.Contains(got, " de y otras") || strings.Contains(got, " del y otras") {
		t.Fatalf("termina en conector: %q", got)
	}
}

func TestNormalizeTipoDocumento(t *testing.T) {
	cases := map[string]string{
		"convenio general de colaboración": "Convenio General",
		"CONVENIO ESPECIFICO":              "Convenio Específico",
		"Addendum al convenio":             "Convenio Modificatorio",
		"Carta de intención":               "Carta de Intención",
		"contrato de prestación":           "Contrato",
		"Convenio de colaboración":         "Convenio de Colaboración",
		"otro":                             "Otro",
		"memorándum":                       "",
		"":                                 "",
	}
	for in, want := range cases {
		if got := NormalizeTipoDocumento(in); got != want {
			t.Errorf("NormalizeTipoDocumento(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueFileName(t *testing.T) {
	used := map[string]bool{}
	for i, want := range []string{"a.pdf", "A (2).pdf", "a (3).pdf"} {
		name := "a.pdf"
		if i == 1 {
			name = "A.pdf"
		}
		if got := UniqueFileName(name, used); got != want {
			t.Fatalf("UniqueFileName #%d = %q, want %q", i, got, want)
		}
	}
}

func TestSavedAgreementGetsFileName(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "Convenio de prueba")
	if err := st.Update(ConventionInput{ID: id, Dependencia: "Universidad de Sonora", DependenciaCorta: "UNISON", TipoDocumento: "convenio marco", FechaFirma: "2022-05-10"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Update(ConventionInput{ID: id, TipoDocumento: "memorándum"}); err == nil {
		t.Fatal("se aceptó un tipo de documento inválido")
	}
	c, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if c.TipoDocumento != "Convenio Marco" || !strings.HasPrefix(c.ArchivoNombre, "2022-05-10 Convenio Marco - UNISON") {
		t.Fatalf("ficha: tipo %q, archivo %q", c.TipoDocumento, c.ArchivoNombre)
	}
	res, err := st.List(ListFilter{Query: "UNISON"})
	if err != nil || res.Total != 1 || res.Items[0].ArchivoNombre != c.ArchivoNombre {
		t.Fatalf("listado: %+v, %v", res.Items, err)
	}
}
