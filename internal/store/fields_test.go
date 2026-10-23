package store

import (
	"testing"

	"convenios-uagro-desktop/internal/pdfutil"
)

func TestSignatoriesFacultyAndSingleCity(t *testing.T) {
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
	cities := 0
	for _, tag := range tags {
		if tag.Tipo == "ciudad" {
			cities++
		}
	}
	if cities != 1 {
		t.Fatalf("se permitieron %d ciudades: %+v", cities, tags)
	}
}

func TestOnlyOneFacultyTag(t *testing.T) {
	st, _ := testStore(t)
	id, err := st.Insert(pdfutil.SavedFile{NombreGuardado: "1234567890abcdef1234567890abcdef.pdf", NombreOriginal: "x.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := st.CreateTag(TagInput{Nombre: "Acapulco", Tipo: "ciudad", Color: ColorCiudad})
	if err != nil {
		t.Fatal(err)
	}
	f1, err := st.CreateTag(TagInput{Nombre: "Facultad de Turismo Acapulco", Tipo: "facultad", Color: ColorFacultad})
	if err != nil {
		t.Fatal(err)
	}
	f2, err := st.CreateTag(TagInput{Nombre: "Facultad de Medicina Acapulco", Tipo: "facultad", Color: ColorFacultad})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceTags(id, []int64{c.ID, f1.ID, f2.ID}, true); err != nil {
		t.Fatal(err)
	}
	tags, err := st.TagsForConvention(id)
	if err != nil {
		t.Fatal(err)
	}
	faculties := 0
	for _, tag := range tags {
		if tag.Tipo == "facultad" {
			faculties++
		}
	}
	if faculties != 1 {
		t.Fatalf("facultades = %d, %+v", faculties, tags)
	}
}
