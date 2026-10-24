package store

import (
	"fmt"
	"math/bits"
	"path/filepath"
	"strings"
	"testing"
)

// sampleText arma un texto largo y reproducible a partir de una semilla.
func sampleText(seed, words int) string {
	vocab := strings.Fields("convenio universidad autonoma guerrero partes clausula objeto vigencia firma " +
		"colaboracion academica investigacion servicio social practicas profesionales estudiantes docentes " +
		"compromisos obligaciones confidencialidad terminacion anticipada controversias jurisdiccion domicilio " +
		"representante legal rector director facultad medicina derecho turismo programa proyecto recursos")
	var b strings.Builder
	x := uint32(seed*7919 + 17)
	for i := 0; i < words; i++ {
		x = x*1664525 + 1013904223
		b.WriteString(vocab[int(x>>8)%len(vocab)])
		b.WriteByte(' ')
	}
	return b.String()
}

func TestFingerprintToleratesReadingErrors(t *testing.T) {
	text := sampleText(1, 600)
	words := strings.Fields(text)
	for i := 0; i < len(words); i += 40 { // un error de lectura cada 40 palabras
		words[i] = words[i][:len(words[i])-1] + "x"
	}
	a, b := fingerprint(text), fingerprint(strings.Join(words, " "))
	other := fingerprint(sampleText(2, 600))
	if d := bits.OnesCount64(a ^ b); d > fingerprintMaxDist {
		t.Fatalf("misma lectura con errores: %d bits distintos", d)
	}
	if d := bits.OnesCount64(a ^ other); d <= fingerprintMaxDist*2 {
		t.Fatalf("textos distintos demasiado parecidos: %d bits", d)
	}
	if fingerprint("muy poco texto") != 0 {
		t.Fatal("un texto corto no debe tener huella")
	}
}

func listByID(t *testing.T, st *Store, f ListFilter) map[int64]ConventionSummary {
	t.Helper()
	res, err := st.List(f)
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]ConventionSummary{}
	for _, c := range res.Items {
		out[c.ID] = c
	}
	return out
}

func refresh(t *testing.T, st *Store) {
	t.Helper()
	if err := st.RefreshDuplicates(); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateMarkFollowsTheDuplicate(t *testing.T) {
	st, paths := testStore(t)
	a := insertWithPDF(t, st, paths, strings.Repeat("a", 32)+".pdf", "mismo", "uno")
	b := insertWithPDF(t, st, paths, strings.Repeat("b", 32)+".pdf", "mismo", "uno")
	other := insertWithPDF(t, st, paths, strings.Repeat("c", 32)+".pdf", "otro", "dos")
	refresh(t, st)

	items := listByID(t, st, ListFilter{})
	if items[a].Duplicados != 1 || items[b].Duplicados != 1 || items[other].Duplicados != 0 {
		t.Fatalf("marcas: a=%d b=%d otro=%d", items[a].Duplicados, items[b].Duplicados, items[other].Duplicados)
	}
	pairs, err := st.ListDuplicates()
	if err != nil || len(pairs) != 1 || pairs[0].Motivo != MotivoArchivo {
		t.Fatalf("pares: %+v %v", pairs, err)
	}
	d, err := st.Get(a)
	if err != nil || len(d.Relacionados) != 1 || d.Relacionados[0].ID != b {
		t.Fatalf("relacionados: %+v %v", d.Relacionados, err)
	}
	if s, _ := st.Stats(); s.Duplicados != 1 {
		t.Fatalf("stats duplicados = %d", s.Duplicados)
	}

	// Al borrar el duplicado, la marca del original desaparece sola.
	if _, err := st.DeleteMany([]int64{b}); err != nil {
		t.Fatal(err)
	}
	if items := listByID(t, st, ListFilter{}); items[a].Duplicados != 0 {
		t.Fatalf("la marca sigue tras borrar el duplicado: %d", items[a].Duplicados)
	}
}

func TestDismissedDuplicateIsNotSuggestedAgain(t *testing.T) {
	st, paths := testStore(t)
	a := insertWithPDF(t, st, paths, strings.Repeat("a", 32)+".pdf", "mismo", "uno")
	b := insertWithPDF(t, st, paths, strings.Repeat("b", 32)+".pdf", "mismo", "uno")
	refresh(t, st)
	if err := st.DismissDuplicate(b, a); err != nil {
		t.Fatal(err)
	}
	refresh(t, st)
	if pairs, _ := st.ListDuplicates(); len(pairs) != 0 {
		t.Fatalf("se volvió a sugerir: %+v", pairs)
	}
}

func setData(t *testing.T, st *Store, id int64, dep, tipo, fecha string) {
	t.Helper()
	if err := st.Update(ConventionInput{ID: id, Dependencia: dep, TipoDocumento: tipo, FechaFirma: fecha}); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateBySameDataOrSameText(t *testing.T) {
	st, _ := testStore(t)
	a := insertSample(t, st, "uno")
	b := insertSample(t, st, "dos")
	c := insertSample(t, st, "tres")
	setData(t, st, a, "Instituto Mexicano del Seguro Social", "Convenio General", "2024-03-15")
	setData(t, st, b, "INSTITUTO MEXICANO DEL SEGURO SOCIAL.", "", "2024-03-15")
	// Mismo día, misma contraparte, otro tipo de documento: es otro convenio.
	setData(t, st, c, "Instituto Mexicano del Seguro Social", "Convenio Específico", "2024-03-15")
	refresh(t, st)
	pairs, err := st.ListDuplicates()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range pairs {
		got[fmt.Sprintf("%d-%d", p.A.ID, p.B.ID)] = p.Motivo
	}
	if got[fmt.Sprintf("%d-%d", a, b)] != MotivoDatos || got[fmt.Sprintf("%d-%d", b, c)] != MotivoDatos || len(got) != 2 {
		t.Fatalf("pares por datos: %v", got)
	}

	// Dos escaneos del mismo documento, uno aún sin analizar.
	st2, _ := testStore(t)
	text := sampleText(3, 500)
	x := insertSample(t, st2, text)
	y := insertSample(t, st2, strings.Replace(text, "convenio", "conveni0", 3))
	setData(t, st2, x, "Ayuntamiento de Taxco", "", "2020-01-10")
	refresh(t, st2)
	if pairs, _ := st2.ListDuplicates(); len(pairs) != 1 || pairs[0].Motivo != MotivoTexto || pairs[0].A.ID != x || pairs[0].B.ID != y {
		t.Fatalf("pares por texto: %+v", pairs)
	}
}

func TestRenewalsAndHistory(t *testing.T) {
	st, paths := testStore(t)
	a := insertWithPDF(t, st, paths, strings.Repeat("a", 32)+".pdf", "sha-a", "uno")
	b := insertWithPDF(t, st, paths, strings.Repeat("b", 32)+".pdf", "sha-b", "dos")
	c := insertWithPDF(t, st, paths, strings.Repeat("c", 32)+".pdf", "sha-c", "tres")
	setData(t, st, a, "Secretaría de Salud", "Convenio General", "2018-01-10")
	setData(t, st, b, "Secretaría de Salud", "Convenio General", "2021-01-10")
	setData(t, st, c, "Secretaría de Salud", "Convenio General", "2024-01-10")
	refresh(t, st)
	pairs, _ := st.ListDuplicates()
	if len(pairs) != 2 {
		t.Fatalf("se esperaban 2 renovaciones consecutivas: %+v", pairs)
	}
	for _, p := range pairs {
		if p.Motivo != MotivoRenovacion || (p.A.ID == a && p.B.ID == c) {
			t.Fatalf("par inesperado %d-%d %s", p.A.ID, p.B.ID, p.Motivo)
		}
	}
	if !listByID(t, st, ListFilter{})[a].PosibleRenovacion {
		t.Fatal("falta la marca de posible renovación")
	}

	if err := st.LinkVersion(a, b); err != nil {
		t.Fatal(err)
	}
	if err := st.LinkVersion(b, c); err != nil {
		t.Fatal(err)
	}
	if err := st.LinkVersion(c, a); err == nil {
		t.Fatal("se permitió un ciclo en el historial")
	}
	items := listByID(t, st, ListFilter{})
	if len(items) != 1 || items[c].Versiones != 2 {
		t.Fatalf("listado tras unir: %+v", items)
	}
	if all := listByID(t, st, ListFilter{IncluirAnteriores: true}); len(all) != 3 || all[a].ReemplazadoPor != b {
		t.Fatalf("listado con anteriores: %+v", all)
	}
	if s, _ := st.Stats(); s.Total != 1 || s.VersionesAnteriores != 2 || s.Renovaciones != 0 {
		t.Fatalf("stats: %+v", s)
	}
	old, err := st.Get(a)
	if err != nil {
		t.Fatal(err)
	}
	if old.VersionActual != c || len(old.Historial) != 3 || old.Historial[0].ID != c || old.Historial[2].ID != a || !old.Historial[0].Actual {
		t.Fatalf("historial: actual=%d %+v", old.VersionActual, old.Historial)
	}

	// Un paquete conserva el historial en otra instalación.
	pkg := filepath.Join(t.TempDir(), "historial.zip")
	if _, err := st.ExportPackage(pkg, nil, "test"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(newPaths(t))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.ImportPackage(pkg); err != nil {
		t.Fatal(err)
	}
	if s, _ := other.Stats(); s.Total != 1 || s.VersionesAnteriores != 2 {
		t.Fatalf("historial importado: %+v", s)
	}

	// Al borrar la versión vigente, la anterior vuelve a mostrarse.
	if _, err := st.DeleteMany([]int64{c}); err != nil {
		t.Fatal(err)
	}
	if items := listByID(t, st, ListFilter{}); len(items) != 1 || items[b].Versiones != 1 {
		t.Fatalf("tras borrar la vigente: %+v", items)
	}
	if err := st.UnlinkVersion(a); err != nil {
		t.Fatal(err)
	}
	if items := listByID(t, st, ListFilter{}); len(items) != 2 {
		t.Fatalf("tras separar: %+v", items)
	}
}
