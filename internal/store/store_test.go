package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"convenios-uagro-desktop/internal/pdfutil"
)

func testStore(t *testing.T) (*Store, Paths) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CONVENIOS_DATA_DIR", dir)
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, paths
}

func TestSeedAndEmptyState(t *testing.T) {
	st, _ := testStore(t)
	tags, err := st.ListTags()
	if err != nil {
		t.Fatal(err)
	}
	// 2 ámbitos, 32 estados y la facultad "Convenio Abierto".
	if len(tags) != 35 {
		t.Fatalf("tags = %d, want 35", len(tags))
	}
	stats, err := st.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 0 {
		t.Fatalf("total = %d, want 0", stats.Total)
	}
}

func TestCRUDAndFilters(t *testing.T) {
	st, paths := testStore(t)
	id, err := st.Insert(pdfutil.SavedFile{
		NombreGuardado: "0123456789abcdef0123456789abcdef.pdf",
		NombreOriginal: "convenio.pdf",
		Bytes:          123,
		SHA256:         "abc",
		Paginas:        3,
		Texto:          "Convenio de colaboración con el ayuntamiento de Acapulco, Guerrero.",
	})
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	if err := st.Update(ConventionInput{
		ID: id, Dependencia: "Ayuntamiento de Acapulco", FechaFirma: "2024-01-10",
		FechaVigencia: future, VigenciaTexto: "Tres años",
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Dependencia != "Ayuntamiento de Acapulco" || detail.Estado != "vigente" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	res, err := st.List(ListFilter{Query: "acapulco"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("fts result = %+v", res)
	}
	res, err = st.List(ListFilter{Query: "guerrero", Vigencia: "vigentes"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("fts+vigencia total = %d", res.Total)
	}
	res, err = st.List(ListFilter{Query: "noexiste"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 {
		t.Fatalf("unexpected search result: %d", res.Total)
	}

	tag, err := st.CreateTag(TagInput{Nombre: "Acapulco", Tipo: "ciudad", Color: "#2E7D32"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(id, []int64{tag.ID}, true); err != nil {
		t.Fatal(err)
	}
	res, err = st.List(ListFilter{TagIDs: []int64{tag.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("tag filter total = %d", res.Total)
	}
	if _, err := os.Stat(filepath.Join(paths.PDFs, detail.ArchivoGuardado)); err == nil {
		t.Fatal("el Store no debe crear archivos PDF")
	}
}

func TestSuggestionsAndProtection(t *testing.T) {
	st, _ := testStore(t)
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pdf", NombreOriginal: "x.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSuggestions(id, []Suggestion{
		{Tipo: "ambito", Nombre: "Nacional"},
		{Tipo: "estado", Nombre: "Guerrero"},
		{Tipo: "ciudad", Nombre: "Acapulco", Nueva: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptSuggestions(id); err != nil {
		t.Fatal(err)
	}
	tags, err := st.TagsForConvention(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("tags = %d, want 3: %+v", len(tags), tags)
	}
	detail, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.EtiquetasRevisadas || len(detail.EtiquetasSugeridas) != 0 {
		t.Fatalf("clasificación no marcada: %+v", detail)
	}
	for _, tag := range tags {
		if tag.Protegida {
			if err := st.DeleteTag(tag.ID); err == nil {
				t.Fatalf("se pudo borrar etiqueta protegida %s", tag.Nombre)
			}
		}
	}
}

// sampleSeq evita nombres repetidos cuando el reloj tiene poca resolución.
var sampleSeq int64

func insertSample(t *testing.T, st *Store, texto string) int64 {
	t.Helper()
	sampleSeq++
	name := fmt.Sprintf("%016x%016x.pdf", time.Now().UnixNano(), sampleSeq)
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: name, NombreOriginal: "muestra.pdf", Paginas: 1, Texto: texto, SHA256: name})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
