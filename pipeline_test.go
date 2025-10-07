package pipeline_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lastro-co/pipeline"
)

func TestPipeline(t *testing.T) {
	t.Run("basic string pipeline", func(t *testing.T) {
		result, err := pipeline.
			New("  hello ").
			Do(pipeline.ToStep[string](strings.TrimSpace)).
			Do(pipeline.ToStep[string](strings.ToUpper)).
			Do(pipeline.ToStep[string](func(s string) string { return s + "!" })).
			Do(pipeline.ToStep[string](func(s string) string { return "[" + s + "]" })).
			Execute()

		require.NoError(t, err)
		assert.Equal(t, "[HELLO!]", result)
	})

	t.Run("early error exit", func(t *testing.T) {
		var stepExecuted bool

		failStep := func(s string) (string, error) {
			return "", errors.New("Error")
		}

		shouldNotExecute := func(s string) (string, error) {
			stepExecuted = true
			return s, nil
		}

		result, err := pipeline.
			New(" test").
			Do(pipeline.ToStep[string](strings.TrimSpace)).
			Do(pipeline.ToStep[string](failStep)).
			Do(pipeline.ToStep[string](shouldNotExecute)).
			Execute()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Error")
		assert.Equal(t, "test", result)
		assert.False(t, stepExecuted, "subsequent step should not execute after error")
	})

	t.Run("empty pipeline returns initial value", func(t *testing.T) {
		initial := "initial"
		result, err := pipeline.New(initial).Execute()

		require.NoError(t, err)
		assert.Equal(t, initial, result)
	})
}

func TestToStep(t *testing.T) {
	t.Run("converts func(context.Context, T) (T, error) directly", func(t *testing.T) {
		// This test covers the case where ToStep receives a func(context.Context, T) (T, error)
		// and assigns it directly to step (line 80: step = s)
		contextStepWithError := func(ctx context.Context, str string) (string, error) {
			if str == "error" {
				return "", errors.New("test error")
			}
			return str + "-context", nil
		}

		// Test successful case
		result, err := pipeline.
			New("hello").
			Do(pipeline.ToStep[string](contextStepWithError)).
			Execute()

		require.NoError(t, err)
		assert.Equal(t, "hello-context", result)

		// Test error case
		result, err = pipeline.
			New("error").
			Do(pipeline.ToStep[string](contextStepWithError)).
			Execute()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "test error")
		assert.Equal(t, "error", result)
	})
}

func TestContext(t *testing.T) {
	t.Run("mixed regular and context steps", func(t *testing.T) {
		regularStep := func(str string) (string, error) {
			return strings.ToUpper(str), nil
		}

		contextStep := func(ctx context.Context, str string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
				return str + "!", nil
			}
		}

		result, err := pipeline.
			New("hello").
			Do(pipeline.ToStep[string](regularStep)).
			Do(contextStep).
			ExecuteWithContext(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "HELLO!", result)
	})

	t.Run("context-aware no-error step", func(t *testing.T) {
		contextNoErrStep := func(ctx context.Context, str string) string {
			// Example: append context deadline info if available
			if deadline, ok := ctx.Deadline(); ok {
				return str + "-with-deadline-" + deadline.Format("15:04:05")
			}
			return str + "-no-deadline"
		}

		result, err := pipeline.
			New("test").
			Do(pipeline.ToStep[string](contextNoErrStep)).
			ExecuteWithContext(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "test-no-deadline", result)
	})

	t.Run("context err", func(t *testing.T) {
		slowStep := func(ctx context.Context, str string) (string, error) {
			timer := time.NewTimer(200 * time.Millisecond)
			defer timer.Stop()

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-timer.C:
				return str + "-processed", nil
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := pipeline.
			New("test").
			Do(slowStep).
			ExecuteWithContext(ctx)

		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("context cancelled during pipeline execution", func(t *testing.T) {
		step1 := func(ctx context.Context, str string) (string, error) {
			return str + "-step1", nil
		}

		step2 := func(ctx context.Context, str string) (string, error) {
			// This step should not execute due to context cancellation
			return str + "-step2", nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result, err := pipeline.
			New("test").
			Do(step1).
			Do(step2).
			ExecuteWithContext(ctx)

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, "test", result) // Should return initial value when cancelled

		var pipelineErr *pipeline.PipelineError
		require.ErrorAs(t, err, &pipelineErr)
		assert.Equal(t, 0, pipelineErr.StepIndex)
		assert.Equal(t, 2, pipelineErr.TotalSteps)
		assert.Equal(t, "test", pipelineErr.LastValue)
	})
}
