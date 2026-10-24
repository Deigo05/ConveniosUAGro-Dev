package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"convenios-uagro-desktop/internal/pdfutil"
)

// insertWithPDF registra un convenio con un PDF mínimo en la carpeta de datos.
func insertWithPDF(t *testing.T, st *Store, paths Paths, saved, sha, texto string) int64 {
	t.Helper()
	if err := os.WriteFile(filepath.Join(paths.PDFs, saved), []byte("%PDF-1.4\n"+sha+"\n%%EOF"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: saved, NombreOriginal: "c-" + sha + ".pdf", SHA256: sha, Paginas: 2, Texto: texto})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func tagID(t *testing.T, st *Store, tipo, nombre string) int64 {
	t.Helper()
	tag, ok, err := st.findTag(tipo, nombre)
	if err != nil || !ok {
		t.Fatalf("no existe la etiqueta %s/%s: %v", tipo, nombre, err)
	}
	return tag.ID
}

func TestOpenAgreementTag(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, strings.Repeat("a", 32)+".pdf", "sha-a", "Convenio general")
	if err := st.MarkOpenAgreement(id); err != nil {
		t.Fatal(err)
	}
	tags, _ := st.TagsForConvention(id)
	if len(tags) != 1 || tags[0].Nombre != TagConvenioAbierto || !tags[0].Protegida {
		t.Fatalf("etiquetas = %+v", tags)
	}
	if err := st.DeleteTag(tags[0].ID); err == nil {
		t.Fatal("la etiqueta base se pudo borrar")
	}
	// Con una facultad elegida no se agrega "Convenio Abierto".
	fac, err := st.CreateTag(TagInput{Nombre: "Facultad de Turismo Acapulco", Tipo: "facultad"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(id, []int64{fac.ID}, true); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkOpenAgreement(id); err != nil {
		t.Fatal(err)
	}
	tags, _ = st.TagsForConvention(id)
	if len(tags) != 1 || tags[0].ID != fac.ID {
		t.Fatalf("etiquetas tras elegir facultad = %+v", tags)
	}
}

func TestSearchFindsSignatoriesTagsAndFallback(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, strings.Repeat("b", 32)+".pdf", "sha-b", "Convenio de colaboración académica en materia de turismo.")
	if err := st.Update(ConventionInput{ID: id, Dependencia: "Hotel Princess", FirmanteDependencia: "María López Ruiz", Facultad: "Facultad de Turismo"}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(id, []int64{tagID(t, st, "estado", "Quintana Roo")}, true); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"maria lopez", "quintana", `"colaboración académica"`, "turismo de la"} {
		res, err := st.List(ListFilter{Query: q})
		if err != nil {
			t.Fatal(err)
		}
		if res.Total != 1 || res.Aproximado {
			t.Fatalf("búsqueda %q: total=%d aproximado=%v", q, res.Total, res.Aproximado)
		}
	}
	res, err := st.List(ListFilter{Query: "turismo inexistente"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || !res.Aproximado {
		t.Fatalf("respaldo OR: total=%d aproximado=%v", res.Total, res.Aproximado)
	}
	if !strings.Contains(res.Items[0].Fragmento, "\x02") {
		t.Fatalf("el fragmento no marca la coincidencia: %q", res.Items[0].Fragmento)
	}
	// Renombrar la etiqueta actualiza el índice.
	tag, _ := st.TagByID(tagID(t, st, "estado", "Quintana Roo"))
	if _, err := st.DB().Exec("UPDATE etiquetas SET nombre='Caribe Mexicano' WHERE id=?", tag.ID); err != nil {
		t.Fatal(err)
	}
	if res, _ := st.List(ListFilter{Query: "caribe"}); res.Total != 1 {
		t.Fatalf("la etiqueta renombrada no se encuentra: %d", res.Total)
	}
}

// Buscar el nombre de un convenio lo pone primero aunque sus palabras también
// sean el inicio de palabras comunes ("par" de "para", "3" de "39070").
func TestSearchRanksNameFirst(t *testing.T) {
	st, paths := testStore(t)
	comun := strings.Repeat("Las partes acuerdan para los grupos de estudiantes, C.P. 39070, cláusula 3. ", 20)
	add := func(n int, dependencia, texto string) int64 {
		id := insertWithPDF(t, st, paths, strings.Repeat(string(rune('d'+n)), 32)+".pdf", "sha-rank-"+dependencia, texto)
		if err := st.Update(ConventionInput{ID: id, Dependencia: dependencia}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	target := add(0, "GRUPO PAR DE 3 S DE RL DE CV", "Convenio con GRUPO PAR DE 3 para prácticas.")
	mention := add(1, "Grupo Vidanta", comun+"Antes colaboró con Grupo Par de 3.")
	add(2, "ALFARO GRUPO APÍCOLA", comun)
	add(3, "Instituto Municipal de la Juventud", comun)
	// En el texto, la frase exacta gana a la que solo coincide como prefijo ("podrán").
	frase := add(4, "Hotel Uno", comun+"El presente convenio podrá modificarse por escrito.")
	add(5, "Hotel Dos", strings.Repeat("El presente convenio podrán modificarlo las partes; ninguna podrá cederlo. ", 3))
	for q, want := range map[string][]int64{
		"GRUPO PAR DE 3":          {target, mention},
		"grupo par":               {target},
		"grupo par de":            {target},
		"par 3":                   {target},
		"presente convenio podrá": {frase},
	} {
		res, err := st.List(ListFilter{Query: q, Order: "relevancia"})
		if err != nil {
			t.Fatal(err)
		}
		for i, id := range want {
			if len(res.Items) <= i || res.Items[i].ID != id {
				t.Fatalf("búsqueda %q: resultado %d = %+v, se esperaba el id %d", q, i+1, res.Items, id)
			}
		}
	}
}

func TestApplyNameCorrections(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, strings.Repeat("c", 32)+".pdf", "sha-c", "")
	if err := st.BeginTranscription(id, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(id, 1, "Comparece el Dr. Juan Ramón García, Rector de la UAGro."); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(id, 2, "Firma: JUAN RAMÓN GARAY[?]\nTestigo: Pedro Garay Soto"); err != nil {
		t.Fatal(err)
	}
	applied, err := st.ApplyNameCorrections(id, []NameCorrection{
		{Original: "Juan Ramón Garay", Correcto: "Juan Ramón García"},
		{Original: "Pedro Garay Soto", Correcto: "Ana Torres"},        // no se parecen: se ignora
		{Original: "Juan Ramón García", Correcto: "Juan Ramón Gómez"}, // la forma "correcta" no está en el texto
		{Original: "Garay", Correcto: "García"},                       // una sola palabra: se ignora
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 || applied[0].Cambios != 1 {
		t.Fatalf("correcciones aplicadas = %+v", applied)
	}
	tr, _ := st.GetTranscription(id)
	if !strings.Contains(tr.Texto, "JUAN RAMÓN GARCÍA") || strings.Contains(tr.Texto, "RAMÓN GARAY") {
		t.Fatalf("texto corregido = %q", tr.Texto)
	}
	if !strings.Contains(tr.Texto, "Pedro Garay Soto") {
		t.Fatalf("se alteró a otra persona: %q", tr.Texto)
	}
}

func TestCloneAnalysisForDuplicates(t *testing.T) {
	st, paths := testStore(t)
	src := insertWithPDF(t, st, paths, strings.Repeat("d", 32)+".pdf", "sha-d", "texto")
	if err := st.SetAIResult(src, ConventionInput{Dependencia: "IMSS", FirmanteUAGro: "Rector"}, "ok", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(src, []int64{tagID(t, st, "ambito", "Nacional")}, true); err != nil {
		t.Fatal(err)
	}
	dst := insertWithPDF(t, st, paths, strings.Repeat("e", 32)+".pdf", "sha-d", "texto")
	ok, err := st.CloneAnalysis(src, dst)
	if err != nil || !ok {
		t.Fatalf("clonar: ok=%v err=%v", ok, err)
	}
	d, _ := st.Get(dst)
	if d.Dependencia != "IMSS" || d.IAEstado != "ok" || len(d.Etiquetas) != 1 {
		t.Fatalf("duplicado = %+v", d)
	}
}

func TestExportImportPackageAndBackup(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, strings.Repeat("f", 32)+".pdf", "sha-f", "Convenio con Ayuntamiento")
	if err := st.Update(ConventionInput{ID: id, Dependencia: "Ayuntamiento de Taxco", FechaFirma: "2024-01-10", FechaVigencia: "2027-01-10", Facultad: "Facultad de Derecho"}); err != nil {
		t.Fatal(err)
	}
	ciudad, err := st.CreateTag(TagInput{Nombre: "Taxco", Tipo: "ciudad"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(id, []int64{ciudad.ID, tagID(t, st, "estado", "Guerrero")}, true); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), "paquete.zip")
	if n, err := st.ExportPackage(pkg, nil, "test"); err != nil || n != 1 {
		t.Fatalf("exportar: n=%d err=%v", n, err)
	}
	backup := filepath.Join(t.TempDir(), "respaldo.zip")
	if err := st.BackupZip(backup); err != nil {
		t.Fatal(err)
	}

	// Otra instalación vacía.
	other, err := Open(newPaths(t))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	sum, err := other.ImportPackage(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Importados != 1 || len(sum.Errores) != 0 {
		t.Fatalf("importar paquete: %+v", sum)
	}
	d, err := other.Get(sum.IDs[0])
	if err != nil {
		t.Fatal(err)
	}
	// Guerrero implica «Nacional» y la ciudad conserva su estado.
	if d.Dependencia != "Ayuntamiento de Taxco" || d.FechaVigencia != "2027-01-10" || len(d.Etiquetas) != 3 {
		t.Fatalf("convenio importado = %+v", d)
	}
	for _, tag := range d.Etiquetas {
		if tag.Tipo == "ciudad" && tag.Estado != "Guerrero" {
			t.Fatalf("la ciudad importada perdió su estado: %+v", tag)
		}
	}
	if res, _ := other.List(ListFilter{Query: "taxco derecho"}); res.Total != 1 {
		t.Fatalf("el convenio importado no se encuentra: %d", res.Total)
	}
	if _, _, err := other.PDF(d.ID); err != nil {
		t.Fatal(err)
	}
	// Importar otra vez el mismo contenido, ahora desde el respaldo, no duplica.
	sum, err = other.ImportPackage(backup)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Importados != 0 || sum.Duplicados != 1 {
		t.Fatalf("importar respaldo duplicado: %+v", sum)
	}

	// Desde el respaldo hacia una tercera base vacía.
	third, err := Open(newPaths(t))
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	sum, err = third.ImportPackage(backup)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Importados != 1 {
		t.Fatalf("importar respaldo: %+v", sum)
	}

	csvPath := filepath.Join(t.TempDir(), "lista.csv")
	if n, err := st.ExportCSV(csvPath, []int64{id}); err != nil || n != 1 {
		t.Fatalf("csv: n=%d err=%v", n, err)
	}
	data, _ := os.ReadFile(csvPath)
	if !strings.HasPrefix(string(data), string(utf8BOM)+"ID,") || !strings.Contains(string(data), "Ayuntamiento de Taxco") {
		t.Fatalf("csv inesperado: %q", data)
	}
}

func newPaths(t *testing.T) Paths {
	t.Helper()
	base := t.TempDir()
	p := Paths{Base: base, DB: filepath.Join(base, "convenios.db"), PDFs: filepath.Join(base, "pdfs"), Backups: filepath.Join(base, "respaldos"), Config: filepath.Join(base, "settings.json")}
	for _, d := range []string{p.PDFs, p.Backups} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestPorVencerFilter(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, strings.Repeat("9", 32)+".pdf", "sha-9", "x")
	soon := Today()
	if err := st.Update(ConventionInput{ID: id, FechaVigencia: soon}); err != nil {
		t.Fatal(err)
	}
	res, err := st.List(ListFilter{Vigencia: "por_vencer"})
	if err != nil || res.Total != 1 {
		t.Fatalf("por vencer: total=%d err=%v", res.Total, err)
	}
}

// Las correcciones confirmadas con la imagen se aplican aunque las variantes
// no se parezcan, pero solo en su página y nunca a un párrafo entero.
func TestApplyPageCorrections(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, strings.Repeat("d", 32)+".pdf", "sha-d", "")
	if err := st.BeginTranscription(id, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(id, 1, "Representada por la Dra. Joaquín Zorro Falcón en el proemio."); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(id, 2, "a los 4 días del mes de noviembre\n[Firma]\nDR. JOAQUÍN ZORRO[?] FALCÓN"); err != nil {
		t.Fatal(err)
	}
	applied, err := st.ApplyPageCorrections(id, 2, []NameCorrection{
		{Original: "Joaquín Zorro Falcón", Correcto: "Jacqueline Cerón Falcón"},
		{Original: "4 días del mes de noviembre", Correcto: "4 días del mes de septiembre"},
		{Original: "texto que no está en la página", Correcto: "otro"},
		{Original: "a los 4 días del mes de noviembre de un año que no cabe en doce palabras", Correcto: "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 {
		t.Fatalf("correcciones aplicadas = %+v", applied)
	}
	p2, _ := st.TranscriptionPage(id, 2)
	if !strings.Contains(p2, "JACQUELINE CERÓN FALCÓN") || !strings.Contains(p2, "mes de septiembre") {
		t.Fatalf("página 2 = %q", p2)
	}
	if p1, _ := st.TranscriptionPage(id, 1); !strings.Contains(p1, "Joaquín Zorro Falcón") {
		t.Fatalf("no debía tocar otra página: %q", p1)
	}
	if tr, _ := st.GetTranscription(id); !strings.Contains(tr.Texto, "septiembre") {
		t.Fatalf("el texto completo no se reconstruyó: %q", tr.Texto)
	}
}
