package store

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Un convenio archivado sale de la lista principal, de su búsqueda y de sus
// contadores, y aparece solo en la sección de archivados, con su propia
// búsqueda. Restaurarlo lo devuelve a la lista principal.
func TestArchiveSeparatesLists(t *testing.T) {
	st, _ := testStore(t)
	a := insertSample(t, st, "convenio con el municipio de Taxco")
	b := insertSample(t, st, "convenio con el municipio de Iguala")
	if err := st.Update(ConventionInput{ID: a, Dependencia: "Ayuntamiento de Taxco", FechaVigencia: "2001-01-01"}); err != nil {
		t.Fatal(err)
	}

	n, err := st.SetArchived([]int64{a, a}, true)
	if err != nil || n != 1 {
		t.Fatalf("SetArchived = %d, %v", n, err)
	}
	main, err := st.List(ListFilter{})
	if err != nil || main.Total != 1 || main.Items[0].ID != b || main.TotalRegistrados != 1 {
		t.Fatalf("lista principal = %+v, %v", main, err)
	}
	if res, _ := st.List(ListFilter{Query: "Taxco"}); res.Total != 0 {
		t.Fatalf("el buscador principal encontró un archivado: %+v", res.Items)
	}
	arch, err := st.List(ListFilter{Archivados: true, Query: "Taxco"})
	if err != nil || arch.Total != 1 || arch.Items[0].ID != a || arch.Items[0].ArchivadoEn == "" {
		t.Fatalf("buscador de archivados = %+v, %v", arch, err)
	}
	if res, _ := st.List(ListFilter{Archivados: true, Query: "Iguala"}); res.Total != 0 {
		t.Fatalf("el buscador de archivados encontró uno sin archivar: %+v", res.Items)
	}
	stats, err := st.Stats()
	if err != nil || stats.Total != 1 || stats.Vencidos != 0 || stats.Archivados != 1 {
		t.Fatalf("contadores = %+v, %v", stats, err)
	}
	d, err := st.Get(a)
	if err != nil || d.ArchivadoEn == "" {
		t.Fatalf("ficha sin fecha de archivado: %q, %v", d.ArchivadoEn, err)
	}

	if n, err := st.SetArchived([]int64{a}, false); err != nil || n != 1 {
		t.Fatalf("restaurar = %d, %v", n, err)
	}
	if res, _ := st.List(ListFilter{Query: "Taxco"}); res.Total != 1 {
		t.Fatalf("restaurado no vuelve a la lista principal: %+v", res)
	}
	if stats, _ := st.Stats(); stats.Archivados != 0 || stats.Total != 2 {
		t.Fatalf("contadores tras restaurar = %+v", stats)
	}
	if _, err := st.SetArchived(nil, true); err == nil {
		t.Fatal("archivar sin selección debía dar error")
	}
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// El CSV de los resultados filtrados trae solo los convenios del filtro:
// por vigencia, por etiqueta y sin los archivados.
func TestExportCSVFollowsFilters(t *testing.T) {
	st, paths := testStore(t)
	future := time.Now().AddDate(2, 0, 0).Format("2006-01-02")
	vencido := insertSample(t, st, "a")
	vigente := insertSample(t, st, "b")
	archivado := insertSample(t, st, "c")
	for id, dep := range map[int64]string{vencido: "Vencido SA", vigente: "Vigente SA", archivado: "Archivado SA"} {
		fecha := "2010-05-01"
		if id == vigente {
			fecha = future
		}
		if err := st.Update(ConventionInput{ID: id, Dependencia: dep, FechaVigencia: fecha}); err != nil {
			t.Fatal(err)
		}
	}
	tema, err := st.CreateTag(TagInput{Nombre: "Salud", Tipo: "tema"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(vigente, []int64{tema.ID}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetArchived([]int64{archivado}, true); err != nil {
		t.Fatal(err)
	}
	export := func(f ListFilter) []string {
		ids, err := st.FilterIDs(f)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(paths.Base, "lista.csv")
		if _, err := st.ExportCSV(dst, ids); err != nil {
			t.Fatal(err)
		}
		var deps []string
		for _, row := range readCSV(t, dst)[1:] {
			deps = append(deps, row[2])
		}
		return deps
	}
	if got := export(ListFilter{Vigencia: "no_vigentes"}); len(got) != 1 || got[0] != "Vencido SA" {
		t.Fatalf("CSV de vencidos = %v", got)
	}
	if got := export(ListFilter{TagIDs: []int64{tema.ID}}); len(got) != 1 || got[0] != "Vigente SA" {
		t.Fatalf("CSV por etiqueta = %v", got)
	}
	if got := export(ListFilter{}); len(got) != 2 {
		t.Fatalf("CSV de la lista principal = %v (no debe traer archivados)", got)
	}
	got := export(ListFilter{Archivados: true})
	if len(got) != 1 || got[0] != "Archivado SA" {
		t.Fatalf("CSV de archivados = %v", got)
	}
	rows := readCSV(t, filepath.Join(paths.Base, "lista.csv"))
	if col := rows[0][len(rows[0])-2]; col != "Archivado" || !strings.HasPrefix(rows[1][len(rows[1])-2], "Sí (") {
		t.Fatalf("columna de archivado = %q / %q", col, rows[1][len(rows[1])-2])
	}
}

// El archivado viaja en el paquete exportado.
func TestPackageKeepsArchived(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pdf", "sha-archivado", "convenio archivado")
	if _, err := st.SetArchived([]int64{id}, true); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), "paquete.zip")
	if _, err := st.ExportPackage(pkg, nil, "test"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(newPaths(t))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	sum, err := other.ImportPackage(pkg)
	if err != nil || sum.Importados != 1 {
		t.Fatalf("importar = %+v, %v", sum, err)
	}
	if res, _ := other.List(ListFilter{Archivados: true}); res.Total != 1 {
		t.Fatalf("el convenio importado no quedó archivado: %+v", res)
	}
}

// Un nuevo análisis con IA no borra las etiquetas puestas a mano: los temas se
// suman y la ubicación solo cambia si la IA sugiere una.
func TestAcceptSuggestionsKeepsManualTags(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "convenio")
	salud, _ := st.CreateTag(TagInput{Nombre: "Salud", Tipo: "tema"})
	if _, err := st.CreateTag(TagInput{Nombre: "Cultura", Tipo: "tema"}); err != nil {
		t.Fatal(err)
	}
	taxco, _ := st.CreateTag(TagInput{Nombre: "Taxco de Alarcón", Tipo: "ciudad", Estado: "Guerrero"})
	if err := st.ReplaceTags(id, []int64{salud.ID, taxco.ID}, true); err != nil {
		t.Fatal(err)
	}
	// La IA solo sugiere un tema: se conservan el tema y la ciudad manuales.
	if err := st.SetSuggestions(id, []Suggestion{{Tipo: "tema", Nombre: "Cultura"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptSuggestions(id); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	tags, _ := st.TagsForConvention(id)
	for _, tag := range tags {
		names[tag.Nombre] = true
	}
	for _, want := range []string{"Salud", "Cultura", "Taxco de Alarcón", "Guerrero", "Nacional"} {
		if !names[want] {
			t.Fatalf("falta %q tras el nuevo análisis: %v", want, names)
		}
	}
	// Si la IA sugiere otra ubicación, reemplaza a la anterior.
	if err := st.SetSuggestions(id, []Suggestion{{Tipo: "estado", Nombre: "Puebla"}, {Tipo: "ciudad", Nombre: "Huauchinango", Nueva: true}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptSuggestions(id); err != nil {
		t.Fatal(err)
	}
	got := tagNames(t, st, id)
	if got["ciudad"] != "Huauchinango" || got["estado"] != "Puebla" {
		t.Fatalf("ubicación nueva = %v", got)
	}
}
