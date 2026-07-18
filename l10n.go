package main

// Localization helpers. The app supports English, Russian, and Uzbek.
// - In LIVE mode, withLang() tells the ChatGPT model which language to answer in.
// - In MOCK mode, tr() picks the right hard-coded string so the no-key demo
//   also runs fully in the chosen language.

func normLang(lang string) string {
	switch lang {
	case "ru", "uz", "en":
		return lang
	default:
		return "en"
	}
}

func langName(lang string) string {
	switch normLang(lang) {
	case "ru":
		return "Russian"
	case "uz":
		return "Uzbek"
	default:
		return "English"
	}
}

// tr returns the string for the active language.
func tr(lang, en, ru, uz string) string {
	switch normLang(lang) {
	case "ru":
		return ru
	case "uz":
		return uz
	default:
		return en
	}
}

// withLang appends a language directive to a system prompt (live ChatGPT mode).
// JSON keys and enum values stay in English so parsing is unaffected.
func withLang(base, lang string) string {
	return base + "\n\nIMPORTANT: Write ALL user-facing text — questions, overview, " +
		"decline message, assessment, feasibility, phase titles and summaries, milestone " +
		"titles, todo titles, and setup names/rationales — in " + langName(lang) + ". " +
		"Keep the JSON keys and enum values (frequency, priority, category) in English."
}
