package handlers

import (
	"log"
	"sync"

	"github.com/robfig/cron/v3"
)

var (
	scheduler     *cron.Cron
	schedulerOnce sync.Once
	policyEntries = make(map[int64]cron.EntryID)
	policyMu      sync.Mutex
)

// InitScheduler initialises the cron scheduler and loads all enabled policies.
// Must be called after InitDB from main.go.
func InitScheduler() {
	schedulerOnce.Do(func() {
		scheduler = cron.New()
		loadAndScheduleAllPolicies()
		scheduler.Start()
		log.Println("Cron backup scheduler started.")
	})
}

// StopScheduler stops the scheduler gracefully. Called on server shutdown.
func StopScheduler() {
	if scheduler != nil {
		ctx := scheduler.Stop()
		<-ctx.Done()
		log.Println("Cron backup scheduler stopped.")
	}
}

// loadAndScheduleAllPolicies reads every enabled policy from DB and schedules each one.
func loadAndScheduleAllPolicies() {
	rows, err := db.Query("SELECT id, schedule FROM backup_policies WHERE enabled = 1")
	if err != nil {
		log.Printf("Scheduler: failed to query policies: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int64
		var schedule string
		if err := rows.Scan(&id, &schedule); err != nil {
			log.Printf("Scheduler: scan error: %v", err)
			continue
		}
		schedulePolicy(id, schedule)
		count++
	}
	log.Printf("Scheduler: loaded %d enabled policy(ies).", count)
}

// schedulePolicy adds or replaces a single policy in the cron scheduler.
func schedulePolicy(policyID int64, schedule string) {
	policyMu.Lock()
	defer policyMu.Unlock()

	// Remove existing entry if present (handles updates)
	if entryID, ok := policyEntries[policyID]; ok {
		scheduler.Remove(entryID)
		delete(policyEntries, policyID)
	}

	id := policyID // capture for closure
	entryID, err := scheduler.AddFunc(schedule, func() {
		log.Printf("Scheduler: firing policy %d", id)
		go RunPolicy(id)
	})
	if err != nil {
		log.Printf("Scheduler: failed to schedule policy %d (%q): %v", policyID, schedule, err)
		return
	}
	policyEntries[policyID] = entryID
	log.Printf("Scheduler: policy %d registered (%s)", policyID, schedule)
}

// unschedulePolicy removes a single policy from the scheduler.
func unschedulePolicy(policyID int64) {
	policyMu.Lock()
	defer policyMu.Unlock()

	if entryID, ok := policyEntries[policyID]; ok {
		scheduler.Remove(entryID)
		delete(policyEntries, policyID)
		log.Printf("Scheduler: policy %d removed", policyID)
	}
}

// ReloadScheduler removes all entries and reloads from DB. Used after bulk changes.
func ReloadScheduler() {
	policyMu.Lock()
	for _, entryID := range policyEntries {
		scheduler.Remove(entryID)
	}
	policyEntries = make(map[int64]cron.EntryID)
	policyMu.Unlock()

	loadAndScheduleAllPolicies()
}

// NextRunTime returns the next scheduled run time for a policy, or "" if not scheduled.
func NextRunTime(policyID int64) string {
	policyMu.Lock()
	defer policyMu.Unlock()

	entryID, ok := policyEntries[policyID]
	if !ok {
		return "—"
	}
	entry := scheduler.Entry(entryID)
	if entry.Next.IsZero() {
		return "—"
	}
	return entry.Next.Format("2006-01-02 15:04")
}
