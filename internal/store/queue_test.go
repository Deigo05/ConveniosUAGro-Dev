package store

import (
	"testing"
)

func TestQueueOrderAndDedup(t *testing.T) {
	st, _ := testStore(t)
	var queued []int64
	for _, n := range []string{"a", "b", "c"} {
		queued = append(queued, insertSample(t, st, n))
	}
	if err := st.EnqueueForAI([]int64{queued[2], queued[0], queued[1]}, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.QueuedIDs(); len(got) != 3 || got[0] != queued[2] || got[1] != queued[0] || got[2] != queued[1] {
		t.Fatalf("orden = %v", got)
	}
	// Repetir no duplica.
	if err := st.EnqueueForAI([]int64{queued[0]}, false); err != nil {
		t.Fatal(err)
	}
	if n, err := st.QueueCount(); err != nil || n != 3 {
		t.Fatalf("count = %d, %v", n, err)
	}
	if err := st.Dequeue(queued[0]); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.QueuedIDs(); len(got) != 2 || got[0] != queued[2] || got[1] != queued[1] {
		t.Fatalf("tras Dequeue = %v", got)
	}
}

func TestQueueCascadesOnDelete(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "x")
	if err := st.EnqueueForAI([]int64{id}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteMany([]int64{id}); err != nil {
		t.Fatal(err)
	}
	if n, err := st.QueueCount(); err != nil || n != 0 {
		t.Fatalf("la fila no se borró con el convenio: %d, %v", n, err)
	}
}

// PruneQueue respeta el trabajo pendiente: una importación recién encolada
// sobrevive aunque la IA aún no la toque, y una transcripción a medias también.
func TestPruneQueueKeepsPendingWork(t *testing.T) {
	st, _ := testStore(t)
	fresh := insertSample(t, st, "fresco")
	if err := st.EnqueueForAI([]int64{fresh}, false); err != nil {
		t.Fatal(err)
	}
	if err := st.PruneQueue(); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.QueuedIDs(); len(got) != 1 || got[0] != fresh {
		t.Fatalf("la importación fresca se perdió: %v", got)
	}

	// Transcripción a medias y análisis sin terminar: sigue pendiente.
	medio := insertSample(t, st, "medio")
	if err := st.BeginTranscription(medio, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(medio, 1, "texto"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetIAStatus(medio, "error", "HTTP 500"); err != nil {
		t.Fatal(err)
	}
	if err := st.EnqueueForAI([]int64{medio}, false); err != nil {
		t.Fatal(err)
	}
	if err := st.PruneQueue(); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.QueuedIDs(); len(got) != 2 {
		t.Fatalf("se perdió trabajo pendiente: %v", got)
	}
}

// Un reanálisis pedido a mano sobre un convenio ya analizado sobrevive a la
// limpieza; un convenio que ya terminó (o se archivó) sale de la cola.
func TestPruneQueueDropsFinishedButKeepsReanalysis(t *testing.T) {
	st, _ := testStore(t)
	finished := insertSample(t, st, "terminado")
	re := insertSample(t, st, "reanalisis")
	arch := insertSample(t, st, "archivado")
	for _, id := range []int64{finished, re} {
		if err := st.SetAIResult(id, ConventionInput{ID: id, Dependencia: "X"}, "ok", nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.SetArchived([]int64{arch}, true); err != nil {
		t.Fatal(err)
	}
	if err := st.EnqueueForAI([]int64{finished}, false); err != nil {
		t.Fatal(err)
	}
	if err := st.EnqueueForAI([]int64{re}, true); err != nil {
		t.Fatal(err)
	}
	if err := st.EnqueueForAI([]int64{arch}, false); err != nil {
		t.Fatal(err)
	}
	if err := st.PruneQueue(); err != nil {
		t.Fatal(err)
	}
	got, _ := st.QueuedIDs()
	if len(got) != 1 || got[0] != re {
		t.Fatalf("tras PruneQueue = %v; esperaba solo %d", got, re)
	}
	if err := st.ClearQueue(); err != nil {
		t.Fatal(err)
	}
	if n, err := st.QueueCount(); err != nil || n != 0 {
		t.Fatalf("ClearQueue no vació: %d, %v", n, err)
	}
}

func TestQueueSurvivesReopen(t *testing.T) {
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
	id := insertSample(t, st, "q")
	if err := st.EnqueueForAI([]int64{id}, false); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st2, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if got, _ := st2.QueuedIDs(); len(got) != 1 || got[0] != id {
		t.Fatalf("la cola no sobrevivió: %v", got)
	}
}

// Al pausar, la transcripción vuelve a «pendiente» y conserva sus páginas.
func TestSuspendTranscriptionKeepsPages(t *testing.T) {
	st, _ := testStore(t)
	id := insertSample(t, st, "pausa")
	if err := st.BeginTranscription(id, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveTranscriptionPage(id, 1, "página uno"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetIAStatus(id, "error", "el servicio falló"); err != nil {
		t.Fatal(err)
	}
	if err := st.SuspendTranscription(id); err != nil {
		t.Fatal(err)
	}
	stt, err := st.GetTranscription(id)
	if err != nil {
		t.Fatal(err)
	}
	if stt.Estado != "pendiente" || stt.PaginasTranscritas != 1 {
		t.Fatalf("tras pausar: estado=%q paginas=%d", stt.Estado, stt.PaginasTranscritas)
	}
	done, err := st.ResumeTranscription(id, 3)
	if err != nil || len(done) != 1 || done[0] != 1 {
		t.Fatalf("retomar = %v, %v", done, err)
	}
}
