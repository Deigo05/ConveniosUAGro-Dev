package store

import (
	"testing"
)

func TestRetryableError(t *testing.T) {
	for msg, want := range map[string]bool{
		"el servicio de IA tardó demasiado en responder":                        true,
		"se alcanzó el límite de uso de la IA; intente más tarde":               true,
		"el servicio de IA devolvió un formato inválido":                        true,
		"no se pudo conectar con el servicio de IA":                             true,
		"la clave de IA es inválida":                                            false,
		"el PDF no tiene texto seleccionable; capture los datos manualmente":    false,
		"configure y active la IA antes de transcribir":                         false,
		"Activa la IA y configura la clave en Ajustes.":                         false,
		MsgAnalisisInterrumpido:                                                 true,
		"el servicio rechazó la solicitud; si es una imagen, revise el modelo…": false,
	} {
		if got := RetryableError(msg); got != want {
			t.Errorf("RetryableError(%q) = %v, want %v", msg, got, want)
		}
	}
}

// Los convenios con error pasajero se reintentan al abrir, como máximo
// MaxRetryRounds veces; un análisis correcto reinicia la cuenta.
func TestRetryCandidates(t *testing.T) {
	st, _ := testStore(t)
	iaErr := insertSample(t, st, "a")
	permanente := insertSample(t, st, "b")
	transErr := insertSample(t, st, "c")
	interrumpida := insertSample(t, st, "d")
	ok := insertSample(t, st, "e")
	sinEmpezar := insertSample(t, st, "f")
	mustExec(t, st, "UPDATE convenios SET ia_estado='error', ia_error='el servicio de IA tardó demasiado en responder' WHERE id=?", iaErr)
	mustExec(t, st, "UPDATE convenios SET ia_estado='error', ia_error='el PDF no tiene texto seleccionable; capture los datos manualmente' WHERE id=?", permanente)
	mustExec(t, st, "UPDATE convenios SET transcripcion_estado='error', transcripcion_error='HTTP 503', paginas_por_transcribir=3, paginas_transcritas=1 WHERE id=?", transErr)
	mustExec(t, st, "UPDATE convenios SET transcripcion_estado='pendiente', paginas_por_transcribir=4, paginas_transcritas=2 WHERE id=?", interrumpida)
	mustExec(t, st, "UPDATE convenios SET ia_estado='ok' WHERE id=?", ok)
	// Importado sin IA activa: no es un error, no se procesa solo.
	mustExec(t, st, "UPDATE convenios SET transcripcion_estado='pendiente', paginas_por_transcribir=4, paginas_transcritas=0 WHERE id=?", sinEmpezar)

	want := []int64{iaErr, transErr, interrumpida}
	for round := 1; round <= MaxRetryRounds; round++ {
		got, err := st.RetryCandidates()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("ronda %d: candidatos = %v, want %v", round, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("ronda %d: candidatos = %v, want %v", round, got, want)
			}
		}
	}
	if got, _ := st.RetryCandidates(); len(got) != 0 {
		t.Fatalf("después de %d rondas no debía reintentar: %v", MaxRetryRounds, got)
	}
	// Un análisis correcto reinicia la cuenta: si vuelve a fallar, se reintenta.
	if err := st.SetAIResult(iaErr, ConventionInput{ID: iaErr, Dependencia: "X"}, "ok", nil, ""); err != nil {
		t.Fatal(err)
	}
	mustExec(t, st, "UPDATE convenios SET ia_estado='error', ia_error='HTTP 500' WHERE id=?", iaErr)
	if got, _ := st.RetryCandidates(); len(got) != 1 || got[0] != iaErr {
		t.Fatalf("tras un análisis correcto = %v, want [%d]", got, iaErr)
	}
}

// Un análisis que quedó «pendiente» al cerrar la app pasa a error al abrirla,
// para que se reintente en lugar de quedarse «en análisis» para siempre.
func TestOpenResetsInterruptedAnalysis(t *testing.T) {
	st, paths := testStore(t)
	id := insertSample(t, st, "a")
	if err := st.SetIAStatus(id, "pendiente", ""); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st2, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	d, err := st2.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if d.IAEstado != "error" || d.IAError != MsgAnalisisInterrumpido {
		t.Fatalf("ia = %q, %q", d.IAEstado, d.IAError)
	}
}

// Retomar conserva las páginas ya transcritas.
func TestResumeTranscriptionKeepsPages(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "")
	if err := st.BeginTranscription(id, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(id, 1, "uno"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(id, 3, "tres"); err != nil {
		t.Fatal(err)
	}
	if err := st.FailTranscription(id, "HTTP 503"); err != nil {
		t.Fatal(err)
	}
	done, err := st.ResumeTranscription(id, 3)
	if err != nil || len(done) != 2 || done[0] != 1 || done[1] != 3 {
		t.Fatalf("páginas hechas = %v, %v", done, err)
	}
	status, err := st.SaveTranscriptionPage(id, 2, "dos")
	if err != nil {
		t.Fatal(err)
	}
	if status.Estado != "completa" || status.PaginasTranscritas != 3 {
		t.Fatalf("estado = %+v", status)
	}
}

func mustExec(t *testing.T, st *Store, q string, args ...interface{}) {
	t.Helper()
	if _, err := st.db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}
