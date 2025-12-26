package expressions

import (
	"errors"
	"fmt"

	"github.com/google/cel-go/cel"

	harness "github.com/kube-green/kuttl/pkg/apis/testharness/v1beta1"
)

func buildProgram(expr string, env *cel.Env) (cel.Program, error) {
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("type-check error: %s", issues.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("program construction error: %w", err)
	}

	return prg, nil
}

func buildEnv(resourceRefs []harness.TestResourceRef) (*cel.Env, error) {
	var errs []error
	for _, resourceRef := range resourceRefs {
		if err := resourceRef.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("validation failed for reference '%v': %w", resourceRef.String(), err))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to load resource reference(s): %w", errors.Join(errs...))
	}

	env, err := cel.NewEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	for _, resourceRef := range resourceRefs {
		env, err = env.Extend(cel.Variable(resourceRef.Ref, cel.DynType))
		if err != nil {
			return nil, fmt.Errorf("failed to add resource parameter '%v' to environment: %w", resourceRef.Ref, err)
		}
	}

	return env, nil
}

// RunAssertExpressions evaluates a set of CEL expressions.
func RunAssertExpressions(
	programs map[string]cel.Program,
	variables map[string]interface{},
	assertAny,
	assertAll []*harness.Assertion,
) []error {
	var errs []error
	if len(assertAny) == 0 && len(assertAll) == 0 {
		return errs
	}

	var anyExprErrors, allExprErrors []error
	for _, expr := range assertAny {
		if err := evaluateExpression(expr.CELExpression, programs, variables); err != nil {
			anyExprErrors = append(anyExprErrors, err)
		}
	}

	for _, expr := range assertAll {
		if err := evaluateExpression(expr.CELExpression, programs, variables); err != nil {
			allExprErrors = append(allExprErrors, err)
		}
	}

	if len(assertAny) != 0 && len(anyExprErrors) == len(assertAny) {
		errs = append(errs, fmt.Errorf("no expression evaluated to true: %w", errors.Join(anyExprErrors...)))
	}

	if len(allExprErrors) > 0 {
		errs = append(errs, fmt.Errorf("not all assertAll expressions evaluated to true: %w", errors.Join(allExprErrors...)))
	}

	return errs
}

func LoadPrograms(testAssert *harness.TestAssert) (map[string]cel.Program, error) {
	var errs []error
	var assertions []*harness.Assertion
	assertions = append(assertions, testAssert.AssertAny...)
	assertions = append(assertions, testAssert.AssertAll...)

	env, err := buildEnv(testAssert.ResourceRefs)
	if err != nil {
		return nil, fmt.Errorf("failed to build CEL environment: %w", err)
	}

	if len(assertions) == 0 {
		return nil, nil
	}
	programs := make(map[string]cel.Program)

	for _, assertion := range assertions {
		if prg, err := buildProgram(assertion.CELExpression, env); err != nil {
			errs = append(
				errs,
				fmt.Errorf("failed to build CEL program from expression %q: %w", assertion.CELExpression, err),
			)
		} else {
			programs[assertion.CELExpression] = prg
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to load expression(s): %w", errors.Join(errs...))
	}

	return programs, nil
}

func evaluateExpression(expr string,
	programs map[string]cel.Program,
	variables map[string]interface{},
) error {
	prg, ok := programs[expr]
	if !ok {
		return fmt.Errorf("couldn't find pre-built parsed CEL expression %q", expr)
	}
	out, _, err := prg.Eval(variables)
	if err != nil {
		return fmt.Errorf("failed to evaluate CEL expression: %w", err)
	}

	if out.Value() != true {
		return fmt.Errorf("expression %q evaluated to '%v'", expr, out.Value())
	}

	return nil
}
