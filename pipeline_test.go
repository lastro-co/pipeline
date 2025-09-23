package pipeline_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lastro-co/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
}
