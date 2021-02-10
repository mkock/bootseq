package bootseq

import "fmt"

const (
	// panicServiceLimit triggers when client attempts to add step 65536 to the manager.
	panicServiceLimit = "reached limit of max 65535 services"

	// panicUnknownState triggers when calling service.byState() with incorrect state.
	panicUnknownState = "unknown state: must match stateUp or stateDown"

	// calleeErrorMessage triggers if client calls both Agent.Wait() and Agent.Progress().
	calleeErrorMessage = "invalid callee: you may call Agent.Wait() or Agent.Progress(), not both"

	// idleErrorMessage triggers when agent.Down is called on an idle Agent.
	idleErrorMessage = "need to start up first"

	// upErrorMessage triggers when agent.Down is called on an Agent that is in process of booting up.
	upErrorMessage = "in process of starting up"

	// inProgressErrorMessage triggers when agent.Up/Down is called on an Agent that is already in progress.
	inProgressErrorMessage = "already in progress"

	// doneErrorMessage triggers when agent.Up is called on an Agent that has already shut down.
	doneErrorMessage = "has already shut down"
)

// EmptySequenceError indicates an empty boot sequence.
type EmptySequenceError string

// Error returns the error message for a EmptySequenceError.
func (e EmptySequenceError) Error() string {
	return fmt.Sprintf("empty boot sequence: %q", string(e))
}

// SelfReferenceError indicates a service that references itself in an After method call.
type SelfReferenceError string

// Error returns the error message for a SelfReferenceError.
func (s SelfReferenceError) Error() string {
	return fmt.Sprintf("self-reference: %q", string(s))
}

// UnregisteredServiceError indicates a service has not been registered with the boot sequence manager.
type UnregisteredServiceError string

// Error returns the error message for a UnregisteredServiceError.
func (u UnregisteredServiceError) Error() string {
	return fmt.Sprintf("no such service: %q", string(u))
}

// InvalidStateError indicates that the Agent was unable to run the boot sequence, either because it is already
// running, or because it has already completed.
type InvalidStateError string

// Error returns the error message for a InvalidStateError.
func (i InvalidStateError) Error() string {
	return fmt.Sprintf("cannot run sequence: %s", string(i))
}

// CyclicReferenceError indicates that two Services are referencing each other.
type CyclicReferenceError string

// Error returns the error message for a CyclicReferenceError.
func (c CyclicReferenceError) Error() string {
	return fmt.Sprintf("cyclic reference: %s", string(c))
}

// CalleeError indicates that a "callee" was called after another one already has been called: Wait/Progress.
type CalleeError string

// Error returns the error message for a CalleeError.
func (c CalleeError) Error() string {
	return string(c)
}

// NilFuncError indicates that a Service contains one or more nil Funcs.
type NilFuncError string

// Error returns the error message for a CalleeError.
func (n NilFuncError) Error() string {
	return fmt.Sprintf("nil Func provided: %s", string(n))
}

// Check that errors satisfy the error interface.
var _ error = EmptySequenceError("")
var _ error = SelfReferenceError("")
var _ error = UnregisteredServiceError("")
var _ error = InvalidStateError("")
var _ error = CyclicReferenceError("")
var _ error = CalleeError("")
var _ error = NilFuncError("")
