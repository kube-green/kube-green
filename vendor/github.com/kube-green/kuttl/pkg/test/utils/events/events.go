package events

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsbeta1 "k8s.io/api/events/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	testutils "github.com/kube-green/kuttl/pkg/test/utils"
)

// byFirstTimestamp sorts a slice of events by first timestamp, using their involvedObject's name as a tie breaker.
type byFirstTimestamp []eventsbeta1.Event

func (o byFirstTimestamp) Len() int      { return len(o) }
func (o byFirstTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o byFirstTimestamp) Less(i, j int) bool {
	if o[i].ObjectMeta.CreationTimestamp.Equal(&o[j].ObjectMeta.CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].ObjectMeta.CreationTimestamp.Before(&o[j].ObjectMeta.CreationTimestamp)
}

// byFirstTimestampV1 sorts a slice of eventsv1 by first timestamp, using their involvedObject's name as a tie breaker.
type byFirstTimestampV1 []eventsv1.Event

func (o byFirstTimestampV1) Len() int      { return len(o) }
func (o byFirstTimestampV1) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o byFirstTimestampV1) Less(i, j int) bool {
	if o[i].ObjectMeta.CreationTimestamp.Equal(&o[j].ObjectMeta.CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].ObjectMeta.CreationTimestamp.Before(&o[j].ObjectMeta.CreationTimestamp)
}

// byFirstTimestampCoreV1 sorts a slice of corev1 by first timestamp, using their involvedObject's name as a tie breaker.
type byFirstTimestampCoreV1 []corev1.Event

func (o byFirstTimestampCoreV1) Len() int      { return len(o) }
func (o byFirstTimestampCoreV1) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o byFirstTimestampCoreV1) Less(i, j int) bool {
	if o[i].ObjectMeta.CreationTimestamp.Equal(&o[j].ObjectMeta.CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].ObjectMeta.CreationTimestamp.Before(&o[j].ObjectMeta.CreationTimestamp)
}

// CollectAndLog retrieves event resources for a given namespace and logs them with the logger.
// It tries several event APIs in turn: events/v1, events/v1beta1, core/v1.
func CollectAndLog(ctx context.Context, cl client.Client, namespace string, caseName string, logger testutils.Logger) {
	err := collectEventsV1(ctx, cl, namespace, caseName, logger)
	if err != nil {
		logger.Log("Trying with events eventsv1beta1 API...")
		err = collectEventsBeta1(ctx, cl, namespace, caseName, logger)
		if err != nil {
			logger.Log("Trying with events corev1 API...")
			err = collectEventsCoreV1(ctx, cl, namespace, caseName, logger)
			if err != nil {
				logger.Log("All event APIs failed")
			}
		}
	}
}

func collectEventsBeta1(ctx context.Context, cl client.Client, namespace string, caseName string, logger testutils.Logger) error {
	eventsList := &eventsbeta1.EventList{}

	err := cl.List(ctx, eventsList, client.InNamespace(namespace))
	if err != nil {
		logger.Logf("Failed to collect events for %s in ns %s: %v", caseName, namespace, err)
		return err
	}

	events := eventsList.Items
	sort.Sort(byFirstTimestamp(events))

	logger.Logf("%s events from ns %s:", caseName, namespace)
	printEventsBeta1(events, logger)
	return nil
}

func collectEventsV1(ctx context.Context, cl client.Client, namespace string, caseName string, logger testutils.Logger) error {
	eventsList := &eventsv1.EventList{}

	err := cl.List(ctx, eventsList, client.InNamespace(namespace))
	if err != nil {
		logger.Logf("Failed to collect events for %s in ns %s: %v", caseName, namespace, err)
		return err
	}

	events := eventsList.Items
	sort.Sort(byFirstTimestampV1(events))

	logger.Logf("%s events from ns %s:", caseName, namespace)
	printEventsV1(events, logger)
	return nil
}

func collectEventsCoreV1(ctx context.Context, cl client.Client, namespace string, caseName string, logger testutils.Logger) error {
	eventsList := &corev1.EventList{}

	err := cl.List(ctx, eventsList, client.InNamespace(namespace))
	if err != nil {
		logger.Logf("Failed to collect events for %s in ns %s: %v", caseName, namespace, err)
		return err
	}

	events := eventsList.Items
	sort.Sort(byFirstTimestampCoreV1(events))

	logger.Logf("%s events from ns %s:", caseName, namespace)
	printEventsCoreV1(events, logger)
	return nil
}

func printEventsBeta1(events []eventsbeta1.Event, logger testutils.Logger) {
	for _, e := range events {
		// time type regarding action reason note reportingController related
		logger.Logf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
			e.ObjectMeta.CreationTimestamp,
			e.Type,
			shortString(&e.Regarding),
			e.Action,
			e.Reason,
			e.Note,
			e.ReportingController,
			shortString(e.Related))
	}
}

func printEventsV1(events []eventsv1.Event, logger testutils.Logger) {
	for _, e := range events {
		// time type regarding action reason note reportingController related
		logger.Logf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
			e.ObjectMeta.CreationTimestamp,
			e.Type,
			shortString(&e.Regarding),
			e.Action,
			e.Reason,
			e.Note,
			e.ReportingController,
			shortString(e.Related))
	}
}

func printEventsCoreV1(events []corev1.Event, logger testutils.Logger) {
	for _, e := range events {
		// time type regarding action reason note reportingController related
		logger.Logf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
			e.ObjectMeta.CreationTimestamp,
			e.Type,
			shortString(&e.InvolvedObject),
			e.Action,
			e.Reason,
			e.Message,
			e.ReportingController,
			shortString(e.Related))
	}
}

func shortString(obj *corev1.ObjectReference) string {
	if obj == nil {
		return ""
	}
	fieldRef := ""
	if obj.FieldPath != "" {
		fieldRef = "." + obj.FieldPath
	}
	return fmt.Sprintf("%s %s%s",
		obj.GroupVersionKind().GroupKind().String(),
		obj.Name,
		fieldRef)
}
