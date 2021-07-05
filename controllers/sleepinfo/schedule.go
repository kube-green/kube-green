package controllers

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

func (r *SleepInfoReconciler) getNextSchedule(data SleepInfoData, now time.Time, scheduleDeltaSeconds int64) (bool, time.Time, time.Duration, error) {
	scheduleDelta := time.Duration(scheduleDeltaSeconds) * time.Second
	sched, err := getCronParsed(data.CurrentOperationSchedule)
	if err != nil {
		return false, time.Time{}, 0, fmt.Errorf("current schedule not valid: %s", err)
	}

	lastSchedule := data.LastSchedule

	// subtract delta seconds because if now is after current schedule we skip
	// the current schedule
	var earliestTime time.Time = now.Add(-scheduleDelta)
	if !lastSchedule.IsZero() {
		earliestTime = lastSchedule
	}
	nextSchedule := sched.Next(earliestTime)

	if earliestTime == lastSchedule && nextSchedule.Before(now) && !isTimeInDelta(nextSchedule, now, scheduleDelta) {
		nextSchedule = sched.Next(now.Add(-scheduleDelta))
	}
	isToExecute := isTimeInDelta(now, nextSchedule, scheduleDelta)

	var requeueAfter time.Duration
	if isToExecute {
		nextOpSched, err := getCronParsed(data.NextOperationSchedule)
		if err != nil {
			return false, time.Time{}, 0, fmt.Errorf("next op schedule not valid: %s", err)
		}
		nextSchedule = nextOpSched.Next(now.Add(scheduleDelta))
	}
	requeueAfter = getRequeueAfter(nextSchedule, now)
	r.Log.Info("is time to execute", "execute", isToExecute, "next", nextSchedule, "last", lastSchedule, "now", now)

	return isToExecute, nextSchedule, requeueAfter, nil
}

func getRequeueAfter(schedule, now time.Time) time.Duration {
	return schedule.Sub(now)
}

func getCronParsed(schedule string) (cron.Schedule, error) {
	return cron.ParseStandard(schedule)
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
