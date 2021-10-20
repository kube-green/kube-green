package sleepinfo

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Test Schedule", func() {
	testLogger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))

	sleepInfoReconciler := SleepInfoReconciler{
		Client: k8sClient,
		Log:    testLogger,
	}

	type expected struct {
		isToExecute  bool
		nextSchedule string
		requeueAfter time.Duration
		err          string
	}

	tests := []struct {
		name                 string
		now                  string
		data                 SleepInfoData
		scheduleDeltaSeconds int64
		expected             expected
	}{
		{
			name: "fails if current schedule is invalid",
			now:  "2021-03-23T20:05:20.555Z",
			data: SleepInfoData{CurrentOperationSchedule: "* * * *"},
			expected: expected{
				isToExecute:  false,
				nextSchedule: "",
				requeueAfter: 0,
				err:          "current schedule not valid: expected exactly 5 fields, found 4: [* * * *]",
			},
		},
		{
			name: "fails if next op schedule is invalid",
			now:  "2021-03-23T20:05:59.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "* * * * *",
				NextOperationSchedule:    "* * * *",
			},
			expected: expected{
				isToExecute:  false,
				nextSchedule: "",
				requeueAfter: 0,
				err:          "next op schedule not valid: expected exactly 5 fields, found 4: [* * * *]",
			},
		},
		{
			name: "no last schedule, is time to execute [now -1s]",
			now:  "2021-03-23T20:05:59.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 4*time.Minute + 1*time.Second,
			},
		},
		{
			name: "last schedule (+1s), is time to execute [now -1s]",
			now:  "2021-03-23T20:05:59.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z").Add(1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 4*time.Minute + 1*time.Second,
			},
		},
		{
			name: "last schedule (-1s), is time to execute [now -1s]",
			now:  "2021-03-23T20:05:59.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z").Add(-1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 4*time.Minute + 1*time.Second,
			},
		},
		{
			name: "last schedule (at least one operation skipped), is time to execute [now -1s]",
			now:  "2021-03-23T20:05:59.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z"),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 4*time.Minute + 1*time.Second,
			},
		},
		{
			name: "no last schedule, is time to execute [now +1s]",
			now:  "2021-03-23T20:06:00.999Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 3*time.Minute + 59*time.Second + 1*time.Millisecond,
			},
		},
		{
			name: "last schedule (+1s), is time to execute [now +1s]",
			now:  "2021-03-23T20:06:01.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z").Add(1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 3*time.Minute + 59*time.Second,
			},
		},
		{
			name: "last schedule (-1s), is time to execute [now +1s]",
			now:  "2021-03-23T20:06:01.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z").Add(-1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 3*time.Minute + 59*time.Second,
			},
		},
		{
			name: "last schedule (+1s), is time to execute [now]",
			now:  "2021-03-23T20:06:00.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z").Add(1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 4 * time.Minute,
			},
		},
		{
			name: "last schedule (-1s), is time to execute [now]",
			now:  "2021-03-23T20:06:00.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z").Add(-1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 4 * time.Minute,
			},
		},
		{
			name: "last schedule (at least one operation skipped), is time to execute [now +1s]",
			now:  "2021-03-23T20:06:01.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z"),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 3*time.Minute + 59*time.Second,
			},
		},
		{
			name: "no last schedule, no execution",
			now:  "2021-03-23T20:00:59.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
			},
			expected: expected{
				isToExecute:  false,
				nextSchedule: "2021-03-23T20:06:00Z",
				requeueAfter: 5*time.Minute + 1*time.Second,
			},
		},
		{
			name: "with last schedule, no execution",
			now:  "2021-03-23T20:00:59.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z"),
			},
			expected: expected{
				isToExecute:  false,
				nextSchedule: "2021-03-23T20:06:00Z",
				requeueAfter: 5*time.Minute + 1*time.Second,
			},
		},
		{
			name: "with last schedule (at least one operation skipped), no execution",
			now:  "2021-03-23T20:00:59.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T15:10:00.000Z"),
			},
			expected: expected{
				isToExecute:  false,
				nextSchedule: "2021-03-23T20:06:00Z",
				requeueAfter: 5*time.Minute + 1*time.Second,
			},
		},
		{
			name: "same next and current schedule - last schedule (-1s), is time to execute [now]",
			now:  "2021-03-23T20:06:00.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "6 * * * *",
				LastSchedule:             getTime("2021-03-23T19:06:00.000Z").Add(-1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T21:06:00Z",
				requeueAfter: 60 * time.Minute,
			},
		},
		{
			name: "same next and current schedule - last schedule (+1s), is time to execute [now]",
			now:  "2021-03-23T20:06:00.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "6 * * * *",
				LastSchedule:             getTime("2021-03-23T19:06:00.000Z").Add(1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T21:06:00Z",
				requeueAfter: 60 * time.Minute,
			},
		},
		{
			name: "schedule contains a timezone",
			now:  "2021-03-23T17:00:00.000Z", // timezone UTC+1
			data: SleepInfoData{
				CurrentOperationSchedule: "CRON_TZ=Europe/Rome 0 19 * * *",
				NextOperationSchedule:    "CRON_TZ=Europe/Rome 0 8 * * *",
				LastSchedule:             getTime("2021-03-23T08:00:00.000Z").Add(1 * time.Second),
			},
			expected: expected{
				isToExecute:  false,
				nextSchedule: "2021-03-23T18:00:00Z",
				requeueAfter: 1 * time.Hour,
			},
		},
		{
			name: "schedule contains a timezone - timezone after hour change",
			now:  "2021-04-29T17:00:00.000Z", // timezone UTC+2
			data: SleepInfoData{
				CurrentOperationSchedule: "CRON_TZ=Europe/Rome 0 19 * * *",
				NextOperationSchedule:    "CRON_TZ=Europe/Rome 0 8 * * *",
				LastSchedule:             getTime("2021-04-29T06:00:00.000Z").Add(1 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-04-30T06:00:00Z",
				requeueAfter: 13 * time.Hour,
			},
		},
		{
			name: "no last schedule, is time to execute [now -60s] - delta 60s",
			now:  "2021-03-23T20:05:00.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 5 * time.Minute,
			},
			scheduleDeltaSeconds: 60,
		},
		{
			name: "no last schedule, is time to execute [now +60s] - delta 60s",
			now:  "2021-03-23T20:06:59.999Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 3*time.Minute + 1*time.Millisecond,
			},
			scheduleDeltaSeconds: 60,
		},
		{
			name: "last schedule (+60s), is time to execute [now -60s] - delta 60s",
			now:  "2021-03-23T20:05:00.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z").Add(60 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 5 * time.Minute,
			},
			scheduleDeltaSeconds: 60,
		},
		{
			name: "last schedule (-60s), is time to execute [now +60s] - delta 60s",
			now:  "2021-03-23T20:07:00.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z").Add(-60 * time.Second),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 3 * time.Minute,
			},
			scheduleDeltaSeconds: 60,
		},
		{
			name: "last schedule, is time to execute [now +60s] - delta 60s",
			now:  "2021-03-23T20:06:00.000Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "6 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T19:10:00.000Z"),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 4 * time.Minute,
			},
			scheduleDeltaSeconds: 60,
		},
		{
			name: "last schedule (at least one operation skipped), is time to execute [now -60s]",
			now:  "2021-03-23T20:05:59.999Z",
			data: SleepInfoData{
				CurrentOperationSchedule: "5 * * * *",
				NextOperationSchedule:    "10 * * * *",
				LastSchedule:             getTime("2021-03-23T18:05:00.000Z"),
			},
			expected: expected{
				isToExecute:  true,
				nextSchedule: "2021-03-23T20:10:00Z",
				requeueAfter: 4*time.Minute + 1*time.Millisecond,
			},
			scheduleDeltaSeconds: 60,
		},
	}

	for _, test := range tests {
		test := test //necessary to ensure the correct value is passed to the closure
		It(test.name, func() {
			scheduleDeltaSeconds := test.scheduleDeltaSeconds
			if scheduleDeltaSeconds == 0 {
				scheduleDeltaSeconds = 1
			}
			isToExecute, nextSchedule, requeueAfter, err := sleepInfoReconciler.getNextSchedule(test.data, getTime(test.now), scheduleDeltaSeconds)

			expected := test.expected
			if expected.err != "" {
				Expect(err).To(MatchError(expected.err))
			} else {
				Expect(err).To(BeNil())
			}
			Expect(isToExecute).To(Equal(expected.isToExecute))
			if expected.nextSchedule != "" {
				Expect(nextSchedule.Format(time.RFC3339)).To(Equal(expected.nextSchedule))
			}
			Expect(requeueAfter).To(Equal(expected.requeueAfter))
		})
	}
})

var _ = Describe("TestIsTimeInDeltaMs", func() {
	now := time.Now()
	tests := []struct {
		name     string
		t1       time.Time
		t2       time.Time
		expected bool
		delta    time.Duration
	}{
		{
			name:     "t1 > t2 30s - delta 60s",
			t1:       now,
			t2:       now.Add(60 * time.Second),
			delta:    time.Second * 60,
			expected: true,
		},
		{
			name:     "t1 > t2 1ms - delta 1ms",
			t1:       now,
			t2:       now.Add(1 * time.Millisecond),
			delta:    time.Millisecond * 1,
			expected: true,
		},
		{
			name:     "t1 > t2 31s - delta 30s",
			t1:       now,
			t2:       now.Add(31 * time.Second),
			delta:    time.Second * 30,
			expected: false,
		},
		{
			name:     "t1 > t2 30s - delta 60s",
			t1:       now.Add(60 * time.Second),
			t2:       now,
			delta:    time.Second * 60,
			expected: true,
		},
		{
			name:     "t1 < t2 31s - delta 30s",
			t1:       now.Add(31 * time.Second),
			t2:       now,
			delta:    time.Second * 30,
			expected: false,
		},
		{
			name:     "t1 > t2 1s - delta 1s",
			t1:       now.Add(1 * time.Second),
			t2:       now,
			delta:    time.Second * 1,
			expected: true,
		},
	}
	for _, test := range tests {
		test := test //necessary to ensure the correct value is passed to the closure
		It(fmt.Sprintf("name, %s", test.name), func() {
			output := isTimeInDelta(test.t1, test.t2, test.delta)
			Expect(output).To(Equal(test.expected))
		})
	}
})
