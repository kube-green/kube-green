package controllers

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

func (r *SleepInfoReconciler) getNextSchedule(schedule, nextOpSchedule string, lastSchedule, now time.Time) (bool, time.Time, time.Duration, error) {
	sched, err := getCronParsed(schedule)
	if err != nil {
		return false, time.Time{}, 0, fmt.Errorf("current schedule not valid: %s", err)
	}

	// sub 1 second because if now is after current schedule of some ms we skip
	// the current schedule
	var earliestTime time.Time = now.Add(-1 * time.Second)
	if !lastSchedule.IsZero() {
		earliestTime = lastSchedule
	}
	nextSchedule := sched.Next(earliestTime)

	if earliestTime == lastSchedule && nextSchedule.Before(now) {
		nextSchedule = sched.Next(now)
	}
	isToExecute := isTimeInDelta(now, nextSchedule, 1*time.Second)

	var requeueAfter time.Duration
	if isToExecute {
		nextOpSched, err := getCronParsed(nextOpSchedule)
		if err != nil {
			return false, time.Time{}, 0, fmt.Errorf("next op schedule not valid: %s", err)
		}
		nextSchedule = nextOpSched.Next(now.Add(1 * time.Second))
	}
	requeueAfter = getRequeueAfter(nextSchedule, now)
	r.Log.Info("is time to execute", "execute", isToExecute, "next", nextSchedule, "last", lastSchedule, "now", now)

	// TODO: add a better algorithm to correctly set requeue.
	return isToExecute, nextSchedule, requeueAfter, nil
}

func getRequeueAfter(schedule, now time.Time) time.Duration {
	return schedule.Sub(now)
}

func getCronParsed(schedule string) (cron.Schedule, error) {
	sched, err := cron.ParseStandard(schedule)
	if err != nil {
		return nil, err
	}
	return sched, nil
}

func isTimeInDelta(t1, t2 time.Time, delta time.Duration) bool {
	var diffInMs int64
	if t1.Before(t2) {
		diffInMs = t2.Sub(t1).Milliseconds()
	} else {
		diffInMs = t1.Sub(t2).Milliseconds()
	}
	return diffInMs <= delta.Milliseconds()
}
