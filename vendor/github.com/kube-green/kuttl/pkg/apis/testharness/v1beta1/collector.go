package v1beta1

import (
	"errors"
	"fmt"
	"strings"
)

const (
	pod     = "pod"
	events  = "events"
	command = "command"
)

// validate checks user input and updates type if not provided.
// It is expected to be called prior to any other call
func (tc *TestCollector) validate() error {
	cleanType(tc)
	switch tc.Type {
	case command:
		return validateCmd(tc)
	case pod:
		return validPod(tc)
	case events:
		return validEvents(tc)
	default:
		return fmt.Errorf("collector type %q unknown", tc.Type)
	}
}

func validEvents(tc *TestCollector) error {
	if tc.Cmd != "" || tc.Selector != "" || tc.Container != "" {
		return errors.New("event collector can not have a selector, container or command")
	}
	return nil
}

func validPod(tc *TestCollector) error {
	if tc.Cmd != "" {
		return errors.New("pod collector can NOT have a command")
	}
	if tc.Pod == "" && tc.Selector == "" {
		return errors.New("pod collector requires a pod or selector")
	}
	return nil
}

func validateCmd(tc *TestCollector) error {
	if tc.Cmd == "" {
		return errors.New("command collector requires a command")
	}
	if tc.Pod != "" || tc.Namespace != "" || tc.Container != "" || tc.Selector != "" {
		return errors.New("command collectors can NOT have pod, namespace, container or selectors")
	}
	return nil
}

// determines and cleans collector type
func cleanType(tc *TestCollector) {
	// intuit pod or command or invalid
	if tc.Type == "" {
		// assume command if cmd provided
		if tc.Cmd != "" {
			tc.Type = command
		} else {
			tc.Type = pod
		}
	}
	tc.Type = strings.ToLower(tc.Type)
}

// Command provides the command to exec to perform the collection
func (tc *TestCollector) Command() *Command {
	err := tc.validate()
	if err != nil {
		return nil
	}
	switch tc.Type {
	case pod:
		return podCommand(tc)
	case command:
		return &Command{
			Command:       tc.Cmd,
			IgnoreFailure: true,
		}
	case events:
		return eventCommand(tc)
	}
	return nil
}

func eventCommand(tc *TestCollector) *Command {
	var b strings.Builder
	b.WriteString("kubectl get events")
	if len(tc.Pod) > 0 {
		fmt.Fprintf(&b, " %s", tc.Pod)
	}
	ns := tc.Namespace
	if len(tc.Namespace) == 0 {
		ns = "$NAMESPACE"
	}
	fmt.Fprintf(&b, " -n %s", ns)
	return &Command{
		Command:       b.String(),
		IgnoreFailure: true,
	}
}

func podCommand(tc *TestCollector) *Command {
	var b strings.Builder
	b.WriteString("kubectl logs --prefix")
	if len(tc.Pod) > 0 {
		fmt.Fprintf(&b, " %s", tc.Pod)
	}
	if len(tc.Selector) > 0 {
		fmt.Fprintf(&b, " -l %s", tc.Selector)
	}
	ns := tc.Namespace
	if len(tc.Namespace) == 0 {
		ns = "$NAMESPACE"
	}
	fmt.Fprintf(&b, " -n %s", ns)
	if len(tc.Container) > 0 {
		fmt.Fprintf(&b, " -c %s", tc.Container)
	} else {
		b.WriteString(" --all-containers")
	}
	if tc.Tail == 0 {
		if len(tc.Selector) > 0 {
			tc.Tail = 10
		} else {
			tc.Tail = -1
		}
	}
	fmt.Fprintf(&b, " --tail=%d", tc.Tail)
	return &Command{
		Command:       b.String(),
		IgnoreFailure: true,
	}
}

// String provides defaults of the type of collector
func (tc *TestCollector) String() string {
	err := tc.validate()
	if err != nil {
		return fmt.Sprintf("[collector invalid: %s]", err.Error())
	}

	var b strings.Builder
	b.WriteString("[")
	details := []string{}
	details = append(details, fmt.Sprintf("type==%s", tc.Type))
	if len(tc.Pod) > 0 {
		details = append(details, fmt.Sprintf("pod==%s", tc.Pod))
	}
	if len(tc.Selector) > 0 {
		details = append(details, fmt.Sprintf("label: %s", tc.Selector))
	}
	if len(tc.Namespace) > 0 {
		details = append(details, fmt.Sprintf("namespace: %s", tc.Namespace))
	}
	if len(tc.Container) > 0 {
		details = append(details, fmt.Sprintf("container: %s", tc.Container))
	}
	if len(tc.Cmd) > 0 {
		details = append(details, fmt.Sprintf("command: %s", tc.Cmd))
	}
	b.WriteString(strings.Join(details, ","))
	b.WriteString("]")
	return b.String()
}
