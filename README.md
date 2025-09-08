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

- A step is `func(T) (T, error)`.
- Build your pipeline with `pipeline.New(initial).Do(step).Do(step).Execute()`.
- Use closures to adapt existing functions that expect more parameters: `Do(func(t T) (T, error) { return fn(t, a, b) })`.
- Use `DoWithoutErr` helper for steps that never return errors, ex: `DoWithoutErr(strings.TrimSpace)`.
- For Context support for logging, timeouts, cancellation, etc. use `DoWithContext/ExecuteWithContext`.

## API Overview

- `type Step[T any] func(T) (T, error)`
- `type ContextStep[T any] func(context.Context, T) (T, error)`
- `func New[T any](initial T) *Pipeline[T]`
- `func (p *Pipeline[T]) Do(step Step[T]) *Pipeline[T]`
- `func (p *Pipeline[T]) DoWithoutErr(step Step[T]) *Pipeline[T]`
- `func (p *Pipeline[T]) DoWithContext(step ContextStep[T]) *Pipeline[T]`
- `func (p *Pipeline[T]) Execute() (T, error)`
- `func (p *Pipeline[T]) ExecuteWithContext(ctx context.Context) (T, error)`

## Examples

### Quick Start

```go
import (
    "fmt"
    "strings"
    "github.com/lastro-co/development-kit/pipeline"
)

// Step functions that match the Step[T] signature
ensureNonEmpty := func(s string) (string, error) {
    if s == "" { return "", fmt.Errorf("empty") }
    return s, nil
}

appendSuffix := func(s, suffix string) string { return s + suffix }

out, err := pipeline.
    New(" hello").
    DoWithoutErr(strings.TrimSpace). // DoWithoutErr simplifies calling functions that return no err
    DoWithoutErr(strings.ToUpper).
    Do(func(s string) (string, error) { return appendSuffix(s, "!"), nil }). // Using closures to adapt functions with more parameters
    Do(ensureNonEmpty).
    Execute()

// out == "HELLO!", err == nil
```

### Adapting functions with multiple parameters

```go
add := func(x, y int) int { return x + y }
multiply := func(x, y int) int { return x * y }

res, err := pipeline.
    New(2).
    Do(func(x int) (int, error) { return add(x, 3), nil }).
    Do(func(x int) (int, error) { return multiply(x, 10), nil }).
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
    Do(func(x int) (int, error) { return add(x, 2), nil }).
    Do(failIfOdd). // errors here
    Do(func(x int) (int, error) { return add(x, 1), nil }). // not executed
    Execute()
```

### Context Support

```go
import "context"

// Function that returns after 100ms
contextStepFunction := func(ctx context.Context, s string) (string, error) {
    select {
    case <-ctx.Done():
        return "", ctx.Err()
    case <-time.After(100 * time.Millisecond):
        return s + "-processed", nil
    }
}

ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
defer cancel()

_, err := pipeline.
    New("data").
    Do(func(s string) (string, error) { return strings.ToUpper(s), nil }). // regular step
    DoWithContext(contextStepFunction).                                    // context step
    ExecuteWithContext(ctx)

// err will be context.DeadlineExceeded due to timeout
```
