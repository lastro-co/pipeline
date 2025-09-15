// Package pipeline provides a small, type-safe, and idiomatic way to
// compose a sequence of operations (steps) on a value, similar to Elixir's
// pipe operator.
package pipeline

import (
	"context"
	"fmt"
)

// Step represents a single transformation over a value of type T.
// It returns the transformed value or an error and receives a context.
// If an error occurs or context is cancelled, execution stops.
type Step[T any] func(context.Context, T) (T, error)

// StepFunc defines the constraint for valid step function types
type StepFunc[T any] interface {
	~func(T) (T, error) | ~func(context.Context, T) (T, error) | ~func(T) T | ~func(context.Context, T) T
}

// PipelineError wraps errors that occur during pipeline execution with additional context.
type PipelineError struct {
	StepIndex   int
	TotalSteps  int
	OriginalErr error
	LastValue   any
}

func (e *PipelineError) Error() string {
	return fmt.Sprintf("pipeline failed at step %d/%d: %v", e.StepIndex, e.TotalSteps, e.OriginalErr)
}

func (e *PipelineError) Unwrap() error {
	return e.OriginalErr
}

// Pipeline holds a value of type T and a list of steps to apply to it.
// Use New to create a pipeline, Do/DoWithContext to register steps, and
// Execute/ExecuteWithContext to run them.
type Pipeline[T any] struct {
	value T
	steps []Step[T]
}

// New creates a new Pipeline with an initial value.
func New[T any](initial T) *Pipeline[T] {
	return &Pipeline[T]{value: initial}
}

// Do appends a step to the pipeline. For other function types,
// use the helper function ToStep to convert them.
func (p *Pipeline[T]) Do(step Step[T]) *Pipeline[T] {
	p.steps = append(p.steps, step)
	return p
}

// ToStep converts various function types to Step[T] for use with Do.
// It accepts functions of type:
// - func(T) T: Functions that never return an error
// - func(T) (T, error): Functions that may return an error
// - func(context.Context, T) T: Context-aware functions that never return an error
// - func(context.Context, T) (T, error): Functions that are already context-aware
func ToStep[T any, F StepFunc[T]](f F) Step[T] {
	var step Step[T]

	switch s := any(f).(type) {
	case func(T) T:
		step = func(ctx context.Context, value T) (T, error) {
			return s(value), nil
		}
	case func(T) (T, error):
		step = func(ctx context.Context, value T) (T, error) {
			return s(value)
		}
	case func(context.Context, T) T:
		step = func(ctx context.Context, value T) (T, error) {
			return s(ctx, value), nil
		}
	case func(context.Context, T) (T, error):
		step = s
	}

	return step
}

// Execute runs each registered step in order, passing the result of the
// previous step to the next. Execution stops on the first error.
// It returns the final value (or the last successful value) and the error, if any.
func (p *Pipeline[T]) Execute() (T, error) {
	return p.ExecuteWithContext(context.Background())
}

// Same as Execute, but with context support. Each step receives the context
// in addition to the result of the previous step. Execution stops on the
// first error or if the context is cancelled.
func (p *Pipeline[T]) ExecuteWithContext(ctx context.Context) (T, error) {
	current := p.value
	totalSteps := len(p.steps)

	for i, step := range p.steps {
		select {
		case <-ctx.Done():
			return current, &PipelineError{
				StepIndex:   i,
				TotalSteps:  totalSteps,
				OriginalErr: ctx.Err(),
				LastValue:   current,
			}
		default:
		}

		next, err := step(ctx, current)
		if err != nil {
			return current, &PipelineError{
				StepIndex:   i,
				TotalSteps:  totalSteps,
				OriginalErr: err,
				LastValue:   current,
			}
		}
		current = next
	}
	return current, nil
}
