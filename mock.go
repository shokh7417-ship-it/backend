package main

import (
	"regexp"
	"strings"
)

// This file is the mock brain: deterministic stand-ins for each AI stage so the
// whole product demos end-to-end with no OPENAI_API_KEY. With a key set, the
// pipeline uses the real ChatGPT API instead and none of this runs.

var (
	reInt      = regexp.MustCompile(`\d+`)
	reISODate  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	weekdayHit = map[string]string{
		"mon": "Mon", "tue": "Tue", "wed": "Wed", "thu": "Thu",
		"fri": "Fri", "sat": "Sat", "sun": "Sun",
	}
)

type skillDef struct {
	name    string
	pivotal string
	options []string
}

// known concrete skills → canonical name (+ optional pivotal fork).
var knownSkills = []struct {
	keys []string
	def  skillDef
}{
	{[]string{"ielts"}, skillDef{"IELTS", "Academic vs General", []string{"Academic", "General"}}},
	{[]string{"toefl"}, skillDef{"TOEFL", "", nil}},
	{[]string{"english", "английск", "ingliz"}, skillDef{"English", "", nil}},
	{[]string{"guitar", "гитар", "gitara"}, skillDef{"Guitar", "Acoustic vs Electric", []string{"Acoustic", "Electric"}}},
	{[]string{"piano", "пианино", "pianino"}, skillDef{"Piano", "", nil}},
	{[]string{"python", "programming", "coding", "code", "программир", "dasturlash", "kod"}, skillDef{"Programming", "", nil}},
	{[]string{"data analytics", "data", "аналитик", "data tahlil", "tahlil"}, skillDef{"Data analytics", "", nil}},
	{[]string{"valorant"}, skillDef{"Valorant", "", nil}},
	{[]string{"chess", "шахмат", "shaxmat"}, skillDef{"Chess", "", nil}},
	{[]string{"driving", "driver", "вожден", "haydovchi"}, skillDef{"Driving test", "", nil}},
	{[]string{"photography", "photo", "фотограф", "fotograf"}, skillDef{"Photography", "", nil}},
	{[]string{"drawing", "draw", "рисова", "chizish"}, skillDef{"Drawing", "", nil}},
	{[]string{"public speaking", "speaking", "ораторск", "notiqlik"}, skillDef{"Public speaking", "", nil}},
}

// vague goals that must pass through the disambiguation gate first.
var vagueGoals = []struct {
	id   string
	keys []string
}{
	{"gaming", []string{"gamer", "gaming", "games", "геймер", "гейм", "видеоигр", "geymer"}},
	{"fitness", []string{"get fit", "fitness", "fit", "healthy", "in shape", "фитнес", "спорт", "fitnes", "sport"}},
	{"business", []string{"business", "бизнес", "biznes"}},
}

// vagueQ returns the localized narrowing question + options for a vague goal.
func vagueQ(id, lang string) (string, []string) {
	switch id {
	case "gaming":
		return tr(lang,
				"“Gamer” can mean a few different journeys — which one fits you?",
				"«Геймер» — это несколько разных путей. Какой вам ближе?",
				"«Geymer» bir necha xil yo'lni anglatadi — qaysi biri sizga mos?"),
			[]string{
				tr(lang, "Get good at a game", "Прокачаться в игре", "O'yinda zo'r bo'lish"),
				tr(lang, "Go competitive", "Киберспорт", "Kibersportga kirish"),
				tr(lang, "Stream / build an audience", "Стриминг / аудитория", "Striming / auditoriya"),
				tr(lang, "Make games", "Разработка игр", "O'yin yaratish"),
			}
	case "fitness":
		return tr(lang,
				"Fitness covers a lot — what's your main aim?",
				"Фитнес — это широко. Какая у вас главная цель?",
				"Fitnes keng tushuncha — asosiy maqsadingiz nima?"),
			[]string{
				tr(lang, "Lose weight", "Похудеть", "Vazn tashlash"),
				tr(lang, "Build muscle", "Набрать мышцы", "Mushak yig'ish"),
				tr(lang, "Run / endurance", "Бег / выносливость", "Yugurish / chidamlilik"),
				tr(lang, "General health", "Общее здоровье", "Umumiy salomatlik"),
			}
	case "business":
		return tr(lang,
				"Business is broad — where do you want to start?",
				"Бизнес — это широко. С чего хотите начать?",
				"Biznes keng — nimadan boshlamoqchisiz?"),
			[]string{
				tr(lang, "Start a small business", "Открыть малый бизнес", "Kichik biznes ochish"),
				tr(lang, "Learn marketing", "Маркетинг", "Marketingni o'rganish"),
				tr(lang, "Learn finance", "Финансы", "Moliyani o'rganish"),
				tr(lang, "Freelance / side income", "Фриланс / подработка", "Frilans / qo'shimcha daromad"),
			}
	}
	return "", nil
}

var learningIntent = []string{"learn", "study", "prepare", "improve", "master", "practice", "how to", "become", "be a", "get good", "get better", "start"}
var offTopicStarts = []string{"what is", "what's the", "who is", "who's", "when is", "where is", "why is", "weather", "news", "tell me a joke", "translate", "write me", "capital of"}

func mockUnderstand(message, lang string) understandResult {
	lower := strings.ToLower(message)

	// vague first — the disambiguation gate
	for _, v := range vagueGoals {
		if containsAny(lower, v.keys) {
			q, opts := vagueQ(v.id, lang)
			return understandResult{
				InScope:                true,
				Skill:                  capitalize(firstMatch(lower, v.keys)),
				NeedsDisambiguation:    true,
				DisambiguationQuestion: q,
				Options:                opts,
			}
		}
	}

	// known concrete skill
	for _, ks := range knownSkills {
		if containsAny(lower, ks.keys) {
			return understandResult{
				InScope:       true,
				Skill:         ks.def.name,
				PivotalChoice: ks.def.pivotal,
				Options:       ks.def.options,
				Overview:      overviewFor(ks.def.name, lang),
			}
		}
	}

	hasIntent := containsAny(lower, learningIntent)
	looksOffTopic := containsAny(lower, offTopicStarts)
	if looksOffTopic && !hasIntent {
		return understandResult{
			InScope: false,
			Decline: tr(lang,
				"I'm your learning coach, so I can't help with that — but tell me a skill you'd like to learn and we'll build a plan.",
				"Я ваш коуч по обучению и с этим помочь не смогу — но назовите навык, который хотите освоить, и мы составим план.",
				"Men sizning o'quv murabbiyingizman, bunga yordam bera olmayman — lekin o'rganmoqchi bo'lgan ko'nikmani ayting, biz reja tuzamiz."),
		}
	}

	// otherwise treat it as a (novel) skill the framework can still handle
	skill := strings.TrimSpace(message)
	if len(skill) > 40 || skill == "" {
		skill = tr(lang, "your goal", "вашей цели", "maqsadingiz")
	}
	return understandResult{
		InScope: true,
		Skill:   skill,
		Overview: tr(lang,
			"We'll treat this as a skill and build a personalized plan around it.",
			"Будем считать это навыком и построим вокруг него персональный план.",
			"Buni ko'nikma sifatida olib, atrofida shaxsiy reja tuzamiz."),
	}
}

func overviewFor(skill, lang string) string {
	switch skill {
	case "IELTS":
		return tr(lang,
			"IELTS has four sections — Listening, Reading, Writing, Speaking — scored 0–9. Academic and General are different tests.",
			"IELTS состоит из четырёх частей — Listening, Reading, Writing, Speaking — по шкале 0–9. Academic и General — это разные тесты.",
			"IELTS to'rt bo'limdan iborat — Listening, Reading, Writing, Speaking — 0–9 ball bilan. Academic va General — bu ikki xil imtihon.")
	case "Guitar":
		return tr(lang,
			"Guitar splits into acoustic and electric, and early progress is mostly chords, rhythm, and finger strength.",
			"Гитара делится на акустическую и электро, и на старте главное — аккорды, ритм и сила пальцев.",
			"Gitara akustik va elektrga bo'linadi, boshida asosiysi — akkordlar, ritm va barmoq kuchi.")
	case "Programming":
		return tr(lang,
			"Programming is built in layers — syntax, problem-solving, then real projects — and daily reps beat weekend cramming.",
			"Программирование строится слоями — синтаксис, решение задач, затем реальные проекты — и ежедневная практика лучше рывков по выходным.",
			"Dasturlash qatlamma-qatlam quriladi — sintaksis, masala yechish, so'ng real loyihalar — kunlik mashq dam olish kunidagi tiqishtirishdan afzal.")
	default:
		return tr(lang,
			"Here's roughly what this skill involves; a few questions will let me tailor the plan to you.",
			"Вот примерно, что включает этот навык; пара вопросов — и я подстрою план под вас.",
			"Bu ko'nikma taxminan nimani o'z ichiga oladi; bir necha savol bilan rejani sizga moslayman.")
	}
}

// mockIntake returns the next question given how many have already been asked,
// merging the newest user answer into the framework answers.
func mockIntake(sess *IntakeSession, latest string) intakeResult {
	res := intakeResult{Answers: map[string]string{}}
	ingestAnswer(sess, latest)

	type q struct {
		key     string
		text    string
		options []string
	}
	lang := sess.Lang
	queue := []q{}
	if sess.PivotalChoice != "" && sess.Answers.PivotalChoice == "" {
		queue = append(queue, q{"pivotal",
			tr(lang, "Quick fork: "+sess.PivotalChoice+"?", "Быстрый выбор: "+sess.PivotalChoice+"?", "Tezkor tanlov: "+sess.PivotalChoice+"?"),
			pivotalOptions(sess)})
	}
	queue = append(queue,
		q{"currentLevel", tr(lang,
			"What's your current level — where are you starting from?",
			"Какой у вас сейчас уровень — с чего начинаете?",
			"Hozirgi darajangiz qanday — qayerdan boshlayapsiz?"), nil},
		q{"target", tr(lang,
			"What exactly do you want to achieve (your target)?",
			"Чего именно вы хотите достичь (ваша цель)?",
			"Aynan nimaga erishmoqchisiz (maqsadingiz)?"), nil},
		q{"deadline", tr(lang,
			"Is there a deadline or test date? (a date, or “no deadline”)",
			"Есть ли срок или дата экзамена? (дата или «без срока»)",
			"Muddat yoki imtihon sanasi bormi? (sana yoki «muddatsiz»)"),
			[]string{tr(lang, "No deadline", "Без срока", "Muddatsiz")}},
		q{"time", tr(lang,
			"How many hours a week can you commit, and which days?",
			"Сколько часов в неделю вы можете уделять и в какие дни?",
			"Haftasiga necha soat ajrata olasiz va qaysi kunlari?"), nil},
		q{"budget", tr(lang,
			"What's your budget for materials or gear?",
			"Какой у вас бюджет на материалы или оборудование?",
			"Materiallar yoki jihozlar uchun byudjetingiz qancha?"),
			[]string{
				tr(lang, "Free / minimal", "Бесплатно / минимум", "Bepul / minimal"),
				tr(lang, "Some budget", "Есть бюджет", "Byudjet bor"),
				tr(lang, "Flexible", "Гибко", "Moslashuvchan"),
			}},
	)

	for _, item := range queue {
		if !answered(sess, item.key) {
			res.NextQuestion = item.text
			res.Options = item.options
			return res
		}
	}
	res.Done = true
	return res
}

func pivotalOptions(sess *IntakeSession) []string {
	for _, ks := range knownSkills {
		if ks.def.name == sess.Skill {
			return ks.def.options
		}
	}
	return nil
}

func answered(sess *IntakeSession, key string) bool {
	a := sess.Answers
	switch key {
	case "pivotal":
		return a.PivotalChoice != ""
	case "currentLevel":
		return a.CurrentLevel != ""
	case "target":
		return a.Target != ""
	case "deadline":
		return a.Deadline != "" || sess.AnswerBag["deadline_asked"] == "yes"
	case "time":
		return a.HoursPerWeek > 0
	case "budget":
		return a.Budget != ""
	}
	return false
}

// ingestAnswer maps the newest free-text reply onto whichever category is next.
func ingestAnswer(sess *IntakeSession, latest string) {
	if latest == "" {
		return
	}
	if sess.AnswerBag == nil {
		sess.AnswerBag = map[string]string{}
	}
	a := &sess.Answers
	lower := strings.ToLower(latest)

	// pivotal
	if sess.PivotalChoice != "" && a.PivotalChoice == "" {
		for _, opt := range pivotalOptions(sess) {
			if strings.Contains(lower, strings.ToLower(opt)) {
				a.PivotalChoice = opt
				return
			}
		}
	}
	if a.CurrentLevel == "" {
		a.CurrentLevel = latest
		return
	}
	if a.Target == "" {
		a.Target = latest
		return
	}
	if a.Deadline == "" && sess.AnswerBag["deadline_asked"] != "yes" {
		sess.AnswerBag["deadline_asked"] = "yes"
		if d := reISODate.FindString(latest); d != "" {
			a.Deadline = d
		}
		return
	}
	if a.HoursPerWeek == 0 {
		if m := reInt.FindString(latest); m != "" {
			a.HoursPerWeek = clamp(atoi(m), 1, 60)
		} else {
			a.HoursPerWeek = 6
		}
		a.Days = detectDays(lower)
		return
	}
	if a.Budget == "" {
		a.Budget = latest
		return
	}
}

func detectDays(lower string) []string {
	var out []string
	seen := map[string]bool{}
	for k, v := range weekdayHit {
		if strings.Contains(lower, k) && !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out
}

// mockPlan builds a believable, schedulable plan without any model call.
func mockPlan(sess *IntakeSession) planAI {
	lang := sess.Lang
	weeks := 12
	if d, ok := parseDate(sess.Answers.Deadline); ok {
		w := int(d.Sub(today()).Hours()/24/7) + 1
		weeks = clamp(w, 2, 24)
	}
	skill := sess.Skill
	target := firstNonEmpty(sess.Answers.Target, tr(lang, "your goal", "вашей цели", "maqsadingiz"))
	level := firstNonEmpty(sess.Answers.CurrentLevel, tr(lang, "your current level", "вашего уровня", "hozirgi darajangiz"))

	p := planAI{
		Assessment: tr(lang,
			"Going from "+level+" to "+target+" in about "+itoa(weeks)+" weeks is realistic with steady, focused practice. We'll front-load fundamentals, then drill your weak spots, then rehearse under real conditions.",
			"Переход от «"+level+"» к «"+target+"» примерно за "+itoa(weeks)+" недель реалистичен при регулярной, сфокусированной практике. Сначала укрепим основы, затем проработаем слабые места, затем — репетиции в реальных условиях.",
			"«"+level+"» dan «"+target+"» ga taxminan "+itoa(weeks)+" hafta ichida yetish — muntazam, izchil mashq bilan real. Avval asoslarni mustahkamlaymiz, so'ng zaif tomonlarni ishlaymiz, keyin real sharoitda mashq qilamiz."),
		Feasibility: tr(lang,
			"Feasible at your pace if you protect the weekly hours. If you fall behind, the schedule rolls forward automatically and the finish date shifts.",
			"Достижимо в вашем темпе, если беречь недельные часы. Если отстанете, расписание автоматически сдвигается вперёд, и дата финиша меняется.",
			"Haftalik soatlarni saqlasangiz, sur'atingizda erishsa bo'ladi. Orqada qolsangiz, jadval avtomatik oldinga suriladi va tugash sanasi o'zgaradi."),
		WeeksTotal: weeks,
		Phases: []phaseAI{
			{"fundamentals",
				tr(lang, "Fundamentals & diagnostic", "Основы и диагностика", "Asoslar va diagnostika"),
				tr(lang, "Establish a baseline and cover the basics.", "Определить стартовый уровень и закрыть основы.", "Boshlang'ich darajani aniqlab, asoslarni yopish."), 1, 2},
			{"drills",
				tr(lang, "Focused drills", "Целевые тренировки", "Yo'naltirilgan mashqlar"),
				tr(lang, "Target the highest-impact weaknesses.", "Проработать самые важные слабые места.", "Eng ta'sirli zaif tomonlarni ishlash."), 3, weeks - 3},
			{"rehearsal",
				tr(lang, "Full rehearsal", "Полная репетиция", "To'liq mashq"),
				tr(lang, "Simulate the real thing under time pressure.", "Смоделировать реальные условия на время.", "Real sharoitni vaqt bosimida modellashtirish."), weeks - 2, weeks},
		},
		Milestones: []milestoneAI{
			{tr(lang, "Complete a diagnostic and know your weak areas", "Пройти диагностику и узнать слабые места", "Diagnostikadan o'tib, zaif tomonlarni bilish"), "fundamentals", 2},
			{tr(lang, "Hit a solid mid-point checkpoint on "+skill, "Достичь уверенной контрольной точки по «"+skill+"»", "«"+skill+"» bo'yicha ishonchli oraliq nuqtaga yetish"), "drills", weeks / 2},
			{tr(lang, "Pass a full practice run at target level", "Пройти полный пробный прогон на целевом уровне", "Maqsad darajasida to'liq sinov mashqidan o'tish"), "rehearsal", weeks},
		},
		Todos:      mockTodos(skill, lang),
		SetupItems: mockSetup(skill, lang),
	}
	return p
}

func mockTodos(skill, lang string) []todoAI {
	if strings.EqualFold(skill, "IELTS") {
		return []todoAI{
			{tr(lang, "Diagnostic full practice test + score yourself", "Диагностический пробный тест + самооценка", "Diagnostik to'liq sinov testi + o'zingizni baholang"), 120, "once", "high", "fundamentals", nil, "cambridge-ielts"},
			{tr(lang, "Reading: one Cambridge test + review mistakes", "Reading: один тест Cambridge + разбор ошибок", "Reading: bitta Cambridge testi + xatolarni tahlil"), 60, "twice_weekly", "high", "drills", nil, "cambridge-ielts"},
			{tr(lang, "Writing Task 2 essay + self-check against band descriptors", "Writing Task 2 эссе + самопроверка по критериям", "Writing Task 2 esse + band mezonlari bo'yicha o'z-o'zini tekshirish"), 60, "twice_weekly", "high", "drills", nil, "band-descriptors"},
			{tr(lang, "Speaking practice out loud (record & review)", "Speaking вслух (запись и разбор)", "Speaking ovoz chiqarib (yozib olib, tahlil qilish)"), 30, "thrice_weekly", "medium", "drills", nil, ""},
			{tr(lang, "Listening section under timed conditions", "Listening на время", "Listening bo'limi vaqt bilan"), 45, "weekly", "medium", "drills", nil, "cambridge-ielts"},
			{tr(lang, "Full timed mock test", "Полный пробный экзамен на время", "To'liq vaqtli sinov imtihoni"), 165, "weekly", "high", "rehearsal", nil, "cambridge-ielts"},
		}
	}
	return []todoAI{
		{tr(lang, "Diagnostic: assess where you stand in "+skill, "Диагностика: оценить ваш уровень в «"+skill+"»", "Diagnostika: «"+skill+"» bo'yicha darajangizni baholash"), 60, "once", "high", "fundamentals", nil, ""},
		{tr(lang, "Core practice session on fundamentals", "Базовая тренировка по основам", "Asoslar bo'yicha asosiy mashg'ulot"), 45, "thrice_weekly", "high", "drills", nil, ""},
		{tr(lang, "Targeted drill on your weakest area", "Целевая тренировка слабого места", "Eng zaif tomon bo'yicha yo'naltirilgan mashq"), 45, "twice_weekly", "high", "drills", nil, ""},
		{tr(lang, "Review progress & reflect on what's working", "Обзор прогресса и что работает", "Taraqqiyotni ko'rib chiqish va nima ishlayotganini baholash"), 30, "weekly", "medium", "drills", nil, ""},
		{tr(lang, "Full rehearsal at target difficulty", "Полная репетиция на целевой сложности", "Maqsad darajasidagi to'liq mashq"), 90, "weekly", "high", "rehearsal", nil, ""},
	}
}

func mockSetup(skill, lang string) []setupAI {
	if strings.EqualFold(skill, "IELTS") {
		return []setupAI{
			{tr(lang, "Official Cambridge IELTS practice books", "Официальные сборники Cambridge IELTS", "Rasmiy Cambridge IELTS mashq kitoblari"), "materials", "high", "$0-25", false,
				tr(lang, "The single best-value resource; older editions and library copies are near-free.", "Лучший по соотношению цена/польза; старые издания и библиотека — почти бесплатно.", "Eng foydali resurs; eski nashrlar va kutubxona nusxalari deyarli bepul.")},
			{tr(lang, "Decent headphones for Listening practice", "Нормальные наушники для Listening", "Listening uchun yaxshi quloqchin"), "gear", "medium", "$15-30", false,
				tr(lang, "Clear audio matters for the Listening section; you may already own a pair.", "Чистый звук важен для Listening; возможно, у вас уже есть.", "Listening uchun toza ovoz muhim; ehtimol sizda bor.")},
			{tr(lang, "Notebook + timer (phone works)", "Блокнот + таймер (подойдёт телефон)", "Daftar + taymer (telefon ham bo'ladi)"), "gear", "low", "$0", false,
				tr(lang, "Timed practice is free — use what you have.", "Практика на время бесплатна — используйте то, что есть.", "Vaqtli mashq bepul — bor narsangizdan foydalaning.")},
		}
	}
	return []setupAI{
		{tr(lang, "Free/low-cost starter materials for "+skill, "Бесплатные/недорогие стартовые материалы по «"+skill+"»", "«"+skill+"» uchun bepul/arzon boshlang'ich materiallar"), "materials", "high", "$0-20", false,
			tr(lang, "Start with free resources; upgrade only once you hit a real limit.", "Начните с бесплатных ресурсов; улучшайте только при реальном упоре в потолок.", "Bepul resurslardan boshlang; faqat haqiqiy chegaraga yetganda yangilang.")},
		{tr(lang, "Basic practice tools you likely already own", "Базовые инструменты, которые у вас, вероятно, уже есть", "Sizda allaqachon bor bo'lishi mumkin bo'lgan asosiy vositalar"), "gear", "low", "$0", false,
			tr(lang, "Don't buy anything yet — settings and free tools go a long way.", "Пока ничего не покупайте — настроек и бесплатных инструментов достаточно.", "Hozircha hech narsa sotib olmang — sozlamalar va bepul vositalar yetarli.")},
	}
}

// ---- tiny helpers ----

func containsAny(s string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func firstMatch(s string, keys []string) string {
	for _, k := range keys {
		if strings.Contains(s, k) {
			return k
		}
	}
	return ""
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
