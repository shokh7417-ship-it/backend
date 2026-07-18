package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type API struct {
	cfg   Config
	store *Store
	gw    *Gateway
	pipe  *Pipeline
	sched *Scheduler
}

func newAPI(cfg Config, store *Store, gw *Gateway, pipe *Pipeline, sched *Scheduler) *API {
	return &API{cfg: cfg, store: store, gw: gw, pipe: pipe, sched: sched}
}

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/session", a.handleSession)
	mux.HandleFunc("POST /api/chat", a.handleChat)
	mux.HandleFunc("GET /api/plan/{id}", a.handlePlan)
	mux.HandleFunc("GET /api/plan/{id}/ics", a.handleICS)
	mux.HandleFunc("POST /api/schedule", a.handleSchedule)
	mux.HandleFunc("POST /api/schedule/confirm", a.handleConfirm)
	mux.HandleFunc("GET /api/calendar", a.handleCalendar)
	mux.HandleFunc("POST /api/todo/complete", a.handleComplete)
	mux.HandleFunc("POST /api/rollover", a.handleRollover)
	mux.HandleFunc("GET /api/meter", a.handleMeter)

	// Optionally serve the static frontend so `go run .` gives a one-command app.
	handler := a.withCORS(mux)
	if a.cfg.FrontendDir != "" {
		if fi, err := staticStat(a.cfg.FrontendDir); err == nil && fi.IsDir() {
			fs := http.FileServer(http.Dir(a.cfg.FrontendDir))
			root := http.NewServeMux()
			root.Handle("/api/", handler)
			root.Handle("/healthz", handler)
			root.Handle("/", fs)
			return a.withCORS(root)
		}
	}
	return handler
}

func (a *API) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", a.cfg.CORSOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- handlers ----

type sessionReq struct {
	UserID   string `json:"userId"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	Lang     string `json:"lang"`
}

func (a *API) handleSession(w http.ResponseWriter, r *http.Request) {
	var req sessionReq
	_ = readJSON(r, &req)

	userID := firstNonEmpty(req.UserID, r.Header.Get("X-User-Id"))
	var user *User
	if userID != "" {
		if u, ok := a.store.GetUser(userID); ok {
			user = u
		}
	}
	if user == nil {
		user = &User{ID: newID("user"), Name: req.Name, Timezone: firstNonEmpty(req.Timezone, "Asia/Tashkent"), CreatedAt: time.Now()}
		a.store.SaveUser(user)
	}

	lang := normLang(req.Lang)
	sess := &IntakeSession{
		ID: newID("sess"), UserID: user.ID, Stage: "scope_check", Lang: lang,
		AnswerBag: map[string]string{}, CreatedAt: time.Now(),
	}
	greeting := tr(lang,
		"Hi! I'm start.ai. Tell me something you want to learn — like “IELTS 7.0 by October” or “learn guitar” — and I'll build you a scheduled plan.",
		"Привет! Я start.ai. Скажите, что вы хотите освоить — например, «IELTS 7.0 к октябрю» или «научиться играть на гитаре» — и я составлю вам план с расписанием.",
		"Salom! Men start.ai. Nimani o'rganmoqchi ekaningizni ayting — masalan, «Oktyabrga IELTS 7.0» yoki «gitara o'rganish» — men sizga jadvalli reja tuzib beraman.")
	sess.Messages = append(sess.Messages, Message{Role: "assistant", Content: greeting, At: time.Now()})
	a.store.SaveSession(sess)

	writeJSON(w, 200, map[string]any{
		"userId": user.ID, "sessionId": sess.ID, "stage": sess.Stage, "assistant": greeting,
	})
}

type chatReq struct {
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	Lang      string `json:"lang"`
}

func (a *API) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	sess, ok := a.store.GetSession(req.SessionID)
	if !ok {
		writeErr(w, 404, "session not found")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeErr(w, 400, "empty message")
		return
	}
	if req.Lang != "" {
		sess.Lang = normLang(req.Lang) // let the user switch language mid-conversation
	}

	ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
	defer cancel()

	turn, err := a.pipe.HandleChat(ctx, sess, req.Message)
	if err != nil {
		writeErr(w, 502, "ai error: "+err.Error())
		return
	}
	writeJSON(w, 200, turn)
}

func (a *API) handlePlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plan, ok := a.store.GetPlan(id)
	if !ok {
		writeErr(w, 404, "plan not found")
		return
	}
	writeJSON(w, 200, plan)
}

type planRef struct {
	PlanID string `json:"planId"`
}

func (a *API) handleSchedule(w http.ResponseWriter, r *http.Request) {
	var req planRef
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	plan, ok := a.store.GetPlan(req.PlanID)
	if !ok {
		writeErr(w, 404, "plan not found")
		return
	}
	events := a.sched.Schedule(plan)
	a.store.ReplaceEventsForPlan(plan.ID, events)
	a.store.SavePlan(plan)
	writeJSON(w, 200, map[string]any{
		"planId": plan.ID, "startDate": plan.StartDate, "finishDate": plan.FinishDate,
		"events": events, "count": len(events),
	})
}

func (a *API) handleConfirm(w http.ResponseWriter, r *http.Request) {
	var req planRef
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	plan, ok := a.store.GetPlan(req.PlanID)
	if !ok {
		writeErr(w, 404, "plan not found")
		return
	}
	events := a.store.EventsForPlan(plan.ID)
	for _, ev := range events {
		if ev.Status == "proposed" {
			ev.Status = "scheduled"
		}
	}
	a.store.SaveEvents(events)
	writeJSON(w, 200, map[string]any{"planId": plan.ID, "confirmed": len(events), "finishDate": plan.FinishDate})
}

func (a *API) handleCalendar(w http.ResponseWriter, r *http.Request) {
	if planID := r.URL.Query().Get("planId"); planID != "" {
		writeJSON(w, 200, a.store.EventsForPlan(planID))
		return
	}
	userID := firstNonEmpty(r.URL.Query().Get("userId"), r.Header.Get("X-User-Id"))
	if userID == "" {
		writeErr(w, 400, "userId or planId required")
		return
	}
	writeJSON(w, 200, a.store.EventsForUser(userID))
}

type completeReq struct {
	PlanID string `json:"planId"`
	TodoID string `json:"todoId"`
}

func (a *API) handleComplete(w http.ResponseWriter, r *http.Request) {
	var req completeReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	plan, ok := a.store.GetPlan(req.PlanID)
	if !ok {
		writeErr(w, 404, "plan not found")
		return
	}
	found := false
	for i := range plan.Todos {
		if plan.Todos[i].ID == req.TodoID {
			now := time.Now()
			plan.Todos[i].Status = "done"
			plan.Todos[i].CompletedAt = &now
			found = true
		}
	}
	if !found {
		writeErr(w, 404, "todo not found")
		return
	}
	// Mark the soonest pending event for this todo as done.
	events := a.store.EventsForPlan(plan.ID)
	for _, ev := range events {
		if ev.TodoID == req.TodoID && ev.Status != "done" {
			ev.Status = "done"
			break
		}
	}
	a.store.SaveEvents(events)
	a.store.SavePlan(plan)
	a.store.AddProgress(&ProgressLog{ID: newID("log"), UserID: plan.UserID, PlanID: plan.ID, TodoID: req.TodoID, Event: "completed", At: time.Now()})
	writeJSON(w, 200, map[string]any{"ok": true})
}

type rolloverReq struct {
	UserID string `json:"userId"`
	AsOf   string `json:"asOf"` // optional YYYY-MM-DD to simulate "today" in a demo
}

func (a *API) handleRollover(w http.ResponseWriter, r *http.Request) {
	var req rolloverReq
	_ = readJSON(r, &req)
	userID := firstNonEmpty(req.UserID, r.Header.Get("X-User-Id"))
	if userID == "" {
		writeErr(w, 400, "userId required")
		return
	}
	ref := today()
	if d, ok := parseDate(req.AsOf); ok {
		ref = d
	}
	summaries := a.sched.Rollover(userID, ref)
	writeJSON(w, 200, map[string]any{"asOf": dateStr(ref), "results": summaries})
}

func (a *API) handleICS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plan, ok := a.store.GetPlan(id)
	if !ok {
		writeErr(w, 404, "plan not found")
		return
	}
	events := a.store.EventsForPlan(id)
	ics := a.sched.ICS(plan, events)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=start-ai-"+plan.Skill+".ics")
	_, _ = w.Write([]byte(ics))
}

func (a *API) handleMeter(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.gw.Snapshot())
}

// ---- small helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}
