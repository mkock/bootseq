package bootseq

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// calleeDef keeps track of how the callee decided to wait for the sequence to finish. Possible values: calleeNone
// (undefined), calleeWait (Agent.Wait() was called) and calleeProg (Agent.Progress() was called).
type calleeDef uint8

const (
	calleeNone calleeDef = iota
	calleeWait
	calleeProg
)

// state represents a Manager's state. It's either:
// 1. doing nothing (stateIdle),
// 2. in the startup sequence (stateUp),
// 3. in the shutdown sequence (stateDown).
type state uint8

const (
	stateIdle state = iota
	stateUp
	stateDown
)

// Func is the type used for any function that can be executed as a service in a boot sequence. Any function that you
// wish to register and execute as a service must satisfy this type.
type Func func() error

// Service contains the functions required in order to execute a single Service Func
// in a sequence, the up() and down() functions, respectively.
type Service struct {
	name     string
	priority uint16
	up, down Func
	after    string
}

// After sets the receiver Service to be executed after the one defined by the given name.
func (s *Service) After(name string) {
	s.after = name
}

// byState returns the service function that matches the provided state.
// It panics if the state is unknown.
func (s *Service) byState(ph state) Func {
	switch ph {
	case stateUp:
		return s.up
	case stateDown:
		return s.down
	default:
		panic(panicUnknownState)
	}
}

// Progress is the boot sequence feedback medium.
// Progress is communicated on channels returned by methods Up() and Down() and provides feedback on the current
// progress of the boot sequence. This includes the name of the Service that was last executed, along with an optional
// error if the Service Func failed. Err will be nil on success.
// Progress satisfies the error interface.
type Progress struct {
	Service string
	Err     error
}

// unorderedServices represents a collection of Services before they've been ordered.
type unorderedServices map[string]*Service

// orderedServices represents a collection of Services after they've been ordered.
type orderedServices map[uint16][]Service

// Manager provides registration and storage of boot sequence Services.
// Manager can instantiate an Agent, which is responsible for running the actual startup and shutdown sequences.
type Manager struct {
	sync.Mutex // Protects field services.

	name     string
	services unorderedServices
}

// Agent represents the execution of a sequence of Services. For any sequence, there will be two agents in play: one for
// the startup sequence, and another for the shutdown sequence. The only difference between these two is the order in
// which the sequence is executed.
// Each Agent keeps track of its progress and handles execution of sequence Services.
type Agent struct {
	sync.Mutex             // Controls access to Agent.callee.
	name            string // Name of boot sequence.
	state           state  // Current state: up/down.
	orderedServices orderedServices
	callee          calleeDef     // Did client call Wait/Progress?
	isDone          bool          // Did sequence execution complete?
	progress        chan Progress // Progress reporting.
}

// setPriority looks up the Service with the given name and attempts to set its priority.
// If the Service depends on another, setPriority recursively follows the chain of Services in order to determine
// priorities for the entire chain. setPriority returns the priority that has been resolved for the given Service.
func (u unorderedServices) setPriority(name string) uint16 {
	if name == "" {
		return 0
	}
	service, ok := u[name]
	if !ok {
		panic(fmt.Sprintf("missing Service: %q, was Manager.Validate called?", name))
	}
	if service.priority > 0 {
		return service.priority
	}
	if service.after == "" {
		service.priority = 1
		return 1
	}
	service.priority = u.setPriority(service.after) + 1
	return service.priority
}

// order orders each Service in unorderedServices by priority. order returns the same Services in order of reference.
// The algorithm is:
// 1. Services that don't come after another, receive order 1.
// 2. Services that come immediately after another, receive an order that is one higher than the other.
// 3. If a service refers to another which is unordered, a depth-first approach is taken to resolve the orders
//    of each one.
// order assumes that each referenced service exists.
func (u unorderedServices) order() orderedServices {
	ordered := make(orderedServices, len(u))
	if len(u) == 0 {
		return ordered
	}

	var service *Service
	var priority uint16

	for name := range u {
		priority = u.setPriority(name)
		service = u[name]
		ordered[priority] = append(ordered[priority], *service)
	}

	return ordered
}

// length returns the total number of registered Services.
func (o orderedServices) length() int {
	length := 0

	for _, services := range o {
		length += len(services)
	}

	return length
}

// New returns a new and uninitialised boot sequence Manager.
func New(name string) *Manager {
	services := make(map[string]*Service)
	mgr := Manager{sync.Mutex{}, name, services}
	return &mgr
}

// Register registers a single named Service to the boot sequence, with the given "up" and "down" functions. If a
// Service with the given name already exists, the provided up- and down functions replace those already registered. Add
// returns a pointer to the added Service, that you can call After() on, in order to influence order of execution.
func (m *Manager) Register(name string, up, down Func) *Service {
	m.Lock()
	defer m.Unlock()

	if len(m.services) == 65535 {
		panic(panicServiceLimit)
	}

	ref := &Service{name, 0, up, down, ""}
	m.services[name] = ref
	return ref
}

// ServiceCount returns the number of services currently registered with the
// Manager.
func (m *Manager) ServiceCount() uint16 {
	m.Lock()
	defer m.Unlock()

	return uint16(len(m.services))
}

// ServiceNames returns the name of each registered service, in no
// particular order.
func (m *Manager) ServiceNames() []string {
	m.Lock()
	defer m.Unlock()

	ns := make([]string, 0, len(m.services))

	for name := range m.services {
		ns = append(ns, name)
	}

	return ns
}

// Agent orders the registered services by priority and returns an Agent for controlling the startup and shutdown
// sequences. Agent returns an error if any of the registered Services refer to other Services that are not registered.
func (m *Manager) Agent() (agent *Agent, err error) {
	m.Lock()
	if len(m.services) == 0 {
		err = EmptySequenceError(m.name)
		return
	}
	m.Unlock()
	if err = m.Validate(); err != nil {
		return
	}
	agent = &Agent{}
	agent.name = m.name
	agent.orderedServices = m.services.order()
	return
}

// Validate cycles through each registered service and checks if they refer to other service names that don't exist,
// or if they refer to themselves. Validate returns an error if this is the case, or nil otherwise.
func (m *Manager) Validate() error {
	m.Lock()
	defer m.Unlock()

	if len(m.services) == 0 {
		return EmptySequenceError(m.name)
	}

	for name, srvc := range m.services {
		if srvc.up == nil || srvc.down == nil {
			return NilFuncError(srvc.name)
		}
		if srvc.after == "" {
			continue
		}
		if srvc.after == name {
			return SelfReferenceError(srvc.after)
		}
		prev, ok := m.services[srvc.after]
		if ok {
			if prev.after == srvc.name {
				return CyclicReferenceError(srvc.name)
			}
		} else {
			return UnregisteredServiceError(srvc.after)
		}
	}

	return nil
}

// ServiceCount returns the number of services currently registered with the Agent.
func (a *Agent) ServiceCount() uint16 {
	a.Lock()
	defer a.Unlock()

	return uint16(a.orderedServices.length())
}

// String returns a string representation of the registered Services ordered by priority.
// Service names are wrapped in parentheses, and separated by a colon when it might run concurrently with one or more
// other services, and a right-arrow when it will run before another service.
// Services that have the same priority are sorted alphabetically for reasons of reproducibility.
func (a *Agent) String() string {
	a.Lock()
	defer a.Unlock()

	var sequence strings.Builder

	for i := uint16(1); i <= uint16(len(a.orderedServices)); i++ {
		names := make([]string, len(a.orderedServices[i]))
		for j, service := range a.orderedServices[i] {
			names[j] = service.name
		}
		if len(names) > 1 {
			sort.Strings(names)
		}
		sequence.WriteString("(" + strings.Join(names, " : ") + ") > ")
	}

	ret := sequence.String()
	return ret[:len(ret)-3]
}

// Progress allows the caller to receive progress reports over a channel.
// Progress returns a channel that will receive a Progress struct every time a Service in the boot sequence has
// completed. In case of an error, execution will stop and no further progress reports will be sent. Consequently, there
// will either be a progress report for each Service in the sequence (plus one more to mark the end of execution), or
// if execution stops short, the last progress report sent will contain an error.
func (a *Agent) Progress() (chan Progress, error) {
	a.Lock()
	if a.callee == calleeWait {
		a.Unlock()
		return nil, CalleeError(calleeErrorMessage)
	}
	a.callee = calleeProg
	a.Unlock()

	return a.progress, nil
}

// Wait will block until execution of the boot sequence has completed.
// It returns an error if any Services in the sequence failed.
func (a *Agent) Wait() error {
	a.Lock()
	if a.callee == calleeProg {
		a.Unlock()
		return CalleeError(calleeErrorMessage)
	}
	a.callee = calleeWait
	a.Unlock()

	for p := range a.progress {
		if p.Err != nil {
			return p.Err
		}
	}

	return nil
}

// Up runs the startup sequence.
// Up returns an error if the Agent's current state doesn't allow the sequence to start.
func (a *Agent) Up(ctx context.Context) error {
	a.Lock()
	defer a.Unlock()

	if a.state != stateIdle {
		msg := inProgressErrorMessage
		if a.state == stateDown {
			msg = doneErrorMessage
		}
		return InvalidStateError(msg)
	}

	a.state = stateUp
	a.isDone = false
	a.progress = make(chan Progress, a.orderedServices.length()+1)

	go a.exec(ctx)
	return nil
}

// Down runs the shutdown sequence.
// Down returns an error if the Agent's current state doesn't allow the sequence to start.
func (a *Agent) Down(ctx context.Context) error {
	a.Lock()
	defer a.Unlock()

	if a.state != stateUp || !a.isDone {
		msg := ""
		switch a.state {
		case stateIdle:
			msg = idleErrorMessage
		case stateUp:
			msg = upErrorMessage
		case stateDown:
			msg = inProgressErrorMessage
		}
		return InvalidStateError(msg)
	}

	a.state = stateDown
	a.isDone = false
	a.progress = make(chan Progress, a.orderedServices.length()+1)
	go a.exec(ctx)
	return nil
}

// report sends the provided message and/or error value on the progress channel.
// A message is sent if, and only if, the client has called Wait/Progress.
func (a *Agent) report(progress Progress) {
	a.Lock()
	callee := a.callee
	a.Unlock()

	if callee != calleeNone {
		a.progress <- progress
	}
}

// exec runs through the sequence step by step and runs the relevant Service Func.
// The standard behaviour is to traverse the sequence in chronological order and run the "up" Func. If Agent.state ==
// downState, the traversal is instead done in reverse order, and the "down" Func will run instead. After each Service
// has completed, progress is reported on the "progress" channel.
func (a *Agent) exec(ctx context.Context) {
	var err error
	defer func() {
		a.Lock()
		if err == nil {
			a.isDone = true
		}
		close(a.progress)
		a.Unlock()
	}()

	var (
		current = 0
		step    = 1
		done    = make(chan error)
	)
	if a.state == stateDown {
		current = len(a.orderedServices) + 1
		step = -1
	}

	// Iterate over priority groups. Move in the direction from priority 1..n for startup sequences, and from
	// priority n..1 for shutdown sequences. There is no guarantee regarding order of execution within each
	// priority group. It's possible to interrupt the sequence between each priority group.
	for i := 0; i < len(a.orderedServices); i++ {
		current += step

		go a.execPriority(ctx, uint16(current), done)

		select {
		case <-ctx.Done():
			err = ctx.Err()
			<-done // Wait for execPriority to finish before stopping execution.
			a.report(Progress{Service: "", Err: err})
			return
		case err = <-done:
			if err != nil {
				return
			}
			continue
		}
	}

	a.report(Progress{Service: "", Err: err})
	return
}

// execPriority executes all Services with the same priority/order.
// execPriority creates an errgroup for a single priority level in the Agent's orderedServices slice and runs them.
// execPriority returns an error if any one of the Services in the errgroup failed.
// execPriority is uninterruptible at this level.
func (a *Agent) execPriority(ctx context.Context, priority uint16, done chan<- error) {
	grp, _ := errgroup.WithContext(ctx)

	for _, service := range a.orderedServices[priority] {
		service := service
		grp.Go(func() error {
			err := service.byState(a.state)() // Execute the Service Func.
			a.report(Progress{Service: service.name, Err: err})
			return err
		})
	}

	done <- grp.Wait()
}

// Error returns the error message for the receiver. Error returns an empty string if there is no error.
func (p Progress) Error() string {
	if p.Err == nil {
		return ""
	}
	return p.Err.Error()
}

// NoOp (no operation) is a convenience function you can use in place of a
// Service Func for when you want a function that does nothing.
func NoOp() error {
	return nil
}

// Verify that Progress satisfies the error interface.
var _ error = Progress{}
