package tui

// QuestionType determines how the ProfileModel renders and processes a question.
type QuestionType int

const (
	// SingleSelect means only one option can be chosen.
	SingleSelect QuestionType = iota
	// MultiSelect means any number of options can be toggled.
	MultiSelect
)

// OptionDef is a single selectable answer within a [QuestionDef].
type OptionDef struct {
	// Key is the value persisted in [config.FinancialProfile].
	Key string
	// LabelKey is the i18n message ID for the display label.
	LabelKey string
	// ShowTextField, when true, reveals a free-text input below the option list
	// so the user can type a qualifier (e.g. a country name).
	ShowTextField bool
}

// QuestionDef describes one step of the financial profile questionnaire.
type QuestionDef struct {
	// FieldKey matches the field name in [config.FinancialProfile] that this
	// question populates (used when building the final struct).
	FieldKey string
	// Type is [SingleSelect] or [MultiSelect].
	Type QuestionType
	// LabelKey is the i18n message ID for the question text.
	LabelKey string
	// Options is the ordered list of selectable answers.
	Options []OptionDef
	// HasOther, when true, adds an explicit "other" option that reveals a
	// free-text input for the user to elaborate.
	HasOther bool
	// OtherFieldKey is the [config.FinancialProfile] field that stores the
	// free-text value entered when HasOther is active.
	OtherFieldKey string
}

// Questions is the ordered list of the 10 financial profile questions shown
// during the setup wizard.
var Questions = []QuestionDef{
	{
		FieldKey: "LifeStage",
		Type:     SingleSelect,
		LabelKey: "profile.q1.label",
		Options: []OptionDef{
			{Key: "under35", LabelKey: "profile.q1.opt.under35"},
			{Key: "35to50", LabelKey: "profile.q1.opt.35to50"},
			{Key: "50to65", LabelKey: "profile.q1.opt.50to65"},
			{Key: "retired", LabelKey: "profile.q1.opt.retired"},
		},
	},
	{
		FieldKey: "IncomeStability",
		Type:     SingleSelect,
		LabelKey: "profile.q2.label",
		Options: []OptionDef{
			{Key: "very_stable", LabelKey: "profile.q2.opt.very_stable"},
			{Key: "stable", LabelKey: "profile.q2.opt.stable"},
			{Key: "variable", LabelKey: "profile.q2.opt.variable"},
			{Key: "irregular", LabelKey: "profile.q2.opt.irregular"},
		},
	},
	{
		FieldKey: "EmergencyFund",
		Type:     SingleSelect,
		LabelKey: "profile.q3.label",
		Options: []OptionDef{
			{Key: "yes_margin", LabelKey: "profile.q3.opt.yes_margin"},
			{Key: "yes_tight", LabelKey: "profile.q3.opt.yes_tight"},
			{Key: "no_working", LabelKey: "profile.q3.opt.no_working"},
			{Key: "no_priority", LabelKey: "profile.q3.opt.no_priority"},
		},
	},
	{
		FieldKey:      "InvestmentGoals",
		Type:          MultiSelect,
		LabelKey:      "profile.q4.label",
		HasOther:      true,
		OtherFieldKey: "InvestmentGoalsOther",
		Options: []OptionDef{
			{Key: "retirement", LabelKey: "profile.q4.opt.retirement"},
			{Key: "home_purchase", LabelKey: "profile.q4.opt.home_purchase"},
			{Key: "children_education", LabelKey: "profile.q4.opt.children_education"},
			{Key: "wealth_growth", LabelKey: "profile.q4.opt.wealth_growth"},
			{Key: "passive_income", LabelKey: "profile.q4.opt.passive_income"},
			{Key: "concrete_goal", LabelKey: "profile.q4.opt.concrete_goal"},
			{Key: "other", LabelKey: "profile.q4.opt.other"},
		},
	},
	{
		FieldKey: "TimeHorizon",
		Type:     SingleSelect,
		LabelKey: "profile.q5.label",
		Options: []OptionDef{
			{Key: "under_1y", LabelKey: "profile.q5.opt.under_1y"},
			{Key: "1_3y", LabelKey: "profile.q5.opt.1_3y"},
			{Key: "3_7y", LabelKey: "profile.q5.opt.3_7y"},
			{Key: "over_7y", LabelKey: "profile.q5.opt.over_7y"},
			{Key: "no_rush", LabelKey: "profile.q5.opt.no_rush"},
		},
	},
	{
		FieldKey: "LossScenario",
		Type:     SingleSelect,
		LabelKey: "profile.q6.label",
		Options: []OptionDef{
			{Key: "sell_all", LabelKey: "profile.q6.opt.sell_all"},
			{Key: "sell_some", LabelKey: "profile.q6.opt.sell_some"},
			{Key: "hold", LabelKey: "profile.q6.opt.hold"},
			{Key: "buy_more", LabelKey: "profile.q6.opt.buy_more"},
		},
	},
	{
		FieldKey: "MaxAcceptableLoss",
		Type:     SingleSelect,
		LabelKey: "profile.q7.label",
		Options: []OptionDef{
			{Key: "none", LabelKey: "profile.q7.opt.none"},
			{Key: "10pct", LabelKey: "profile.q7.opt.10pct"},
			{Key: "25pct", LabelKey: "profile.q7.opt.25pct"},
			{Key: "50pct", LabelKey: "profile.q7.opt.50pct"},
			{Key: "all", LabelKey: "profile.q7.opt.all"},
		},
	},
	{
		FieldKey: "FinancialExperience",
		Type:     MultiSelect,
		LabelKey: "profile.q8.label",
		Options: []OptionDef{
			{Key: "none", LabelKey: "profile.q8.opt.none"},
			{Key: "savings", LabelKey: "profile.q8.opt.savings"},
			{Key: "funds", LabelKey: "profile.q8.opt.funds"},
			{Key: "stocks_etfs", LabelKey: "profile.q8.opt.stocks_etfs"},
			{Key: "bonds", LabelKey: "profile.q8.opt.bonds"},
			{Key: "real_estate", LabelKey: "profile.q8.opt.real_estate"},
			{Key: "crypto", LabelKey: "profile.q8.opt.crypto"},
			{Key: "derivatives", LabelKey: "profile.q8.opt.derivatives"},
		},
	},
	{
		FieldKey: "InvestmentPriority",
		Type:     SingleSelect,
		LabelKey: "profile.q9.label",
		Options: []OptionDef{
			{Key: "safety", LabelKey: "profile.q9.opt.safety"},
			{Key: "liquidity", LabelKey: "profile.q9.opt.liquidity"},
			{Key: "returns", LabelKey: "profile.q9.opt.returns"},
			{Key: "impact", LabelKey: "profile.q9.opt.impact"},
		},
	},
	{
		FieldKey:      "Restrictions",
		Type:          MultiSelect,
		LabelKey:      "profile.q10.label",
		HasOther:      true,
		OtherFieldKey: "RestrictionsCountry",
		Options: []OptionDef{
			{Key: "no_tobacco_weapons", LabelKey: "profile.q10.opt.no_tobacco_weapons"},
			{Key: "no_crypto", LabelKey: "profile.q10.opt.no_crypto"},
			{Key: "country_only", LabelKey: "profile.q10.opt.country_only", ShowTextField: true},
			{Key: "no_leverage", LabelKey: "profile.q10.opt.no_leverage"},
			{Key: "none", LabelKey: "profile.q10.opt.none"},
			{Key: "other", LabelKey: "profile.q10.opt.other"},
		},
	},
}
