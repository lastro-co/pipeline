// Package pipeline provides a small, type-safe, and idiomatic way to
// compose a sequence of operations (steps) on a value, similar to Elixir's
// pipe operator.
package pipeline

import (
	"context"
)

// Step represents a single transformation over a value of type T.
// It returns the transformed value or an error. If an error occurs,
type Step[T any] func(T) (T, error)

// Same as Step, but with context support.
type ContextStep[T any] func(context.Context, T) (T, error)

// StepFunc defines the constraint for valid step function types
type StepFunc[T any] interface {
	~func(T) (T, error) | ~func(context.Context, T) (T, error) | ~func(T) T | ~func(context.Context, T) T
}

// Pipeline holds a value of type T and a list of steps to apply to it.
// Use New to create a pipeline, Do/DoWithContext to register steps, and
// Execute/ExecuteWithContext to run them.
type Pipeline[T any] struct {
	value T
	steps []ContextStep[T]
}

// New creates a new Pipeline with an initial value.
func New[T any](initial T) *Pipeline[T] {
	return &Pipeline[T]{value: initial}
}

// Do appends a context-aware step to the pipeline. For other function types,
// use the helper function ToStep to convert them.
func (p *Pipeline[T]) Do(step ContextStep[T]) *Pipeline[T] {
	p.steps = append(p.steps, step)
	return p
}

// ToStep converts various function types to ContextStep[T] for use with Do.
// It accepts functions of type:
// - func(T) T: Functions that never return an error
// - func(T) (T, error): Functions that may return an error
// - func(context.Context, T) T: Context-aware functions that never return an error
// - func(context.Context, T) (T, error): Functions that are already context-aware
func ToStep[T any, F StepFunc[T]](f F) ContextStep[T] {
	var step ContextStep[T]

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

	for _, step := range p.steps {
		select {
		case <-ctx.Done():
			return current, ctx.Err()
		default:
		}

		next, err := step(ctx, current)
		if err != nil {
			return current, err
		}
		current = next
	}
	return current, nil
}
