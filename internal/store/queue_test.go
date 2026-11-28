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
	if err := st.EnqueueForAI([]int64{queued[2], queued[0], queued[1]}); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.QueuedIDs(); len(got) != 3 || got[0] != queued[2] || got[1] != queued[0] || got[2] != queued[1] {
		t.Fatalf("orden = %v", got)
	}
	// Repetir no duplica.
	if err := st.EnqueueForAI([]int64{queued[0]}); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.QueueCount(); n != 3 {
		t.Fatalf("count = %d", n)
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
	if err := st.EnqueueForAI([]int64{id}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteMany([]int64{id}); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.QueueCount(); n != 0 {
		t.Fatalf("la fila no se borró con el convenio: %d", n)
	}
}

func TestPruneQueueDropsFinishedAndArchived(t *testing.T) {
	st, _ := testStore(t)
	ok := insertSample(t, st, "ok")
	errID := insertSample(t, st, "err")
	arch := insertSample(t, st, "arch")
	if err := st.SetAIResult(ok, ConventionInput{ID: ok, Dependencia: "X"}, "ok", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetIAStatus(errID, "error", "HTTP 500"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetArchived([]int64{arch}, true); err != nil {
		t.Fatal(err)
	}
	if err := st.EnqueueForAI([]int64{ok, errID, arch}); err != nil {
		t.Fatal(err)
	}
	if err := st.PruneQueue(); err != nil {
		t.Fatal(err)
	}
	got, _ := st.QueuedIDs()
	if len(got) != 1 || got[0] != errID {
		t.Fatalf("tras PruneQueue = %v", got)
	}
	if err := st.ClearQueue(); err != nil || func() bool { n, _ := st.QueueCount(); return n != 0 }() {
		t.Fatalf("ClearQueue no vació: %v", err)
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
	if err := st.EnqueueForAI([]int64{id}); err != nil {
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
