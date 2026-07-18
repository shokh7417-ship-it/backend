package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Store is an in-memory data store with a JSON-file snapshot, so an MVP demo
// survives restarts with zero external dependencies. Swap for Postgres in prod.
type Store struct {
	mu       sync.RWMutex
	dir      string
	Users    map[string]*User
	Sessions map[string]*IntakeSession
	Goals    map[string]*Goal
	Plans    map[string]*Plan
	Events   map[string]*CalendarEvent
	Progress []*ProgressLog
}

type persistShape struct {
	Users    map[string]*User          `json:"users"`
	Sessions map[string]*IntakeSession `json:"sessions"`
	Goals    map[string]*Goal          `json:"goals"`
	Plans    map[string]*Plan          `json:"plans"`
	Events   map[string]*CalendarEvent `json:"events"`
	Progress []*ProgressLog            `json:"progress"`
}

func newStore(dir string) *Store {
	s := &Store{
		dir:      dir,
		Users:    map[string]*User{},
		Sessions: map[string]*IntakeSession{},
		Goals:    map[string]*Goal{},
		Plans:    map[string]*Plan{},
		Events:   map[string]*CalendarEvent{},
	}
	s.load()
	return s
}

func (s *Store) path() string { return filepath.Join(s.dir, "store.json") }

func (s *Store) load() {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return
	}
	var p persistShape
	if json.Unmarshal(data, &p) != nil {
		return
	}
	if p.Users != nil {
		s.Users = p.Users
	}
	if p.Sessions != nil {
		s.Sessions = p.Sessions
	}
	if p.Goals != nil {
		s.Goals = p.Goals
	}
	if p.Plans != nil {
		s.Plans = p.Plans
	}
	if p.Events != nil {
		s.Events = p.Events
	}
	s.Progress = p.Progress
}

// saveLocked writes a snapshot. Caller must hold s.mu.
func (s *Store) saveLocked() {
	_ = os.MkdirAll(s.dir, 0o755)
	p := persistShape{s.Users, s.Sessions, s.Goals, s.Plans, s.Events, s.Progress}
	if data, err := json.MarshalIndent(p, "", "  "); err == nil {
		_ = os.WriteFile(s.path(), data, 0o644)
	}
}

// ---- Users ----

func (s *Store) SaveUser(u *User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Users[u.ID] = u
	s.saveLocked()
}

func (s *Store) GetUser(id string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.Users[id]
	return u, ok
}

// ---- Sessions ----

func (s *Store) SaveSession(sess *IntakeSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.UpdatedAt = time.Now()
	s.Sessions[sess.ID] = sess
	s.saveLocked()
}

func (s *Store) GetSession(id string) (*IntakeSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.Sessions[id]
	return sess, ok
}

// ---- Goals ----

func (s *Store) SaveGoal(g *Goal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Goals[g.ID] = g
	s.saveLocked()
}

// ---- Plans ----

func (s *Store) SavePlan(p *Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.UpdatedAt = time.Now()
	s.Plans[p.ID] = p
	s.saveLocked()
}

func (s *Store) GetPlan(id string) (*Plan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.Plans[id]
	return p, ok
}

func (s *Store) PlansByUser(userID string) []*Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Plan
	for _, p := range s.Plans {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// ---- Events ----

func (s *Store) SaveEvents(evs []*CalendarEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range evs {
		s.Events[ev.ID] = ev
	}
	s.saveLocked()
}

func (s *Store) ReplaceEventsForPlan(planID string, evs []*CalendarEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ev := range s.Events {
		if ev.PlanID == planID {
			delete(s.Events, id)
		}
	}
	for _, ev := range evs {
		s.Events[ev.ID] = ev
	}
	s.saveLocked()
}

func (s *Store) EventsForPlan(planID string) []*CalendarEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*CalendarEvent{} // non-nil so JSON is [] not null
	for _, ev := range s.Events {
		if ev.PlanID == planID {
			out = append(out, ev)
		}
	}
	sortEvents(out)
	return out
}

func (s *Store) EventsForUser(userID string) []*CalendarEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*CalendarEvent{} // non-nil so JSON is [] not null
	for _, ev := range s.Events {
		if ev.UserID == userID {
			out = append(out, ev)
		}
	}
	sortEvents(out)
	return out
}

func sortEvents(evs []*CalendarEvent) {
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].Date == evs[j].Date {
			return evs[i].StartTime < evs[j].StartTime
		}
		return evs[i].Date < evs[j].Date
	})
}

// ---- Progress ----

func (s *Store) AddProgress(p *ProgressLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Progress = append(s.Progress, p)
	s.saveLocked()
}
