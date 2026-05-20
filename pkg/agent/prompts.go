package agent

import (
	"strings"

	"auris/pkg/config"
	"auris/pkg/llm"
)

var systemPrompts = map[llm.TaskType]string{
	llm.TaskChat: `Eres Auris, un asistente de inteligencia artificial especializado en análisis financiero personal. Tu función es ayudar al usuario a entender los mercados financieros, analizar instrumentos de inversión, interpretar noticias económicas y explorar opciones de inversión acordes a su perfil.

Directrices de comportamiento:
- Proporciona información clara, objetiva y basada en datos cuando estén disponibles.
- Adapta tus respuestas al perfil financiero del usuario cuando sea relevante.
- Cuando uses herramientas de mercado, interpreta los datos obtenidos de forma útil y contextualizada.
- Sé conciso pero completo. Usa listas y tablas cuando mejoren la legibilidad.
- Responde siempre en el idioma en que el usuario se dirige a ti.

Aviso importante: No eres un asesor financiero regulado. Toda la información que proporcionas es de carácter educativo e informativo. Las decisiones de inversión son responsabilidad exclusiva del usuario.`,
}

// BuildSystemMessage returns the system message for the given task type,
// optionally enriched with the user's financial profile. Returns nil if no
// prompt is defined for that task.
func BuildSystemMessage(task llm.TaskType, profile *config.FinancialProfile) *llm.Message {
	base, ok := systemPrompts[task]
	if !ok {
		return nil
	}
	content := base
	if profile != nil {
		if formatted := formatProfile(profile); formatted != "" {
			content += "\n\n## Perfil financiero del usuario\n" + formatted
		}
	}
	return &llm.Message{Role: llm.RoleSystem, Content: content}
}

func formatProfile(p *config.FinancialProfile) string {
	var b strings.Builder
	write := func(label, value string) {
		if value != "" {
			b.WriteString("- ")
			b.WriteString(label)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteByte('\n')
		}
	}
	writeList := func(label string, values []string) {
		if len(values) > 0 {
			write(label, strings.Join(values, ", "))
		}
	}

	write("Etapa de vida", p.LifeStage)
	write("Estabilidad de ingresos", p.IncomeStability)
	write("Fondo de emergencia", p.EmergencyFund)
	writeList("Objetivos de inversión", p.InvestmentGoals)
	write("Horizonte temporal", p.TimeHorizon)
	write("Reacción ante pérdida del 25%", p.LossScenario)
	write("Pérdida máxima aceptable", p.MaxAcceptableLoss)
	writeList("Experiencia financiera", p.FinancialExperience)
	write("Prioridad de inversión", p.InvestmentPriority)
	writeList("Restricciones", p.Restrictions)
	if p.RestrictionsCountry != "" {
		write("País de restricción", p.RestrictionsCountry)
	}

	return b.String()
}
