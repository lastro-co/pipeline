# pipeline

An idiomatic way to compose a sequence of operations on a value in Go, inspired by Elixir's `|>` pipe operator.

This package lets you express:

- take some data
- first do this
- then do that
- finally, do the following

without repeating error-checking boilerplate at each step or having to nest function calls.

## Why

In plain Go, composing a series of operations often looks like this:

```go
res, err := foo()
if err != nil { /* handle */ }

res, err = bar(res)
if err != nil { /* handle */ }

res, err = baz(res)
if err != nil { /* handle */ }

// or

s := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(x)), "/old/", "/new")

// or even

http.HandleFunc("/",
    RequireAuthMiddleware(
        SomeOtherMiddleware(
            LogMiddleware(IndexHandler))))
```

This not only creates repetition but is also easy to get lost on what is happening once the number of operations increases.

`pipeline` provides a tiny, generic builder that chains functions together. It stops execution on the first error and returns it, enabling a clear, linear flow.

## Core Ideas

- A step is `func(context.Context, T) (T, error)`.
- Build your pipeline with `pipeline.New(initial).Do(step).Do(step).Execute()`.
- Use `ToStep()` helper to convert various function types to pipeline steps.
- Use closures to adapt existing functions that receive multiple parameter: `Do(func(ctx context.Context, t T) (T, error) { return fn(t, a, b), nil })`.

## API Overview

- `type Step[T any] func(context.Context, T) (T, error)`
- `type PipelineError struct`
- `func New[T any](initial T) *Pipeline[T]`
- `func (p *Pipeline[T]) Do(step Step[T]) *Pipeline[T]`
- `func ToStep[T any, F StepFunc[T]](f F) Step[T]`
- `func (p *Pipeline[T]) Execute() (T, error)`
- `func (p *Pipeline[T]) ExecuteWithContext(ctx context.Context) (T, error)`

## Examples

### Quick Start

```go
import (
    "context"
    "fmt"
    "strings"
    "github.com/lastro-co/pipeline"
)

// Step functions that match the Step[T] signature
ensureNonEmpty := func(_ context.Context, s string) (string, error) {
    if s == "" { return "", fmt.Errorf("empty") }
    return s, nil
}

appendSuffix := func(s, suffix string) string { return s + suffix }

out, err := pipeline.
    New(" hello").
    Do(pipeline.ToStep(strings.TrimSpace)). // ToStep converts func(string) string to Step[string]
    Do(pipeline.ToStep(strings.ToUpper)).
    Do(func(_ context.Context, s string) (string, error) { return appendSuffix(s, "!"), nil }). // Direct step function
    Do(ensureNonEmpty). // ensureNonEmpty is already compliant with Step[T] signature
    Execute()

// out == "HELLO!", err == nil
```

### Function Conversion with ToStep

The `ToStep` helper automatically converts various function signatures to pipeline steps:

```go
// Functions that never error
func(T) T                    // strings.TrimSpace, strings.ToUpper, etc.
func(context.Context, T) T   // context-aware functions without errors

// Functions that may error
func(T) (T, error)          // validation functions, parsers, etc.
func(context.Context, T) (T, error)  // already pipeline-ready

// Usage examples:
pipeline.New("  hello  ").
    Do(pipeline.ToStep(strings.TrimSpace)).           // func(string) string
    Do(pipeline.ToStep(strings.ToUpper)).             // func(string) string
    Do(pipeline.ToStep(validateInput)).               // func(string) (string, error)
    Do(contextAwareStep).                             // func(context.Context, string) (string, error) - no conversion needed
    Execute()
```

### Adapting functions with multiple parameters

```go
add := func(x, y int) int { return x + y }
multiply := func(x, y int) int { return x * y }

res, err := pipeline.
    New(2).
    Do(func(ctx context.Context, x int) (int, error) { return add(x, 3), nil }).
    Do(func(ctx context.Context, x int) (int, error) { return multiply(x, 10), nil }).
    Execute()

// res == 50, err == nil
```

### Early Exit on Error

```go
add := func(x, y int) int { return x + y }
failIfOdd := func(x int) (int, error) {
    if x%2 == 1 { return 0, fmt.Errorf("odd") }
    return x, nil
}

_, err := pipeline.
    New(3).
    Do(func(ctx context.Context, x int) (int, error) { return add(x, 2), nil }).
    Do(pipeline.ToStep(failIfOdd)). // errors here
    Do(func(ctx context.Context, x int) (int, error) { return add(x, 1), nil }). // not executed
    Execute()
```

### Context Support

```go
import "context"

// Function that returns after 100ms
slowStep := func(ctx context.Context, s string) (string, error) {
    select {
    case <-ctx.Done():
        return "", ctx.Err()
    case <-time.After(100 * time.Millisecond):
        return s + "-processed", nil
    }
}

// Context will be canceled after 50ms
ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
defer cancel()

_, err := pipeline.
    New("data").
    Do(pipeline.ToStep(strings.ToUpper)).
    Do(slowStep).
    ExecuteWithContext(ctx)

// err will be context.DeadlineExceeded due to timeout
```

### Processing Custom Structs

```go
type User struct {
    Name  string
    Email string
    Age   int
    Role  string
}

// User processing functions
normalizeUser := func(ctx context.Context, u User) (User, error) {
    u.Name = strings.TrimSpace(u.Name)
    u.Email = strings.ToLower(strings.TrimSpace(u.Email))
    return u, nil
}

validateUser := func(ctx context.Context, u User) (User, error) {
    if u.Name == "" {
        return u, fmt.Errorf("name cannot be empty")
    }
    if u.Email == "" {
        return u, fmt.Errorf("email cannot be empty")
    }
    if u.Age < 0 {
        return u, fmt.Errorf("age cannot be negative")
    }
    return u, nil
}

addDefaultDomain := func(ctx context.Context, u User) (User, error) {
    if !strings.Contains(u.Email, "@") {
        u.Email = u.Email + "@company.com"
    }
    return u, nil
}

assignRole := func(ctx context.Context, u User) (User, error) {
    if u.Age >= 18 {
        u.Role = "member"
    } else {
        u.Role = "minor"
    }
    return u, nil
}

// Complete pipeline with User struct
result, err := pipeline.New(User{Name: "  Jane Smith  ", Email: "  JANE.SMITH  ", Age: 28}).
    Do(normalizeUser).
    Do(validateUser).
    Do(addDefaultDomain).
    Do(assignRole).
    Execute()

// result == User{Name: "Jane Smith", Email: "jane.smith@company.com", Age: 28, Role: "member"}
```

### Enhanced Error Information

Pipeline errors provide detailed context about where failures occur:

```go
_, err := pipeline.
    New("test").
    Do(pipeline.ToStep(strings.ToUpper)).
    Do(func(ctx context.Context, s string) (string, error) {
        return "", fmt.Errorf("something went wrong")
    }).
    Do(pipeline.ToStep(strings.TrimSpace)). // not executed
    Execute()

// err is a *PipelineError with:
// PipelineError{
//     StepIndex:   1,                                  // the index of the step that failed
//     TotalSteps:  3,                                  // total steps in the pipeline
//     OriginalErr: fmt.Errorf("something went wrong"), // the original error returned by the step
//     LastValue:   "TEST",                             // the last value processed before the error
// }
```

### Step-Specific Error Handling

Use custom error types to identify which step failed and handle errors differently:

```go
import "errors"

// Custom error types for different steps
type ValidationError struct {
    Field   string
    Message string
}

func (e ValidationError) Error() string {
    return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

type ProcessingError struct {
    Operation string
    Cause     error
}

func (e ProcessingError) Error() string {
    return fmt.Sprintf("processing failed during %s: %v", e.Operation, e.Cause)
}

// Steps that return custom errors
validateData := func(ctx context.Context, data string) (string, error) {
    if len(data) < 3 {
        return "", ValidationError{Field: "data", Message: "too short"}
    }
    return data, nil
}

processData := func(ctx context.Context, data string) (string, error) {
    if strings.Contains(data, "error") {
        return "", ProcessingError{Operation: "transform", Cause: fmt.Errorf("invalid content")}
    }
    return strings.ToUpper(data), nil
}

// Execute pipeline and handle specific errors
result, err := pipeline.New("hi").
    Do(validateData).
    Do(processData).
    Execute()

if err != nil {
    var pipeErr *pipeline.PipelineError
    if errors.As(err, &pipeErr) {
        // Check the underlying error type
        var validationErr ValidationError
        var processingErr ProcessingError

        switch {
        case errors.As(pipeErr.OriginalErr, &validationErr):
            fmt.Printf("Validation failed at step %d: %s\n", pipeErr.StepIndex+1, validationErr.Message)
            // Handle validation errors - maybe retry with different input

        case errors.As(pipeErr.OriginalErr, &processingErr):
            fmt.Printf("Processing failed at step %d during %s\n", pipeErr.StepIndex+1, processingErr.Operation)
            // Handle processing errors - maybe use fallback processing

        default:
            fmt.Printf("Unknown error at step %d: %v\n", pipeErr.StepIndex+1, pipeErr.OriginalErr)
        }
    }
}
```
