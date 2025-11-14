# corehsm

`corehsm` is a simple, generic, hierarchical state machine (HSM) library for Go, designed specifically for building non-interactive, stateful CLI applications.

## Overview

This library helps structure a CLI tool around a state machine pattern. It's ideal for applications that need to maintain a consistent state across multiple separate executions. The state is persisted between commands by serializing a "snapshot" of the current machine state (state and data) to a file.

The typical workflow is:
 1. Load a machine snapshot from a file if it exists.
 2. If not, create a new machine with an initial state and data.
 3. Parse a command and its arguments from the command line (e.g., `os.Args`).
 4. Execute the command, which may produce text output and/or transition the HSM to a new state.
 5. Save the new machine snapshot back to the file.

## Design Philosophy: State vs. Data

A core design principle of this library is the separation of "State" and "Data".

- **State**: Represents the *behavior* of the system. A change in state means the system now behaves differently. In this library, a `State` is a simple, concrete struct holding only its name and a reference to its parent.

- **Data**: Represents the *information* or "memory" that the system's behaviors act upon. This is managed in a single, type-safe, user-defined struct passed as a generic parameter.

This distinction keeps handlers clean and focused on a single responsibility, while the state hierarchy explicitly defines the application's modes of operation.

## Installation

```sh
go get github.com/hanpama/corehsm
```

## Example

A minimal counter application:

```go
package main

import (
	"context"
	"fmt"
	"github.com/hanpama/corehsm"
)

// 1. Define a type-safe data structure.
type CounterData struct {
	Count int `json:"count"`
}

// 2. Define states using NewState.
var (
	RootState  = corehsm.NewState("Root", nil)
	ReadyState = corehsm.NewState("Ready", RootState)
)

// 3. Define command handlers.
func increment(ctx context.Context, m *corehsm.Machine[CounterData], cmd *corehsm.Command) (corehsm.Result, error) {
	m.Data.Count++
	return corehsm.Result{Output: fmt.Sprintf("Count is now: %d", m.Data.Count)}, nil
}

func main() {
	// 4. Setup registry and machine.
	registry := corehsm.NewRegistry[CounterData]()
	registry.RegisterState(ReadyState)
	registry.RegisterCommand(ReadyState, corehsm.CommandDef{Name: "inc"}, increment)

	initialData := CounterData{Count: 0}
	m, _ := corehsm.NewMachine(registry, ReadyState, initialData)

	// 5. Execute a command.
	cmd := corehsm.NewCommand("inc")
	output, _ := m.Execute(context.Background(), cmd)
	fmt.Println(output) // Output: Count is now: 1

	// 6. In a real app, you would now save m.GetSnapshot().
}
```
