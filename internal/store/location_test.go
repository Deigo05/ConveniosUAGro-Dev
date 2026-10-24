package store

import (
	"errors"
	"testing"
)

func tagNames(t *testing.T, st *Store, id int64) map[string]string {
	t.Helper()
	tags, err := st.TagsForConvention(id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, tag := range tags {
		out[tag.Tipo] = tag.Nombre
	}
	return out
}

// Una ciudad no puede ir con el estado de otra; al elegirla a mano se agregan
// su estado y «Nacional».
func TestCityStateRules(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "convenio")
	aca, err := st.CreateTag(TagInput{Nombre: "Acapulco de Juárez", Tipo: "ciudad", Estado: "guerrero"})
	if err != nil || aca.Estado != "Guerrero" {
		t.Fatalf("ciudad = %+v, %v", aca, err)
	}
	err = st.ReplaceTags(id, []int64{aca.ID, tagID(t, st, "estado", "Veracruz")}, true)
	var conflict TagConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Acapulco con Veracruz debía rechazarse: %v", err)
	}
	if err := st.ReplaceTags(id, []int64{aca.ID}, true); err != nil {
		t.Fatal(err)
	}
	got := tagNames(t, st, id)
	if got["estado"] != "Guerrero" || got["ambito"] != TagNacional || got["ciudad"] != "Acapulco de Juárez" {
		t.Fatalf("etiquetas = %v", got)
	}
	err = st.ReplaceTags(id, []int64{tagID(t, st, "estado", "Puebla"), tagID(t, st, "ambito", "Internacional")}, true)
	if !errors.As(err, &conflict) {
		t.Fatalf("un estado de México con «Internacional» debía rechazarse: %v", err)
	}
	// Los cambios automáticos corrigen en vez de rechazar: gana el estado de la ciudad.
	if err := st.ReplaceTagsAuto(id, []int64{aca.ID, tagID(t, st, "estado", "Veracruz")}); err != nil {
		t.Fatal(err)
	}
	if got := tagNames(t, st, id); got["estado"] != "Guerrero" {
		t.Fatalf("auto = %v", got)
	}
	if _, err := st.CreateTag(TagInput{Nombre: "Springfield", Tipo: "ciudad", Estado: "Ohio"}); err == nil {
		t.Fatal("el estado de una ciudad debe ser de México")
	}
}

func TestAcceptSuggestionsCreatesCityWithState(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "convenio")
	if err := st.SetSuggestions(id, []Suggestion{
		{Tipo: "estado", Nombre: "Puebla"}, {Tipo: "ciudad", Nombre: "Huauchinango", Nueva: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptSuggestions(id); err != nil {
		t.Fatal(err)
	}
	got := tagNames(t, st, id)
	if got["ambito"] != TagNacional || got["estado"] != "Puebla" || got["ciudad"] != "Huauchinango" {
		t.Fatalf("etiquetas = %v", got)
	}
	city, ok, err := st.findTag("ciudad", "Huauchinango")
	if err != nil || !ok || city.Estado != "Puebla" {
		t.Fatalf("ciudad = %+v %v %v", city, ok, err)
	}
	// Otro documento con la misma ciudad pero otro estado sugerido: gana el
	// estado ya registrado de la ciudad.
	id2 := insertSample(t, st, "otro convenio")
	_ = st.SetSuggestions(id2, []Suggestion{{Tipo: "estado", Nombre: "Hidalgo"}, {Tipo: "ciudad", Nombre: "Huauchinango"}})
	if err := st.AcceptSuggestions(id2); err != nil {
		t.Fatal(err)
	}
	if got := tagNames(t, st, id2); got["estado"] != "Puebla" {
		t.Fatalf("etiquetas = %v", got)
	}
}

func TestBackfillCityStatesAndEnsureAmbito(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "convenio")
	// Una ciudad de una versión anterior, sin estado.
	res, err := st.db.Exec("INSERT INTO etiquetas (nombre, tipo, color) VALUES ('Ometepec', 'ciudad', '#2E7D32')")
	if err != nil {
		t.Fatal(err)
	}
	city, _ := res.LastInsertId()
	for _, tag := range []int64{city, tagID(t, st, "estado", "Guerrero")} {
		if _, err := st.db.Exec("INSERT INTO convenio_etiquetas VALUES (?, ?)", id, tag); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.BackfillCityStates(); err != nil {
		t.Fatal(err)
	}
	tag, err := st.TagByID(city)
	if err != nil || tag.Estado != "Guerrero" {
		t.Fatalf("ciudad = %+v %v", tag, err)
	}
	if err := st.EnsureAmbito(id); err != nil {
		t.Fatal(err)
	}
	if got := tagNames(t, st, id); got["ambito"] != TagNacional {
		t.Fatalf("etiquetas = %v", got)
	}
}

func TestNoConvenioFlag(t *testing.T) {
	st, _ := testStore(t)
	acta := insertSample(t, st, "acta constitutiva")
	normal := insertSample(t, st, "convenio")
	if _, err := st.db.Exec("UPDATE convenios SET notas='El texto es un acta constitutiva de una asociación civil, no un convenio de la UAGro.' WHERE id=?", acta); err != nil {
		t.Fatal(err)
	}
	if n, err := st.FlagNoConveniosFromNotes(); err != nil || n != 1 {
		t.Fatalf("marcados = %d, %v", n, err)
	}
	res, err := st.List(ListFilter{SoloNoConvenios: true})
	if err != nil || res.Total != 1 || res.Items[0].ID != acta || !res.Items[0].NoEsConvenio {
		t.Fatalf("filtro = %+v, %v", res, err)
	}
	if s, _ := st.Stats(); s.NoConvenios != 1 {
		t.Fatalf("estadísticas = %+v", s)
	}
	// El usuario confirma que sí es un convenio: la IA ya no lo vuelve a marcar.
	if err := st.SetNoConvenio(acta, false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetAIConvenioFlag(acta, false); err != nil {
		t.Fatal(err)
	}
	if flagged, _ := st.IsFlaggedNoConvenio(acta); flagged {
		t.Fatal("la IA no debe sobrescribir lo que confirmó el usuario")
	}
	if err := st.SetAIConvenioFlag(normal, false); err != nil {
		t.Fatal(err)
	}
	if flagged, _ := st.IsFlaggedNoConvenio(normal); !flagged {
		t.Fatal("la IA debe poder marcar un archivo")
	}
	if err := st.SetAIConvenioFlag(normal, true); err != nil {
		t.Fatal(err)
	}
	if flagged, _ := st.IsFlaggedNoConvenio(normal); flagged {
		t.Fatal("un nuevo análisis que dice que sí es convenio quita la marca")
	}
}

func TestMetaFlags(t *testing.T) {
	st, _ := testStore(t)
	if done, err := st.Flag("x"); err != nil || done {
		t.Fatalf("flag = %v %v", done, err)
	}
	if err := st.SetFlag("x"); err != nil {
		t.Fatal(err)
	}
	if done, _ := st.Flag("x"); !done {
		t.Fatal("la marca no quedó guardada")
	}
}
