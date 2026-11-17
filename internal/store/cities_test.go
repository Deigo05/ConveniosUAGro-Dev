package store

import "testing"

// Una base de una versión anterior con «Acapulco» y «Acapulco de Juárez»:
// al unificar, todos los convenios quedan con la oficial y la corta desaparece.
func TestUnifyCityNamesMergesShortName(t *testing.T) {
	st, _ := testStore(t)
	oficial, err := st.CreateTag(TagInput{Nombre: "Acapulco de Juárez", Tipo: "ciudad", Estado: "Guerrero"})
	if err != nil {
		t.Fatal(err)
	}
	// Simula la etiqueta que creaban las versiones anteriores.
	res, err := st.db.Exec("INSERT INTO etiquetas (nombre, tipo, color, protegida, estado) VALUES ('Acapulco', 'ciudad', '#2E7D32', 0, 'Guerrero')")
	if err != nil {
		t.Fatal(err)
	}
	corta, _ := res.LastInsertId()
	// Y una ciudad corta sin etiqueta oficial todavía, sin estado.
	res, err = st.db.Exec("INSERT INTO etiquetas (nombre, tipo, color, protegida, estado) VALUES ('Chilpancingo', 'ciudad', '#2E7D32', 0, '')")
	if err != nil {
		t.Fatal(err)
	}
	chilpo, _ := res.LastInsertId()

	a := insertSample(t, st, "convenio a")
	b := insertSample(t, st, "convenio b")
	c := insertSample(t, st, "convenio c")
	for id, city := range map[int64]int64{a: corta, b: oficial.ID, c: chilpo} {
		if _, err := st.db.Exec("INSERT INTO convenio_etiquetas (convenio_id, etiqueta_id) VALUES (?, ?)", id, city); err != nil {
			t.Fatal(err)
		}
	}

	n, err := st.UnifyCityNames()
	if err != nil || n != 2 {
		t.Fatalf("UnifyCityNames = %d, %v; want 2", n, err)
	}
	if got := tagNames(t, st, a)["ciudad"]; got != "Acapulco de Juárez" {
		t.Fatalf("convenio a: ciudad = %q", got)
	}
	if got := tagNames(t, st, b)["ciudad"]; got != "Acapulco de Juárez" {
		t.Fatalf("convenio b: ciudad = %q", got)
	}
	if got := tagNames(t, st, c)["ciudad"]; got != "Chilpancingo de los Bravo" {
		t.Fatalf("convenio c: ciudad = %q", got)
	}
	if _, err := st.TagByID(corta); err == nil {
		t.Fatal("la etiqueta «Acapulco» debía eliminarse")
	}
	tags, err := st.ListTags()
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range tags {
		if tag.Tipo != "ciudad" {
			continue
		}
		switch tag.Nombre {
		case "Acapulco de Juárez":
			if tag.Total != 2 || tag.Estado != "Guerrero" {
				t.Fatalf("Acapulco de Juárez = %+v", tag)
			}
		case "Chilpancingo de los Bravo":
			if tag.Estado != "Guerrero" {
				t.Fatalf("Chilpancingo sin estado: %+v", tag)
			}
		default:
			t.Fatalf("ciudad inesperada: %+v", tag)
		}
	}
	// La búsqueda encuentra al convenio por el nombre nuevo de su ciudad.
	list, err := st.List(ListFilter{Query: "Juárez"})
	if err != nil || list.Total != 2 {
		t.Fatalf("búsqueda por Juárez = %d, %v", list.Total, err)
	}
	// Idempotente.
	if n, err := st.UnifyCityNames(); err != nil || n != 0 {
		t.Fatalf("segunda vez = %d, %v", n, err)
	}
}

// Las ciudades nuevas, sugeridas por la IA o creadas a mano, usan el nombre
// oficial y no duplican la etiqueta que ya existe.
func TestCityTagsUseOfficialName(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "convenio")
	if err := st.SetSuggestions(id, []Suggestion{{Tipo: "estado", Nombre: "Guerrero"}, {Tipo: "ciudad", Nombre: "Acapulco", Nueva: true}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptSuggestions(id); err != nil {
		t.Fatal(err)
	}
	if got := tagNames(t, st, id)["ciudad"]; got != "Acapulco de Juárez" {
		t.Fatalf("ciudad sugerida = %q", got)
	}
	// Otro convenio con «Acapulco, Gro.» usa la misma etiqueta.
	id2 := insertSample(t, st, "otro convenio")
	if err := st.SetSuggestions(id2, []Suggestion{{Tipo: "ciudad", Nombre: "Acapulco, Gro.", Nueva: true}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptSuggestions(id2); err != nil {
		t.Fatal(err)
	}
	if got := tagNames(t, st, id2); got["ciudad"] != "Acapulco de Juárez" || got["estado"] != "Guerrero" {
		t.Fatalf("segunda ciudad = %v", got)
	}
	// Crear «Acapulco» a mano avisa que ya existe la oficial.
	if _, err := st.CreateTag(TagInput{Nombre: "Acapulco", Tipo: "ciudad"}); err == nil {
		t.Fatal("crear «Acapulco» debía avisar que ya existe «Acapulco de Juárez»")
	}
	count := 0
	tags, _ := st.ListTags()
	for _, tag := range tags {
		if tag.Tipo == "ciudad" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("hay %d etiquetas de ciudad, want 1", count)
	}
}
