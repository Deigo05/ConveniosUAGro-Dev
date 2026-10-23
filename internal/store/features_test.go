package store

import (
	"strings"
	"testing"
	"time"

	"convenios-uagro-desktop/internal/pdfutil"
)

func TestRiskRadarAndRelated(t *testing.T) {
	st, _ := testStore(t)
	tags, _ := st.ListTags()
	var nacional, guerrero, acapulco int64
	for _, tag := range tags {
		switch {
		case tag.Tipo == "ambito" && tag.Nombre == "Nacional":
			nacional = tag.ID
		case tag.Tipo == "estado" && tag.Nombre == "Guerrero":
			guerrero = tag.ID
		case tag.Tipo == "ciudad" && tag.Nombre == "Acapulco":
			acapulco = tag.ID
		}
	}
	if acapulco == 0 {
		tag, err := st.CreateTag(TagInput{Nombre: "Acapulco", Tipo: "ciudad", Color: "#2E7D32"})
		if err != nil {
			t.Fatal(err)
		}
		acapulco = tag.ID
	}
	past := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	id1, err := st.Insert(pdfutil.SavedFile{
		NombreGuardado: "11111111111111111111111111111111.pdf", NombreOriginal: "a.pdf",
		Paginas: 1, PaginasTexto: []string{"Convenio con el Ayuntamiento de Acapulco, Guerrero."},
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := st.Insert(pdfutil.SavedFile{
		NombreGuardado: "22222222222222222222222222222222.pdf", NombreOriginal: "b.pdf",
		Paginas: 1, PaginasTexto: []string{"Convenio con el Ayuntamiento de Acapulco."},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{id1, id2} {
		if err := st.Update(ConventionInput{ID: id, Dependencia: "Ayuntamiento de Acapulco", FechaFirma: "2020-01-01", FechaVigencia: past, Responsable: "Dirección de Vinculación"}); err != nil {
			t.Fatal(err)
		}
		if err := st.ReplaceTags(id, []int64{nacional, guerrero, acapulco}, true); err != nil {
			t.Fatal(err)
		}
	}
	d, err := st.Get(id1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Riesgo.Nivel != "alto" || len(d.Riesgo.Motivos) == 0 {
		t.Fatalf("riesgo = %+v", d.Riesgo)
	}
	r, err := st.Radar()
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 2 || r.Vencidos != 2 || r.Alto < 1 {
		t.Fatalf("radar = %+v", r)
	}
	if len(r.Contrapartes) != 1 || r.Contrapartes[0].Total != 2 {
		t.Fatalf("contrapartes = %+v", r.Contrapartes)
	}
	rel, err := st.Related(id1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rel) != 1 || rel[0].ID != id2 {
		t.Fatalf("relacionados = %+v", rel)
	}
	if !strings.Contains(strings.Join(rel[0].Motivos, " "), "Misma contraparte") {
		t.Fatalf("motivos = %+v", rel[0].Motivos)
	}
}

func TestPrepareAsk(t *testing.T) {
	st, _ := testStore(t)
	id, err := st.Insert(pdfutil.SavedFile{
		NombreGuardado: "33333333333333333333333333333333.pdf", NombreOriginal: "ask.pdf",
		Paginas: 2, PaginasTexto: []string{
			"Convenio con el Ayuntamiento de Acapulco sobre salud comunitaria.",
			"La vigencia termina en 2027 y el responsable es la Dirección de Salud.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, sources, err := st.PrepareAsk("¿Qué convenio con Acapulco trata salud?", 8)
	if err != nil {
		t.Fatal(err)
	}
	if ctx == "" || len(sources) == 0 {
		t.Fatalf("sin contexto: %q %+v", ctx, sources)
	}
	if sources[0].ConvenioID != id || sources[0].Pagina < 1 {
		t.Fatalf("fuente = %+v", sources[0])
	}
	if !strings.Contains(ctx, "Convenio #") || !strings.Contains(ctx, "Página") {
		t.Fatalf("contexto sin citas: %s", ctx)
	}
}
