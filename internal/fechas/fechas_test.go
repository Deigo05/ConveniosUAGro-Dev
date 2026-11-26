package fechas

import (
	"reflect"
	"testing"
)

func TestFindDates(t *testing.T) {
	cases := []struct {
		in   string
		want []string // ISO de cada fecha completa encontrada
	}{
		{"Chilpancingo, Gro., 25 de noviembre de 2025.", []string{"2025-11-25"}},
		{"a los veinticinco (25) días del mes de noviembre del año dos mil veinticinco (2025).", []string{"2025-11-25"}},
		{"a los quince días del mes de agosto de dos mil veintidós", []string{"2022-08-15"}},
		{"el treinta y uno de diciembre de dos mil veintisiete", []string{"2027-12-31"}},
		{"el 1o. de enero de 2020 y el 3 de marzo de 2024", []string{"2020-01-01", "2024-03-03"}},
		{"firmado el 25/11/2025", []string{"2025-11-25"}},
		{"el mes de noviembre de 2025", nil}, // sin día no es fecha completa
		{"el primero de julio de mil novecientos noventa y nueve", []string{"1999-07-01"}},
	}
	for _, c := range cases {
		var got []string
		for _, d := range FindDates(c.in) {
			got = append(got, d.ISO)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("FindDates(%q) = %v, quiero %v", c.in, got, c.want)
		}
	}
}

func TestFindDatesConflicto(t *testing.T) {
	d := FindDates("a los veinticinco (26) días del mes de mayo de 2021")
	if len(d) != 1 || !d[0].Conflicto {
		t.Fatalf("letra y número distintos deben marcar conflicto: %+v", d)
	}
	d = FindDates("el 31 de junio de 2021")
	if len(d) != 1 || !d[0].Conflicto || d[0].ISO != "" {
		t.Fatalf("un día imposible debe marcar conflicto sin ISO: %+v", d)
	}
}

func TestFindDatesTextoOriginal(t *testing.T) {
	d := FindDates("lo firman en Taxco, a los Quince días del mes de Agosto de 2022.")
	if len(d) != 1 || d[0].Texto != "Quince días del mes de Agosto de 2022" {
		t.Fatalf("Texto debe ser el fragmento original: %+v", d)
	}
}

const cierre = "SEGUNDA. VIGENCIA. El presente convenio tendrá una vigencia de tres (3) años.\n" +
	"Leído que fue el presente convenio, lo firman por duplicado en Chilpancingo, Guerrero, " +
	"a los veinticinco (25) días del mes de noviembre del año dos mil veinticinco (2025).\n[Firma] [Sello]"

func campos(rs []Result) map[string]Result {
	m := map[string]Result{}
	for _, r := range rs {
		m[r.Campo] = r
	}
	return m
}

func TestCheckFirmaCoincide(t *testing.T) {
	rs := Check(Input{Texto: cierre, FechaFirma: "2025-11-25"})
	if r, ok := campos(rs)[Firma]; ok {
		t.Fatalf("firma igual al cierre no debe marcarse: %+v", r)
	}
}

func TestCheckFirmaDistinta(t *testing.T) {
	r, ok := campos(Check(Input{Texto: cierre, FechaFirma: "2025-09-25"}))[Firma]
	if !ok || r.Propuesta != "2025-11-25" {
		t.Fatalf("firma distinta del cierre debe marcarse con la fecha del cierre: %+v", r)
	}
	if r.Cita == "" {
		t.Fatalf("debe citar el fragmento del documento: %+v", r)
	}
}

func TestCheckFirmaVacia(t *testing.T) {
	r, ok := campos(Check(Input{Texto: cierre}))[Firma]
	if !ok || r.Propuesta != "2025-11-25" {
		t.Fatalf("firma vacía con fecha en el cierre debe proponerla: %+v", r)
	}
}

func TestCheckFirmaConDudaDeLectura(t *testing.T) {
	txt := "Leído el presente, lo firman en Taxco, Guerrero, a los quince[?] días del mes de agosto de 2022."
	r, ok := campos(Check(Input{Texto: txt, FechaFirma: "2022-08-15"}))[Firma]
	if !ok || r.Propuesta != "" {
		t.Fatalf("una fecha marcada [?] en la transcripción se marca aunque coincida: %+v", r)
	}
}

func TestCheckFirmaLetraNumeroDistintos(t *testing.T) {
	txt := "Leído el presente, lo firman en Taxco, a los veinticinco (26) días del mes de mayo de 2021."
	r, ok := campos(Check(Input{Texto: txt, FechaFirma: "2021-05-26"}))[Firma]
	if !ok || r.Propuesta != "" {
		t.Fatalf("letra y número distintos se marcan sin propuesta: %+v", r)
	}
}

func TestCheckFirmaCierreIlegible(t *testing.T) {
	txt := "PRIMERA. OBJETO. Colaborar.\nLeído el presente, lo firman en Acapulco, Gro., a los [ilegible] días del mes de [Sello] de 2021.\n[Firma]"
	r, ok := campos(Check(Input{Texto: txt, FechaFirma: "2021-03-02"}))[Firma]
	if !ok {
		t.Fatalf("un cierre sin fecha legible con fecha capturada debe marcarse")
	}
	_ = r
}

func TestCheckMencionDeFirmaSinRastroDeFechaNoMarca(t *testing.T) {
	rs := Check(Input{Texto: "Convenio que renueva el firmado en 2021", FechaFirma: "2024-02-01"})
	if len(rs) != 0 {
		t.Fatalf("una mención sin rastro de fecha no es un cierre ilegible: %+v", rs)
	}
}

func TestCheckSinCierreNoMarca(t *testing.T) {
	rs := Check(Input{Texto: "Un texto sin fórmula de firma. El 3 de marzo de 2024 se dio de alta.", FechaFirma: "2024-01-01"})
	if len(rs) != 0 {
		t.Fatalf("sin cierre no hay contra qué comparar: %+v", rs)
	}
}

func TestCheckUsaElUltimoCierre(t *testing.T) {
	txt := "Su nombramiento se firmó el 2 de enero de 2019.\nLeído el presente, lo firman en Iguala, a los 10 días del mes de junio de 2023."
	if rs := Check(Input{Texto: txt, FechaFirma: "2023-06-10"}); len(rs) != 0 {
		t.Fatalf("el cierre es el último: %+v", rs)
	}
}

func TestCheckVigenciaExplicita(t *testing.T) {
	in := Input{Texto: cierre, FechaFirma: "2025-11-25", FechaVigencia: "2027-12-01",
		VigenciaTexto: "El presente convenio tendrá vigencia hasta el 31 de diciembre de 2027."}
	r, ok := campos(Check(in))[Vigencia]
	if !ok || r.Propuesta != "2027-12-31" {
		t.Fatalf("vigencia distinta de la cláusula debe marcarse: %+v", r)
	}
	in.FechaVigencia = "2027-12-31"
	if _, ok := campos(Check(in))[Vigencia]; ok {
		t.Fatalf("vigencia igual a la cláusula no se marca")
	}
}

func TestCheckVigenciaFechaDeInicioNoCuenta(t *testing.T) {
	in := Input{Texto: cierre, FechaFirma: "2025-11-25", FechaVigencia: "2028-01-01", VigenciaCalculada: true,
		VigenciaTexto: "tendrá una vigencia de tres años a partir del 1 de enero de 2025."}
	if _, ok := campos(Check(in))[Vigencia]; ok {
		t.Fatalf("una fecha de inicio no es la fecha de término")
	}
}

func TestCheckVigenciaCalculadaHeredaLaFirma(t *testing.T) {
	in := Input{Texto: cierre, FechaFirma: "2025-09-25", FechaVigencia: "2028-09-25", VigenciaCalculada: true,
		VigenciaTexto: "tendrá una vigencia de tres (3) años contados a partir de su firma."}
	r, ok := campos(Check(in))[Vigencia]
	if !ok || r.Propuesta != "" {
		t.Fatalf("la vigencia calculada con una firma por confirmar también se marca: %+v", r)
	}
}

func TestCheckVigenciaAntesDeLaFirma(t *testing.T) {
	in := Input{Texto: cierre, FechaFirma: "2025-11-25", FechaVigencia: "2024-11-25"}
	if _, ok := campos(Check(in))[Vigencia]; !ok {
		t.Fatalf("una vigencia anterior a la firma se marca")
	}
}

func TestCheckIndefinidaNoMarcaVigencia(t *testing.T) {
	in := Input{Texto: cierre, FechaFirma: "2025-09-25", VigenciaIndefinida: true,
		VigenciaTexto: "tendrá vigencia por tiempo indefinido."}
	if _, ok := campos(Check(in))[Vigencia]; ok {
		t.Fatalf("una vigencia indefinida no tiene fecha que confirmar")
	}
}
