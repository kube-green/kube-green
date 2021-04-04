package controllers

import (
	"fmt"
	"time"

	kubegreenv1alpha1 "github.com/davidebianchi/kube-green/api/v1alpha1"
	"github.com/robfig/cron/v3"
)

func (r *SleepInfoReconciler) getNextSchedule(sleepInfo *kubegreenv1alpha1.SleepInfo, now time.Time) (bool, time.Time, time.Duration, error) {
	sched, err := cron.ParseStandard(sleepInfo.Spec.SleepSchedule)
	if err != nil {
		return false, time.Time{}, 0, fmt.Errorf("sleep schedule not valid: %s", err)
	}

	var earliestTime time.Time
	if !sleepInfo.Status.LastScheduleTime.IsZero() {
		earliestTime = sleepInfo.Status.LastScheduleTime.Time
	} else {
		earliestTime = now
	}
	nextSchedule := sched.Next(earliestTime)

	if nextSchedule.Before(now) {
		nextSchedule = sched.Next(now)
	}
	isToExecute := isTimeInDelta(now, nextSchedule, 1*time.Second)

	var requeueAfter time.Duration
	if isToExecute {
		nextSchedule = sched.Next(now.Add(1 * time.Second))
	}
	requeueAfter = nextSchedule.Sub(now)
	r.Log.Info("is time to execute", "execute", isToExecute, "next", nextSchedule)

	// TODO: add a better algorithm to correctly set requeue.
	return isToExecute, nextSchedule, requeueAfter, nil
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
