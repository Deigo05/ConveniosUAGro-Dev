package store

import (
	"strings"
	"testing"
)

const textoConCierre = "PRIMERA. OBJETO. Colaborar en servicio social.\n" +
	"Leído que fue, lo firman por duplicado en Chilpancingo, Guerrero, a los veinticinco (25) días del mes de noviembre de dos mil veinticinco (2025).\n[Firma]"

func TestDateChecksMarcanYConfirman(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, strings.Repeat("f", 32)+".pdf", "sha-fechas", textoConCierre)
	if err := st.Update(ConventionInput{ID: id, Dependencia: "Ayuntamiento", FechaFirma: "2025-09-25"}); err != nil {
		t.Fatal(err)
	}
	if err := st.RefreshDateChecks(); err != nil {
		t.Fatal(err)
	}
	d, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.RevisionFechas) != 1 || d.RevisionFechas[0].Campo != "firma" || d.RevisionFechas[0].Propuesta != "2025-11-25" {
		t.Fatalf("revisión = %+v", d.RevisionFechas)
	}
	res, _ := st.List(ListFilter{SoloFechasPorConfirmar: true})
	if res.Total != 1 || len(res.Items[0].FechasPorConfirmar) != 1 {
		t.Fatalf("filtro: total=%d %+v", res.Total, res.Items)
	}
	stats, _ := st.Stats()
	if stats.FechasPorConfirmar != 1 {
		t.Fatalf("contador = %d", stats.FechasPorConfirmar)
	}
	if !st.HasPendingFirma(id) {
		t.Fatal("HasPendingFirma debe ser verdadero")
	}

	// El usuario la confirma: la marca desaparece y no vuelve al recalcular.
	if err := st.ConfirmDate(id, "firma"); err != nil {
		t.Fatal(err)
	}
	if err := st.RefreshDateChecks(); err != nil {
		t.Fatal(err)
	}
	if d, _ := st.Get(id); len(d.RevisionFechas) != 0 {
		t.Fatalf("confirmada sigue marcada: %+v", d.RevisionFechas)
	}
	if res, _ := st.List(ListFilter{SoloFechasPorConfirmar: true}); res.Total != 0 {
		t.Fatalf("confirmada sigue en el filtro")
	}

	// Si la fecha cambia a otra que tampoco coincide, la marca vuelve.
	if err := st.Update(ConventionInput{ID: id, Dependencia: "Ayuntamiento", FechaFirma: "2025-10-25"}); err != nil {
		t.Fatal(err)
	}
	if err := st.RefreshDateChecks(); err != nil {
		t.Fatal(err)
	}
	if d, _ := st.Get(id); len(d.RevisionFechas) != 1 {
		t.Fatalf("una fecha nueva distinta debe marcarse otra vez: %+v", d.RevisionFechas)
	}

	// Con la fecha del cierre, no queda nada por confirmar.
	if err := st.Update(ConventionInput{ID: id, Dependencia: "Ayuntamiento", FechaFirma: "2025-11-25"}); err != nil {
		t.Fatal(err)
	}
	if err := st.RefreshDateChecks(); err != nil {
		t.Fatal(err)
	}
	if d, _ := st.Get(id); len(d.RevisionFechas) != 0 || len(d.FechasPorConfirmar) != 0 {
		t.Fatalf("fecha correcta marcada: %+v", d.RevisionFechas)
	}
}

func TestDateChecksSeRepitenCuandoCambiaElTexto(t *testing.T) {
	st, paths := testStore(t)
	id := insertWithPDF(t, st, paths, strings.Repeat("g", 32)+".pdf", "sha-fechas-2", "Sin cierre todavía.")
	if err := st.Update(ConventionInput{ID: id, Dependencia: "Hotel", FechaFirma: "2025-09-25"}); err != nil {
		t.Fatal(err)
	}
	if err := st.RefreshDateChecks(); err != nil {
		t.Fatal(err)
	}
	if d, _ := st.Get(id); len(d.RevisionFechas) != 0 {
		t.Fatalf("sin cierre no se marca: %+v", d.RevisionFechas)
	}
	// La transcripción agrega el cierre (rebuildText reescribe el texto).
	if err := st.BeginTranscription(id, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(id, 1, textoConCierre); err != nil {
		t.Fatal(err)
	}
	if err := st.RefreshDateChecks(); err != nil {
		t.Fatal(err)
	}
	if d, _ := st.Get(id); len(d.RevisionFechas) != 1 {
		t.Fatalf("tras transcribir debe marcarse: %+v", d.RevisionFechas)
	}
}
