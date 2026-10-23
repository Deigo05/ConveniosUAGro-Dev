package store

import "strings"

// EvaluateRisk calcula el semáforo de un expediente.
func EvaluateRisk(c ConventionSummary) Risk {
	r := Risk{Nivel: "bajo", Motivos: []string{}}
	add := func(p int, motivo string) {
		r.Puntaje += p
		r.Motivos = append(r.Motivos, motivo)
	}
	switch c.Estado {
	case "vencido":
		add(50, "Convenio vencido")
	case "por_vencer":
		add(30, "Vence en los próximos 90 días")
	case "sin_fecha":
		add(15, "Sin fecha de vigencia")
	}
	if strings.TrimSpace(c.Responsable) == "" {
		add(20, "Sin responsable asignado")
	}
	if strings.TrimSpace(c.Dependencia) == "" {
		add(15, "Sin contraparte capturada")
	}
	if len(c.Etiquetas) == 0 {
		add(10, "Sin etiquetas")
	}
	if c.EsEscaneado && c.TranscripcionEstado != "completa" {
		switch c.TranscripcionEstado {
		case "error":
			add(18, "PDF escaneado con transcripción incompleta")
		case "en_proceso":
			add(8, "PDF escaneado en transcripción")
		default:
			add(15, "PDF escaneado sin transcribir")
		}
	}
	if strings.TrimSpace(c.FechaFirma) == "" {
		add(10, "Sin fecha de firma")
	}
	if !c.VigenciaIndefinida && strings.TrimSpace(c.FechaVigencia) == "" {
		add(5, "Sin fecha de término")
	}
	if c.VigenciaIndefinida {
		// Una vigencia indefinida no es un error, pero conviene revisarla.
		if strings.TrimSpace(c.FechaFirma) == "" {
			add(5, "Vigencia indefinida sin fecha de firma")
		}
	}
	if r.Puntaje >= 50 {
		r.Nivel = "alto"
	} else if r.Puntaje >= 25 {
		r.Nivel = "medio"
	}
	return r
}
