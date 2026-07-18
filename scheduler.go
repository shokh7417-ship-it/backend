package main

import (
	"fmt"
	"strings"
	"time"
)

// Scheduler places sized todos onto real dates and rolls missed days forward.
// This is deliberately plain, deterministic code — NOT the model's job.
type Scheduler struct {
	store *Store
}

func newScheduler(store *Store) *Scheduler { return &Scheduler{store: store} }

const (
	maxSessionsPerDay = 2
	maxTotalSessions  = 60
	dayStartMinute    = 18 * 60 // 18:00
	slotGapMinute     = 15
)

var weekdayName = map[time.Weekday]string{
	time.Sunday: "Sun", time.Monday: "Mon", time.Tuesday: "Tue",
	time.Wednesday: "Wed", time.Thursday: "Thu", time.Friday: "Fri", time.Saturday: "Sat",
}

func availableWeekdays(days []string) map[time.Weekday]bool {
	set := map[time.Weekday]bool{}
	for _, d := range days {
		key := strings.ToLower(d)
		if len(key) > 3 {
			key = key[:3]
		}
		for wd, name := range weekdayName {
			if strings.ToLower(name) == key {
				set[wd] = true
			}
		}
	}
	if len(set) == 0 { // default Mon–Fri
		set[time.Monday], set[time.Tuesday], set[time.Wednesday], set[time.Thursday], set[time.Friday] = true, true, true, true, true
	}
	return set
}

func occurrencesFor(freq string, weeks int) int {
	switch freq {
	case "once":
		return 1
	case "weekly":
		return weeks
	case "twice_weekly":
		return 2 * weeks
	case "thrice_weekly":
		return 3 * weeks
	case "daily":
		return 5 * weeks
	default:
		return weeks
	}
}

// Schedule (re)places all of a plan's todos and returns proposed events.
// It also sets the plan's StartDate / FinishDate / OriginalFinishDate.
func (sc *Scheduler) Schedule(plan *Plan) []*CalendarEvent {
	days := availableWeekdays(plan.Days)
	start := today().AddDate(0, 0, 1)

	// Expand todos into individual sessions, honoring frequency but capping the
	// occurrences so an early-phase todo doesn't run the full plan length.
	type session struct{ todo *Todo }
	var sessions []session
	for i := range plan.Todos {
		t := &plan.Todos[i]
		if t.Status == "done" {
			continue
		}
		n := clamp(occurrencesFor(t.Frequency, plan.WeeksTotal), 1, 12)
		for k := 0; k < n; k++ {
			sessions = append(sessions, session{t})
		}
	}
	if len(sessions) > maxTotalSessions {
		sessions = sessions[:maxTotalSessions]
	}

	var events []*CalendarEvent
	d := start
	idx, guard := 0, 0
	for idx < len(sessions) && guard < 1000 {
		guard++
		if days[d.Weekday()] {
			startMin := dayStartMinute
			for c := 0; c < maxSessionsPerDay && idx < len(sessions); c++ {
				t := sessions[idx].todo
				events = append(events, &CalendarEvent{
					ID:           newID("evt"),
					UserID:       plan.UserID,
					PlanID:       plan.ID,
					TodoID:       t.ID,
					Title:        t.Title,
					Date:         dateStr(d),
					StartTime:    fmt.Sprintf("%02d:%02d", startMin/60, startMin%60),
					DurationMin:  t.DurationMin,
					ReminderMin:  30,
					Status:       "proposed",
					ExportTarget: "web_calendar",
				})
				startMin += t.DurationMin + slotGapMinute
				idx++
			}
		}
		d = d.AddDate(0, 0, 1)
	}

	plan.StartDate = dateStr(start)
	if len(events) > 0 {
		plan.FinishDate = events[len(events)-1].Date
	} else {
		plan.FinishDate = plan.StartDate
	}
	if plan.OriginalFinishDate == "" {
		plan.OriginalFinishDate = plan.FinishDate
	}
	return events
}

// RolloverSummary reports what a rollover run changed.
type RolloverSummary struct {
	PlanID       string `json:"planId"`
	Moved        int    `json:"moved"`
	OldFinish    string `json:"oldFinish"`
	NewFinish    string `json:"newFinish"`
	FinishShifts int    `json:"finishShiftDays"`
	Message      string `json:"message"`
}

// Rollover moves every past, still-pending session forward to the next available
// day and recomputes the finish date — the plan "procrastinates" with the user.
func (sc *Scheduler) Rollover(userID string, ref time.Time) []RolloverSummary {
	var summaries []RolloverSummary
	refStr := dateStr(ref)

	for _, plan := range sc.store.PlansByUser(userID) {
		events := sc.store.EventsForPlan(plan.ID)
		if len(events) == 0 {
			continue
		}
		todoStatus := map[string]string{}
		for _, t := range plan.Todos {
			todoStatus[t.ID] = t.Status
		}

		days := availableWeekdays(plan.Days)
		perDay := map[string]int{}
		var missed []*CalendarEvent

		// Keep future/done events in place; count their load per day.
		for _, ev := range events {
			isPast := ev.Date < refStr
			pending := ev.Status != "done" && todoStatus[ev.TodoID] != "done"
			if isPast && pending {
				missed = append(missed, ev)
			} else {
				perDay[ev.Date]++
			}
		}
		if len(missed) == 0 {
			continue
		}

		oldFinish := plan.FinishDate

		// Re-place each missed session on the next available day with free capacity.
		cursor := ref
		place := func() string {
			for guard := 0; guard < 1000; guard++ {
				ds := dateStr(cursor)
				if days[cursor.Weekday()] && perDay[ds] < maxSessionsPerDay {
					perDay[ds]++
					return ds
				}
				cursor = cursor.AddDate(0, 0, 1)
			}
			return dateStr(cursor)
		}
		for _, ev := range missed {
			newDate := place()
			ev.Date = newDate
			ev.Status = "scheduled"
			sc.store.AddProgress(&ProgressLog{
				ID: newID("log"), UserID: userID, PlanID: plan.ID, TodoID: ev.TodoID,
				Event: "rolled_over", At: time.Now(),
				Note: "missed session rolled forward to " + newDate,
			})
		}
		sc.store.SaveEvents(events)

		// Recompute finish date from the latest event.
		newFinish := plan.StartDate
		for _, ev := range events {
			if ev.Date > newFinish {
				newFinish = ev.Date
			}
		}
		plan.FinishDate = newFinish
		sc.store.SavePlan(plan)

		shift := 0
		if o, ok := parseDate(oldFinish); ok {
			if n, ok2 := parseDate(newFinish); ok2 {
				shift = int(n.Sub(o).Hours() / 24)
			}
		}
		msg := fmt.Sprintf("%d session(s) rolled forward.", len(missed))
		if shift > 0 {
			msg += fmt.Sprintf(" Finish moved from %s → %s (+%d days).", oldFinish, newFinish, shift)
		}
		summaries = append(summaries, RolloverSummary{
			PlanID: plan.ID, Moved: len(missed), OldFinish: oldFinish,
			NewFinish: newFinish, FinishShifts: shift, Message: msg,
		})
	}
	return summaries
}

// ICS renders a plan's events as an iCalendar file (used by the .ics export and
// as the interchange format for the web calendar).
func (sc *Scheduler) ICS(plan *Plan, events []*CalendarEvent) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//start.ai//web//EN\r\nCALSCALE:GREGORIAN\r\n")
	for _, ev := range events {
		d, ok := parseDate(ev.Date)
		if !ok {
			continue
		}
		hh, mm := 18, 0
		if len(ev.StartTime) == 5 {
			hh = atoi(ev.StartTime[:2])
			mm = atoi(ev.StartTime[3:])
		}
		startT := time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, time.Local)
		endT := startT.Add(time.Duration(ev.DurationMin) * time.Minute)
		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString("UID:" + ev.ID + "@start.ai\r\n")
		b.WriteString("DTSTART:" + startT.Format("20060102T150405") + "\r\n")
		b.WriteString("DTEND:" + endT.Format("20060102T150405") + "\r\n")
		b.WriteString("SUMMARY:" + icsEscape(ev.Title) + "\r\n")
		b.WriteString("DESCRIPTION:" + icsEscape("start.ai — "+plan.Skill) + "\r\n")
		if ev.ReminderMin > 0 {
			b.WriteString("BEGIN:VALARM\r\nTRIGGER:-PT" + itoa(ev.ReminderMin) + "M\r\nACTION:DISPLAY\r\nDESCRIPTION:Reminder\r\nEND:VALARM\r\n")
		}
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func icsEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
