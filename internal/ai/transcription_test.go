package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscribeImageRequest(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("ruta = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("autorización = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"Texto transcrito de la página"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	client := New("test-key", srv.URL, "text-model", "vision-model")
	texto, err := client.TranscribeImage(context.Background(), "aW1hZ2Vu", 2)
	if err != nil {
		t.Fatal(err)
	}
	if texto != "Texto transcrito de la página" {
		t.Fatalf("texto = %q", texto)
	}
	if received["model"] != "vision-model" {
		t.Fatalf("modelo = %v", received["model"])
	}
	raw, _ := json.Marshal(received)
	if !strings.Contains(string(raw), "data:image/jpeg;base64,aW1hZ2Vu") {
		t.Fatalf("no se envió la imagen: %s", raw)
	}
}

func TestAnswerRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "Convenio #3") {
			t.Errorf("no se enviaron las fuentes")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"Respuesta con cita [Convenio #3, página 2]."},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	client := New("test-key", srv.URL, "text-model", "vision-model")
	got, err := client.Answer(context.Background(), "¿Qué dice?", "[Convenio #3 | Página 2] texto")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Convenio #3") {
		t.Fatalf("respuesta = %q", got)
	}
}
