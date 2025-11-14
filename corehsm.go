// Package corehsm provides a simple, generic, hierarchical state machine (HSM)
// designed specifically for building non-interactive, stateful CLI applications.
//
// # Overview
//
// This library helps structure a CLI tool around a state machine pattern. It's
// ideal for applications that need to maintain a consistent state across multiple
// separate executions. The state is persisted between commands by serializing a
// "snapshot" of the current context (state and data) to a file.
//
// # Design Philosophy: State vs. Data
//
// A core design principle of this library is the separation of "State" and "Data".
//
//   - State: Represents the *behavior* of the system. A change in state means
//     the system now behaves differently. A State is an immutable, concrete
//     struct created via NewState, holding only its name and a reference to its
//     parent.
//
//   - Data: Represents the *information* or "memory" that the system's behaviors
//     act upon. This is managed in a single, type-safe, user-defined struct.
//
// # Example
//
//	package main
//
//	import (
//		"github.com/hanpama/corehsm"
//		"fmt"
//	)
//
//	type CounterData struct { Count int `json:"count"` }
//
//	var (
//		RootState  = corehsm.NewState("Root", nil)
//		ReadyState = corehsm.NewState("Ready", RootState)
//	)
//
//	func increment(ctx context.Context, m *corehsm.Machine[CounterData], cmd *corehsm.Command) (corehsm.Result, error) {
//		m.Data.Count++
//		return corehsm.Result{Output: fmt.Sprintf("Count is now: %d", m.Data.Count)}, nil
//	}
//
//	func main() {
//		registry := corehsm.NewRegistry[CounterData]()
//		registry.RegisterState(ReadyState)
//		registry.RegisterCommand(ReadyState, corehsm.CommandDef{Name: "inc"}, increment)
//		// ...
//	}
package corehsm

import (
	"context"
	"fmt"
	"sort"
)

// State represents a node in the state hierarchy. It is an immutable, concrete
// struct that acts as a stateless marker, defining a mode of behavior for the
// system. It should be created once via NewState and then referenced.
type State struct {
	name   string
	parent *State
}

// NewState creates a new, immutable State instance.
func NewState(name string, parent *State) *State {
	return &State{name: name, parent: parent}
}

// Name returns the unique name of the state.
func (s *State) Name() string { return s.name }

// Parent returns the parent state, or nil if it is a root state.
func (s *State) Parent() *State { return s.parent }

// Command represents an instruction to be executed by the state machine.
// It is an immutable struct created via NewCommand.
type Command struct {
	name string
	args []string
}

// NewCommand creates a new, immutable Command instance.
func NewCommand(name string, args ...string) *Command {
	return &Command{name: name, args: args}
}

// Name returns the command's name, used to look up the handler.
func (c *Command) Name() string { return c.name }

// Args returns the list of arguments for the command.
func (c *Command) Args() []string { return c.args }

// CommandDef holds metadata about a command, such as its arguments and a
// description. This is primarily used for generating help text or usage
// information.
type CommandDef struct {
	Name        string
	Args        string
	Description string
}

// Result encapsulates the outcome of a command handler's execution.
type Result struct {
	Output    string
	NextState *State
}

// CommandHandlerFunc is the signature for a function that implements a command's
// logic. It receives a context, a type-safe machine, and the command to be executed.
type CommandHandlerFunc[T any] func(ctx context.Context, m *Machine[T], cmd *Command) (Result, error)

// Snapshot is a serializable representation of the HSM's state.
type Snapshot[T any] struct {
	CurrentStateName string `json:"currentStateName"`
	Data             T      `json:"data"`
}

// RegisteredCommand is an internal struct that pairs a command's metadata
// with its handler function.
type RegisteredCommand[T any] struct {
	Def     CommandDef
	Handler CommandHandlerFunc[T]
}

// Registry is the central hub where states and their associated command handlers
// are registered. It acts as a blueprint for the state machine's behavior.
type Registry[T any] struct {
	states          map[string]*State
	commandHandlers map[string]map[string]RegisteredCommand[T]
}

// NewRegistry creates a new, empty registry for a given data type T.
func NewRegistry[T any]() *Registry[T] {
	return &Registry[T]{
		states:          make(map[string]*State),
		commandHandlers: make(map[string]map[string]RegisteredCommand[T]),
	}
}

// RegisterState adds a state and all its parent states to the registry.
func (r *Registry[T]) RegisterState(state *State) {
	if state == nil {
		return
	}
	if _, ok := r.states[state.Name()]; ok {
		return
	}
	r.states[state.Name()] = state
	r.RegisterState(state.Parent())
}

// RegisterCommand associates a command handler and its definition with a
// specific state.
func (r *Registry[T]) RegisterCommand(state *State, def CommandDef, handler CommandHandlerFunc[T]) {
	stateName := state.Name()
	if _, ok := r.commandHandlers[stateName]; !ok {
		r.commandHandlers[stateName] = make(map[string]RegisteredCommand[T])
	}
	r.commandHandlers[stateName][def.Name] = RegisteredCommand[T]{Def: def, Handler: handler}
}

// GetStateByName retrieves a registered State instance by its name.
func (r *Registry[T]) GetStateByName(name string) (*State, bool) {
	s, ok := r.states[name]
	return s, ok
}

// findCommandHandler searches for a command handler by traversing up the state
// hierarchy from the current state.
func (r *Registry[T]) findCommandHandler(state *State, cmdName string) (CommandHandlerFunc[T], bool) {
	for s := state; s != nil; s = s.Parent() {
		if stateHandlers, ok := r.commandHandlers[s.Name()]; ok {
			if cmd, ok := stateHandlers[cmdName]; ok {
				return cmd.Handler, true
			}
		}
	}
	return nil, false
}

// FindAvailableCommands collects all command definitions that are available in
// the given state and its parent states.
func (r *Registry[T]) FindAvailableCommands(state *State) []CommandDef {
	seen := make(map[string]bool)
	var commands []CommandDef

	for s := state; s != nil; s = s.Parent() {
		if stateHandlers, ok := r.commandHandlers[s.Name()]; ok {
			for cmdName, regCmd := range stateHandlers {
				if !seen[cmdName] {
					seen[cmdName] = true
					commands = append(commands, regCmd.Def)
				}
			}
		}
	}

	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	return commands
}

// buildStatePath constructs the full path of states from the root to a given
// target state.
func buildStatePath(targetState *State) []*State {
	var path []*State
	for s := targetState; s != nil; s = s.Parent() {
		path = append(path, s)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// Machine is the runtime engine of the HSM. It manages the current state, the
// state hierarchy stack, and the application's type-safe data.
type Machine[T any] struct {
	CurrentState *State
	StateStack   []*State
	Data         T
	registry     *Registry[T]
}

// NewMachine creates a new runtime machine.
func NewMachine[T any](registry *Registry[T], initialState *State, initialData T) (*Machine[T], error) {
	m := &Machine[T]{
		Data:     initialData,
		registry: registry,
	}
	m.TransitionTo(initialState)
	return m, nil
}

// NewMachineFromSnapshot restores a runtime machine from a snapshot.
func NewMachineFromSnapshot[T any](registry *Registry[T], snapshot *Snapshot[T]) (*Machine[T], error) {
	initialState, ok := registry.GetStateByName(snapshot.CurrentStateName)
	if !ok {
		return nil, fmt.Errorf("state '%s' not found in registry", snapshot.CurrentStateName)
	}

	m := &Machine[T]{
		Data:     snapshot.Data,
		registry: registry,
	}
	m.StateStack = buildStatePath(initialState)
	m.CurrentState = initialState

	return m, nil
}

// Execute finds and executes the appropriate handler for the given command.
func (m *Machine[T]) Execute(ctx context.Context, cmd *Command) (string, error) {
	handler, found := m.registry.findCommandHandler(m.CurrentState, cmd.Name())
	if !found {
		return "", fmt.Errorf("command '%s' not available in state '%s'", cmd.Name(), m.CurrentState.Name())
	}

	result, err := handler(ctx, m, cmd)
	if err != nil {
		return result.Output, err
	}

	if result.NextState != nil && result.NextState.Name() != m.CurrentState.Name() {
		m.TransitionTo(result.NextState)
	}

	return result.Output, nil
}

// TransitionTo switches the HSM to a new state.
func (m *Machine[T]) TransitionTo(newState *State) {
	m.StateStack = buildStatePath(newState)
	m.CurrentState = newState
}

// GetSnapshot creates a serializable snapshot of the current machine.
func (m *Machine[T]) GetSnapshot() *Snapshot[T] {
	return &Snapshot[T]{
		CurrentStateName: m.CurrentState.Name(),
		Data:             m.Data,
	}
}

// Registry returns a read-only reference to the registry.
func (m *Machine[T]) Registry() *Registry[T] {
	return m.registry
}