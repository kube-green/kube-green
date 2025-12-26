package utils

// Contains functions helpful for running commands.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/shlex"
	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // package needed for auth providers like GCP

	harness "github.com/kube-green/kuttl/pkg/apis/testharness/v1beta1"
	"github.com/kube-green/kuttl/pkg/env"
)

// GetArgs parses a command line string into its arguments and appends a namespace if it is not already set.
func GetArgs(ctx context.Context, cmd harness.Command, namespace string, envMap map[string]string) (*exec.Cmd, error) {
	argSlice := []string{}

	if cmd.Command != "" && cmd.Script != "" {
		return nil, errors.New("command and script can not be set in the same configuration")
	}
	if cmd.Command == "" && cmd.Script == "" {
		return nil, errors.New("command or script must be set")
	}
	if cmd.Script != "" && cmd.Namespaced {
		return nil, errors.New("script can not used 'namespaced', use the $NAMESPACE environment variable instead")
	}

	if cmd.Script != "" {
		// #nosec G204 sec is challenged by a variable being used by exec, but that is by design
		builtCmd := exec.CommandContext(ctx, "sh", "-c", cmd.Script)
		return builtCmd, nil
	}
	c := env.ExpandWithMap(cmd.Command, envMap)

	argSplit, err := shlex.Split(c)
	if err != nil {
		return nil, err
	}

	argSlice = append(argSlice, argSplit...)

	if cmd.Namespaced {
		fs := pflag.NewFlagSet("", pflag.ContinueOnError)
		fs.ParseErrorsWhitelist.UnknownFlags = true

		namespaceParsed := fs.StringP("namespace", "n", "", "")
		if err := fs.Parse(argSplit); err != nil {
			return nil, err
		}

		if *namespaceParsed == "" {
			argSlice = append(argSlice, "--namespace", namespace)
		}
	}

	//nolint:gosec // We're running a user provided command. This is insecure by definition
	builtCmd := exec.CommandContext(ctx, argSlice[0])
	builtCmd.Args = argSlice
	return builtCmd, nil
}

// RunCommand runs a command with args.
// args gets split on spaces (respecting quoted strings).
// if the command is run in the background a reference to the process is returned for later cleanup
func RunCommand(ctx context.Context, namespace string, cmd harness.Command, cwd string, stdout io.Writer, stderr io.Writer, logger Logger, timeout int, kubeconfigOverride string) (*exec.Cmd, error) {
	actualDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("command %q with %w", cmd.String(), err)
	}

	kuttlENV := make(map[string]string)
	kuttlENV["NAMESPACE"] = namespace
	kuttlENV["KUBECONFIG"] = kubeconfigPath(actualDir, kubeconfigOverride)
	kuttlENV["PATH"] = fmt.Sprintf("%s/bin/:%s", actualDir, os.Getenv("PATH"))

	// by default testsuite timeout is the command timeout
	// 0 is allowed for testsuite which means forever (or no timeout)
	// cmd.timeout defaults to 0 and is NOT and explicit override to mean forever.
	// using a negative value for cmd.timeout is an override of testsuite.timeout to mean forever
	// if testsuite.timeout is set,  set cmd.timeout = -1 (means no timeout), 0 (default) means using testsuite.timeout, and anything else is an override of time.
	// if testsuite.timeout = 0, cmd.timeout -1 and 0 means forever
	if cmd.Timeout < 0 {
		// negative is always forever
		timeout = 0
	}
	if cmd.Timeout > 0 {
		timeout = cmd.Timeout
	}

	// command context is provided context or a cancel context but only from cmds that are not background
	cmdCtx := ctx
	if timeout > 0 && !cmd.Background {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	builtCmd, err := GetArgs(cmdCtx, cmd, namespace, kuttlENV)
	if err != nil {
		return nil, fmt.Errorf("processing command %q with %w", cmd.String(), err)
	}

	logger.Logf("running command: %v", builtCmd.Args)

	builtCmd.Dir = cwd
	if !cmd.SkipLogOutput {
		builtCmd.Stdout = stdout
		builtCmd.Stderr = stderr
	}
	builtCmd.Env = os.Environ()
	for key, value := range kuttlENV {
		builtCmd.Env = append(builtCmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// process started and exited with error
	var exerr *exec.ExitError
	err = builtCmd.Start()
	if err != nil {
		if errors.As(err, &exerr) && cmd.IgnoreFailure {
			return nil, nil
		}
		return nil, err
	}

	if cmd.Background {
		return builtCmd, nil
	}

	err = builtCmd.Wait()
	if errors.As(err, &exerr) && cmd.IgnoreFailure {
		return nil, nil
	}
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("command %q exceeded %v sec timeout, %w", cmd.String(), timeout, cmdCtx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("command %q failed, %w", cmd.String(), err)
	}
	return nil, nil
}

func kubeconfigPath(actualDir, override string) string {
	if override != "" {
		if filepath.IsAbs(override) {
			return override
		}
		return filepath.Join(actualDir, override)
	}
	return fmt.Sprintf("%s/kubeconfig", actualDir)
}

// convertAssertCommand converts a set of TestAssertCommand to Commands so it all the existing functions can be used
// that expect Commands data type.
func convertAssertCommand(assertCommands []harness.TestAssertCommand, timeout int) (commands []harness.Command) {
	commands = make([]harness.Command, 0, len(assertCommands))

	for _, assertCommand := range assertCommands {
		commands = append(commands, harness.Command{
			Command:       assertCommand.Command,
			Namespaced:    assertCommand.Namespaced,
			Script:        assertCommand.Script,
			SkipLogOutput: assertCommand.SkipLogOutput,
			Timeout:       timeout,
			// This fields will always be this constants for assertions
			IgnoreFailure: false,
			Background:    false,
		})
	}

	return commands
}

// RunAssertCommands runs a set of commands specified as TestAssertCommand
func RunAssertCommands(ctx context.Context, logger Logger, namespace string, commands []harness.TestAssertCommand, workdir string, timeout int, kubeconfigOverride string) ([]*exec.Cmd, error) {
	return RunCommands(ctx, logger, namespace, convertAssertCommand(commands, timeout), workdir, timeout, kubeconfigOverride)
}

// RunCommands runs a set of commands, returning any errors.
// If any (non-background) command fails, the following commands are skipped
// commands running in the background are returned
func RunCommands(ctx context.Context, logger Logger, namespace string, commands []harness.Command, workdir string, timeout int, kubeconfigOverride string) ([]*exec.Cmd, error) {
	bgs := []*exec.Cmd{}

	if commands == nil {
		return nil, nil
	}

	for i, cmd := range commands {
		bg, err := RunCommand(ctx, namespace, cmd, workdir, logger, logger, logger, timeout, kubeconfigOverride)
		if err != nil {
			cmdListSize := len(commands)
			if i+1 < cmdListSize {
				logger.Logf("command failure, skipping %d additional commands", cmdListSize-i-1)
			}
			return bgs, err
		}
		if bg != nil {
			bgs = append(bgs, bg)
		} else {
			// We only need to flush if this is not a background command
			logger.Flush()
		}
	}

	if len(bgs) > 0 {
		logger.Log("background processes", bgs)
	}
	// handling of errs and bg processes external to this function
	return bgs, nil
}
