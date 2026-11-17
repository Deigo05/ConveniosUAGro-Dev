package municipios

import "testing"

func TestOficial(t *testing.T) {
	cases := []struct {
		nombre, estado string
		want           string
		ok             bool
	}{
		{"Acapulco", "", "Acapulco de Juárez", true},
		{"ACAPULCO", "Guerrero", "Acapulco de Juárez", true},
		{"Acapulco, Gro.", "", "Acapulco de Juárez", true},
		{"Acapulco Guerrero", "", "Acapulco de Juárez", true},
		{"Acapulco de Juarez", "", "Acapulco de Juárez", true},
		{"Acapulco de Juárez", "Guerrero", "Acapulco de Juárez", true},
		{"Chilpancingo", "guerrero", "Chilpancingo de los Bravo", true},
		{"Chilpancingo de los Bravos", "", "Chilpancingo de los Bravo", true},
		{"Iguala", "", "Iguala de la Independencia", true},
		{"Taxco", "", "Taxco de Alarcón", true},
		{"Zihuatanejo", "", "Zihuatanejo de Azueta", true},
		{"Tlapa", "", "Tlapa de Comonfort", true},
		{"Tixtla", "", "Tixtla de Guerrero", true},
		{"Tixtla de Guerrero", "", "Tixtla de Guerrero", true},
		// Ayutla también es un municipio de Jalisco: sin estado no se toca.
		{"Ayutla", "", "", false},
		{"Ayutla", "Jalisco", "", false},
		{"Ayutla", "Guerrero", "Ayutla de los Libres", true},
		{"Ayutla, Gro.", "", "Ayutla de los Libres", true},
		// Otro estado o un municipio fuera del catálogo no cambian.
		{"Acapulco", "Veracruz", "", false},
		{"Huauchinango", "Puebla", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		got, est, ok := Oficial(c.nombre, c.estado)
		if ok != c.ok || got != c.want {
			t.Errorf("Oficial(%q, %q) = %q, %v; want %q, %v", c.nombre, c.estado, got, ok, c.want, c.ok)
		}
		if ok && est != "Guerrero" {
			t.Errorf("Oficial(%q) estado = %q", c.nombre, est)
		}
	}
}
