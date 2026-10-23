package store

import (
	"testing"

	"convenios-uagro-desktop/internal/pdfutil"
)

func TestSignatoriesFacultyAndMultipleCities(t *testing.T) {
	st, _ := testStore(t)
	id, err := st.Insert(pdfutil.SavedFile{
		NombreGuardado: "abcdefabcdefabcdefabcdefabcdefab.pdf", NombreOriginal: "x.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(ConventionInput{
		ID: id, Dependencia: "Ayuntamiento", Facultad: "Facultad de Turismo",
		FirmanteDependencia: "Ana López", CargoDependencia: "Presidenta",
		FirmanteUAGro: "Luis Pérez", CargoUAGro: "Director",
	}); err != nil {
		t.Fatal(err)
	}
	d, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if d.FirmanteDependencia != "Ana López" || d.CargoUAGro != "Director" || d.Facultad != "Facultad de Turismo" {
		t.Fatalf("campos = %+v", d.ConventionSummary)
	}
	c1, err := st.CreateTag(TagInput{Nombre: "Acapulco", Tipo: "ciudad", Color: "#2E7D32"})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := st.CreateTag(TagInput{Nombre: "Chilpancingo", Tipo: "ciudad", Color: "#2E7D32"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(id, []int64{c1.ID, c2.ID}, true); err != nil {
		t.Fatal(err)
	}
	tags, err := st.TagsForConvention(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("etiquetas = %+v", tags)
	}
}
