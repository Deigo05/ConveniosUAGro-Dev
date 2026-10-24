package uagro

import "testing"

func check(t *testing.T, facultad, city, wantName, wantCity string, wantOK bool) {
	t.Helper()
	name, sede, ok := Resolve(facultad, city)
	if ok != wantOK || name != wantName || sede != wantCity {
		t.Fatalf("Resolve(%q, %q) = %q, %q, %v; se esperaba %q, %q, %v", facultad, city, name, sede, ok, wantName, wantCity, wantOK)
	}
}

func TestResolveTurismoNeverUsesDependencyCity(t *testing.T) {
	check(t, "Facultad de Turismo", "Tijuana", "Facultad de Turismo", "Acapulco", true)
	check(t, "Facultad de Turismo Acapulco", "", "Facultad de Turismo", "Acapulco", true)
	check(t, "Facultad de Turismo Tijuana", "", "Facultad de Turismo", "Acapulco", true)
	// Taxco es sede de la UAGro, pero no de Turismo: es la ciudad de la contraparte.
	check(t, "Facultad de Turismo", "Taxco de Alarcón", "Facultad de Turismo", "Acapulco", true)
	check(t, "Facultad de Turismo", "Zihuatanejo", "Facultad de Turismo", "Zihuatanejo", true)
}

// Nombres anteriores o abreviados de la misma facultad no crean otra etiqueta.
func TestResolveOldNamesAndAbbreviations(t *testing.T) {
	check(t, "Unidad Académica de Turismo", "Acapulco de Juárez", "Facultad de Turismo", "Acapulco", true)
	check(t, "Unidad Académica de Contaduría y Administración", "", "Facultad de Contaduría y Administración", "Acapulco", true)
	check(t, "Escuela Superior de Nutrición y Ciencia de los Alimentos", "Acapulco de Juárez", "Escuela Superior de Nutrición y Ciencia de los Alimentos", "Acapulco", true)
	check(t, "Facultad de Nutrición y Ciencias de los Alimentos", "", "Escuela Superior de Nutrición y Ciencia de los Alimentos", "Acapulco", true)
	check(t, "EMVZ No. 3", "", "Escuela Superior de Medicina Veterinaria y Zootecnia 03", "Tecpan de Galeana", true)
	check(t, "EMVZ N0. 3", "", "Escuela Superior de Medicina Veterinaria y Zootecnia 03", "Tecpan de Galeana", true)
	check(t, "Facultad de Medicina Veterinaria y Zootecnia No. 2", "Cuajinicuilapa", "Facultad de Medicina Veterinaria y Zootecnia 02", "Cuajinicuilapa", true)
	check(t, "Facultad de Turismo de la UAGro", "", "Facultad de Turismo", "Acapulco", true)
}

func TestResolveAmbiguousWithoutCity(t *testing.T) {
	// Dos sedes y ninguna pista: la etiqueta queda sin ciudad.
	check(t, "Facultad de Derecho", "", "Facultad de Derecho", "", true)
	check(t, "Facultad de Derecho", "Chilpancingo", "Facultad de Derecho", "Chilpancingo", true)
	check(t, "Facultad de Derecho", "Tijuana", "Facultad de Derecho", "", true)
	check(t, "Facultad de Enfermería", "Acapulco", "Facultad de Enfermería 02", "Acapulco", true)
	check(t, "Facultad de Enfermería", "", "Facultad de Enfermería", "", true)
	// El número de unidad manda: Enfermería 3 no es Enfermería 2.
	check(t, "Escuela Superior de Enfermería No. 3", "Ometepec", "Escuela Superior de Enfermería 03", "Ometepec", true)
}

func TestResolveUnknownUnits(t *testing.T) {
	check(t, "Facultad de Gastronomía", "Acapulco", "Facultad de Gastronomía", "Acapulco", true)
	check(t, "Facultad de Gastronomía", "Tijuana", "Facultad de Gastronomía", "", true)
	check(t, "Unidad Académica de Gastronomía", "", "Facultad de Gastronomía", "", true)
	// Lo que no es una unidad académica deja el convenio como abierto.
	check(t, "Rectoría", "Chilpancingo", "", "", false)
	check(t, "Coordinación General Zona Sur", "", "", "", false)
}

func TestResolveNumberAndCityAlias(t *testing.T) {
	check(t, "Facultad de Ciencias de la Educación núm. 2", "Acapulco de Juárez", "Facultad de Ciencias de la Educación 02", "Acapulco", true)
	check(t, "Facultad de Ciencias de la Educación", "Acapulco, Guerrero", "Facultad de Ciencias de la Educación 02", "Acapulco", true)
}

func TestCatalogText(t *testing.T) {
	text := CatalogText()
	if want := "Facultad de Turismo (Acapulco, Zihuatanejo)"; !contains(text, want) {
		t.Fatalf("falta %q en %q", want, text)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
