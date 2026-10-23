package store

import (
	"strings"
	"testing"

	"convenios-uagro-desktop/internal/pdfutil"
)

func TestTranscriptionUpdatesSearch(t *testing.T) {
	st, _ := testStore(t)
	id, err := st.Insert(pdfutil.SavedFile{
		NombreGuardado:  "cccccccccccccccccccccccccccccccc.pdf",
		NombreOriginal:  "escaneado.pdf",
		Paginas:         1,
		PaginasSinTexto: 1,
		Escaneado:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BeginTranscription(id, 1); err != nil {
		t.Fatal(err)
	}
	stt, err := st.SaveTranscriptionPage(id, 1, "Convenio de colaboración con el Ayuntamiento de Acapulco")
	if err != nil {
		t.Fatal(err)
	}
	if stt.Estado != "completa" || stt.PaginasTranscritas != 1 {
		t.Fatalf("estado = %+v", stt)
	}
	detail, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail.Texto, "Ayuntamiento de Acapulco") {
		t.Fatalf("texto no reconstruido: %q", detail.Texto)
	}
	res, err := st.List(ListFilter{Query: "acapulco"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("la transcripción no entró a FTS: total=%d", res.Total)
	}
}
