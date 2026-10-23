package uagro

import "testing"

func TestResolveTurismoNeverUsesDependencyCity(t *testing.T) {
	name, city, ok := Resolve("Facultad de Turismo", "Tijuana")
	if !ok || name != "Facultad de Turismo" || city != "Acapulco" {
		t.Fatalf("Resolve = %q, %q, %v", name, city, ok)
	}
	name, city, ok = Resolve("Facultad de Turismo Acapulco", "")
	if !ok || name != "Facultad de Turismo" || city != "Acapulco" {
		t.Fatalf("Resolve embedded = %q, %q, %v", name, city, ok)
	}
	name, city, ok = Resolve("Facultad de Turismo Tijuana", "")
	if !ok || name != "Facultad de Turismo" || city != "Acapulco" {
		t.Fatalf("Resolve malformed = %q, %q, %v", name, city, ok)
	}
}

func TestResolveDerechoNeedsCityWhenAmbiguous(t *testing.T) {
	if _, _, ok := Resolve("Facultad de Derecho", ""); ok {
		t.Fatal("Derecho no debe resolverse sin ciudad")
	}
	name, city, ok := Resolve("Facultad de Derecho", "Chilpancingo")
	if !ok || name != "Facultad de Derecho" || city != "Chilpancingo" {
		t.Fatalf("Derecho = %q, %q, %v", name, city, ok)
	}
	// Tijuana no es sede de la UAGro y Derecho es ambiguo.
	if _, _, ok := Resolve("Facultad de Derecho", "Tijuana"); ok {
		t.Fatal("no debe inventarse Derecho Tijuana")
	}
}

func TestResolveUnknownRequiresUAGroCampus(t *testing.T) {
	if _, _, ok := Resolve("Facultad de Gastronomía", "Tijuana"); ok {
		t.Fatal("no debe aceptar una ciudad que no es sede UAGro")
	}
	name, city, ok := Resolve("Facultad de Gastronomía", "Acapulco")
	if !ok || name != "Facultad de Gastronomía" || city != "Acapulco" {
		t.Fatalf("desconocida = %q, %q, %v", name, city, ok)
	}
}

func TestResolveNumberAndCityAlias(t *testing.T) {
	name, city, ok := Resolve("Facultad de Ciencias de la Educación núm. 2", "Acapulco de Juárez")
	if !ok || name != "Facultad de Ciencias de la Educación 02" || city != "Acapulco" {
		t.Fatalf("educación = %q, %q, %v", name, city, ok)
	}
}
