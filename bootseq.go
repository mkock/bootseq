package bootseq

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// mode of operation for a sequence: '>' for serial-, ':' for concurrent steps.
type mode rune

// Mode definitions for step execution order.
const (
	serial   mode = '>'
	parallel mode = ':'
)

// calleeDef keeps track of how the callee decided to wait for the sequence to
// finish. Possible values: calleeNone (undefined), calleeWait (Agent.Wait() was
// called) and calleeProg (Agent.Progress() was called).
type calleeDef uint8

const (
	calleeNone calleeDef = iota
	calleeWait
	calleeProg
)

// phase identifies an Agent as any boot sequence will have two; phaseUp for
// bootup sequences and phaseDown for shutdown sequences.
type phase uint8

const (
	phaseUp phase = iota
	phaseDown
)

var (
	// errStepFailure is for error comparisons during testing.
	errStepFailure = errors.New("step has failed")

	// panicServiceLimit triggers when client attempts to add step 65536 to the manager.
	panicServiceLimit = "reached limit of max 65535 services"

	// panicStepLimit triggers when client attempts to add step 256 to any sequence.
	panicStepLimit = "reached limit of max 255 steps per sequence"

	// panicUnknownPhase triggers when calling service.byPhase() with incorrect phase.
	panicUnknownPhase = "unknown phase: must match phaseUp or phaseDown"

	// panicUnknownMode should only trigger if there's an internal library error.
	panicUnknownMode = "unknown mode: failed to boot sequence in serial or parallel mode"

	// panicCallee triggers if client calls both Agent.Wait() and Agent.Progress().
	panicCallee = "invalid callee: you may call Agent.Wait() or Agent.Progress(), not both"

	// panicUp triggers if client calls Agent.Down() while the startup sequence is still running.
	panicUp = "startup sequence is still in progress"

	// panicDown triggers if client calls Agent.Down() twice.
	panicDown = "call to Agent.Down() on agent which is already a shutdown sequence"

	// Various defaults and texts.
	parseErrMsg = "parse error"
)

// Func is the type used for any function that can be executed as a service in
// a boot sequence. Any function that you wish to register and execute as a
// service must satisfy this type.
type Func func() error

// ErrParsingFormula represents a parse problem with the formula to the
// Sequence() method.
type ErrParsingFormula struct {
	message, details string
}

// newParseError is a convenience function for creating a new ErrParsingFormula.
func newParseError(details string) ErrParsingFormula {
	err := ErrParsingFormula{parseErrMsg, details}
	return err
}

// Error satisfies the error interface by returning an error message with parse
// error details.
func (e ErrParsingFormula) Error() string {
	return fmt.Sprintf("%s: %s", e.message, e.details)
}

// A step comprises a sequential slice of sub-steps and a service name which
// acts as a reference to a service in the Manager.srvcs slice.
// Finally, a pointer in each direction to the previous/next step.
type step struct {
	srvc               string
	next, prev, parent *step
	seq                sequence
}

// newStep creates and returns a new step for the service with the given name,
// and initialised to fit n sub-steps.
func newStep(name string) step {
	seq := sequence{}
	seq.mode = serial
	st := step{name, nil, nil, nil, seq}
	seq.parent = &st
	return st
}

// append adds the given step to the end of the sequence of the current step,
// updating pointers as necessary.
func (s *step) append(st step) {
	if s.seq.count == 255 {
		panic(panicStepLimit)
	}

	st.parent = s

	if s.seq.head == nil {
		s.seq.head = &st
		s.seq.tail = &st
		s.seq.count = 1
		return
	}

	tail := s.seq.tail
	s.seq.tail = &st
	tail.next = &st
	st.prev = tail
	s.seq.count++
}

// String draws the sequence diagram from the step and all its sub-steps.
// The returned diagram will always be wrapped in parentheses. No whitespace
// is present in the diagram.
// Ex: "(aaa:(bbb>ccc))"
// Ex: "(aaa>bbb>ccc)"
// Ex: "(aaa)"
func (s step) String() string {
	var out string

	if s.seq.count > 0 {
		names := make([]string, 0, s.seq.count)
		curr := s.seq.head
		for curr != nil {
			names = append(names, curr.String())
			curr = curr.next
		}

		out = strings.Join(names, string(s.seq.mode))
	}

	if out == "" {
		out = s.srvc
	}

	// Outer parens.
	prefix, suffix := "", ""
	if (s.parent == nil && s.srvc != "") || (s.srvc == "" && s.seq.count > 1) {
		prefix, suffix = "(", ")"
	}

	return prefix + out + suffix
}

// Names returns a slice containing all step names contained within the given
// step and each step in its sequence and the sequences of its nested steps.
func (s step) Names() []string {
	if s.seq.count == 0 {
		if s.srvc == "" {
			return []string{}
		}
		return []string{s.srvc}
	}

	names := make([]string, 0, s.seq.count)
	curr := s.seq.head
	for curr != nil {
		names = append(names, curr.Names()...)
		curr = curr.next
	}

	return names
}

// sequence represents a sequence of steps, with the added property that it's
// able to keep track of the head and tail of the chain, as well as the current
// position during traversal. The empty value is immediately usable.
type sequence struct {
	head, tail, curr, parent *step
	mode                     mode
	count                    uint8
}

// first will set the pointer to the current step to point at the head or the
// tail of the sequence, depending on the given phase, and return the step being
// pointed to.
func (s *sequence) first(ph phase) *step {
	switch ph {
	case phaseUp:
		s.curr = s.head
	case phaseDown:
		s.curr = s.tail
	default:
		panic(panicUnknownPhase)
	}
	return s.curr
}

// next will move the pointer to the current step forward or backwards depending
// on the given phase, and return the step being pointed to.
func (s *sequence) next(ph phase) *step {
	switch ph {
	case phaseUp:
		s.curr = s.curr.next
	case phaseDown:
		s.curr = s.curr.prev
	default:
		panic(panicUnknownPhase)
	}
	return s.curr
}

// service contains the functions required in order to execute a single step
// in a sequence, the up() and down() functions, respectively.
type service struct {
	up, down Func
}

// byPhase returns the service function that matches the provided phase.
// It panics if the phase is unknown.
func (s service) byPhase(ph phase) Func {
	switch ph {
	case phaseUp:
		return s.up
	case phaseDown:
		return s.down
	default:
		panic(panicUnknownPhase)
	}
}

// The Progress is communicated on channels returned by methods Up()
// and Down() and provides feedback on the current progress of the boot sequence.
// This includes the name of the service that was last executed, along
// with an optional error if the step failed. err will be nil on success.
type Progress struct {
	Service string
	Err     error
}

// Manager represents a single boot sequence with its own name.
// Actual up/down functions are stored (and referenced) by name in the map
// services.
type Manager struct {
	Name  string
	srvcs map[string]service
}

// New returns a new and uninitialised boot sequence manager.
func New(name string) Manager {
	srvcs := make(map[string]service)
	s := Manager{name, srvcs}
	return s
}

// Add adds a single named service to the boot sequence, with the given "up" and
// "down" functions. If a service with the given name already exists, the provided
// up- and down functions replace those already registered.
func (m Manager) Add(name string, up, down Func) {
	if len(m.srvcs) == 65535 {
		panic(panicServiceLimit)
	}

	m.srvcs[name] = service{up, down}
}

// ServiceCount returns the number of services currently registered with the
// Manager.
func (m Manager) ServiceCount() uint16 {
	return uint16(len(m.srvcs))
}

// ServiceNames returns the name of each registered service, in no
// particular order.
func (m Manager) ServiceNames() []string {
	ns := make([]string, 0, len(m.srvcs))

	for name := range m.srvcs {
		ns = append(ns, name)
	}

	return ns
}

// Sequence takes a formula (see package-level comment)
// and returns an Instance that acts as the main struct for calling Up() and
// keeping track of progress.
func (m Manager) Sequence(form string) (Instance, error) {
	i := Instance{}
	i.mngr = m

	root, err := parse(form)
	if err != nil {
		return i, err
	}

	if err = m.checkNames(root); err != nil {
		return i, err
	}

	i.root = root

	return i, nil
}

// checkNames takes the root step and runs through all child steps in order
// to check if the mentioned service name exists. It returns an appropriate
// ParseError on the first missing/invalid service name.
func (m Manager) checkNames(st step) error {
	if st.srvc != "" {
		if _, ok := m.srvcs[st.srvc]; !ok {
			return newParseError("unknown service: \"" + st.srvc + "\"")
		}
	}

	var err error
	curr := st.seq.head
	for curr != nil {
		if err = m.checkNames(*curr); err != nil {
			return err
		}
		curr = curr.next
	}

	return nil
}

// Instance contains the actual sequence of steps that need to be performed
// during execution of the boot sequence. It also keeps track of progress
// along the way, and provides the Up() method for starting the boot sequence.
type Instance struct {
	mngr Manager
	root step
}

// CountSteps returns the number of steps currently added to the Instance.
// It counts steps recursively, covering all sub-steps.
// The count is for a single sequence (up/down), so you'll need to multiply
// this number by two to cover both sequences.
func (i Instance) CountSteps() uint8 {
	return countRecursively(i.root)
}

// Up executes the startup phase, returning an agent for keeping track of, and
// controlling the execution of the sequence.
func (i Instance) Up(ctx context.Context) *Agent {
	a := newAgent(i)
	go a.exec(ctx)

	return a
}

// Agent represents the execution of a sequence of steps. For any sequence,
// there will be two agents in play: one for the bootup sequence, and another
// for the shutdown sequence. The only difference between these two is the order
// in which the sequence is executed.
// Each agent keeps track of its progress and handles execution of sequence steps.
type Agent struct {
	sync.Mutex               // Controls access to Agent.callee.
	phase      phase         // Current phase: up/down.
	i          Instance      // Ref. to service functions via Instance.
	callee     calleeDef     // Did client call Wait/Progress?
	isDone     bool          // Did sequence execution complete?
	prog       chan Progress // Progress reporting.
}

// newAgent correctly initializes and returns a new agent with the given Instance
// embedded within.
func newAgent(i Instance) *Agent {
	a := Agent{}
	a.i = i
	a.phase = phaseUp
	a.prog = make(chan Progress, i.CountSteps())
	return &a
}

// calleeIs sets the callee to the provided value. Always use this method to
// change callee to avoid data races.
// This method will panic if called more than once.
// It returns true if the callee was successfully changed. It always returns
// false when callee is calleeNone, which is useful.
func (a *Agent) calleeIs(c calleeDef) bool {
	a.Lock()
	defer a.Unlock()
	if c == calleeNone {
		return false
	}
	if a.callee != calleeNone {
		panic(panicCallee)
	}
	a.callee = c
	return true
}

// Progress returns a channel that will receive a Progress struct every time
// a step in the boot sequence has completed. In case of an error, execution
// will stop and no further progress reports will be sent.
// Consequently, there will either be a progress report for each step in the
// sequence, or if execution stops short, the last progress report sent will
// contain an error.
func (a *Agent) Progress() chan Progress {
	a.calleeIs(calleeProg)
	return a.prog
}

// Wait will block until execution of the boot sequence has completed.
// It returns an error if any steps in the sequence failed.
func (a *Agent) Wait() error {
	a.calleeIs(calleeWait)

	for p := range a.prog {
		if p.Err != nil {
			return p.Err
		}
	}

	return nil
}

// Down starts the shutdown sequence. It returns a new agent for controlling
// and monitoring execution of the sequence.
func (a *Agent) Down(ctx context.Context) *Agent {
	if a.phase == phaseDown {
		// Down() has already been called once. Calling it again is a panic.
		panic(panicDown)
	}

	a.Lock()
	if !a.isDone {
		// @TODO: Stop boot process and shutdown from current point in time.
		// But for this initial version, we'll just panic.
		a.Unlock()
		panic(panicUp)
	}
	a.Unlock()

	da := newAgent(a.i)
	da.phase = phaseDown
	go da.exec(ctx)

	return da
}

// report sends the provided message and/or error value on the progress channel
// if, and only if, msg is non-empty and the client has called Wait/Progress.
func (a *Agent) report(msg string, err error) {
	if msg == "" {
		return
	}

	if !a.calleeIs(calleeNone) {
		a.prog <- Progress{msg, err}
	}
}

// exec runs through the sequence step by step and runs the relevant service.
// The standard behavior is to traverse the sequence in chronological order and
// run the "up" function. If Agent.isDownAgent == true, the traversal is instead
// done in reverse order, and the "down" function will run instead.
// After each step has completed, progress is reported on the "prog" channel.
func (a *Agent) exec(ctx context.Context) {
	defer func() {
		a.Lock()
		a.isDone = true
		a.Unlock()
		close(a.prog)
	}()
	_ = a.execStep(ctx, &a.i.root)
	// @TODO: Log errors?
}

// execStep executes a single step. It acts recursively and therefore executes
// every single step in every single sequence, in the correct order.
// A Progress report is sent after execution of each step. If there's an error,
// execution stops and the last Progress report will contain the relevant error.
// In the case of parallel sequences, note that each parallel step will finish
// execution even if one of them reports an error.
func (a *Agent) execStep(ctx context.Context, st *step) (err error) {
	// Check if the context got cancelled.
	select {
	case <-ctx.Done():
		a.report(st.srvc, ctx.Err())
		err = ctx.Err()
		return
	default:
	}

	// Execute the step.
	if st.srvc != "" && st.seq.count == 0 {
		g, _ := errgroup.WithContext(ctx)
		fn := a.i.mngr.srvcs[st.srvc].byPhase(a.phase)
		g.Go(wrapWithReporting(a, st.srvc, fn))
		err = g.Wait()
		return
	}

	// Execute the step sequence.
	switch st.seq.mode {
	case serial:
		for curr := st.seq.first(a.phase); curr != nil && err == nil; curr = st.seq.next(a.phase) {
			err = a.execStep(ctx, curr)
		}
		return
	case parallel:
		g, _ := errgroup.WithContext(ctx)
		for curr := st.seq.first(a.phase); curr != nil; curr = st.seq.next(a.phase) {
			this := curr
			g.Go(func() error {
				return a.execStep(ctx, this)
			})
		}
		err = g.Wait()
	default:
		panic(panicUnknownMode)
	}
	return
}

func unspace(seq string) string {
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllLiteralString(seq, "")
}

// parse treats the given formula as a single group (it will wrap in parenthesis)
// and parse each group recursively until the entire sequence has been parsed.
// An error is returned for empty sequences and illegal
// characters. The returned step contains the entire sequence.
func parse(form string) (step, error) {
	form = unspace(form)
	if form == "" {
		return newStep(""), newParseError("empty sequence")
	}

	return parseFormula([]rune(form))
}

// parseFormula takes a slice of runes that represent a group (ie. it starts and
// ends with parentheses) and returns a step for that formula. If there
// are any sub-groups in the sequence, they are converted recursively into
// sub-steps and added to the sequence. The given group should not
// include the outermost pair of parentheses.
func parseFormula(form []rune) (step, error) {
	var (
		root   = newStep("")
		next   step
		word   = make([]rune, 0, 100)
		parens uint8
	)

	// Starting with seqMode = true, but this can change when we encounter the
	// first symbol (":" or ">") that tells us what kind of step we're
	// dealing with.
	curr := &root
	for _, r := range form {
		switch r {
		case '(':
			curr.append(newStep(""))
			curr = curr.seq.tail
			parens++
		case ')':
			curr = curr.parent
			parens--
		case ':':
			if len(word) > 0 {
				next = newStep(string(word))
				curr.append(next)
				curr.seq.mode = parallel
				word = word[:0]
			}
		case '>':
			if len(word) > 0 {
				next := newStep(string(word))
				curr.append(next)
				curr.seq.mode = serial
				word = word[:0]
			}
		default:
			// Only allow ranges 0-9,a-z,A-Z, underscore and dash.
			if (r < 48 || r > 57) && (r < 65 || r > 90) && (r < 97 || r > 122) && r != 95 && r != 45 {
				return root, newParseError("invalid character(s) in service name")
			}
			word = append(word, r)
		}
	}

	if parens != 0 {
		return root, newParseError("unmatched parenthesis")
	}

	// Handle the last unfinished word if we got one.
	if len(word) > 0 {
		if root.seq.count == 0 {
			// Edge case: replace word into root element.
			root.srvc = string(word)
		} else {
			next := newStep(string(word))
			curr.append(next)
		}
	}

	return root, nil
}

// countRecursively returns the number of steps contained in the given step.
func countRecursively(st step) uint8 {
	var c uint8

	if st.seq.count == 0 {
		return 1
	}

	curr := st.seq.head
	for curr != nil {
		c += countRecursively(*curr)
		curr = curr.next
	}

	return c
}

// wrapWithReporting returns a function that, when called, calls the given
// service function and sends a progress report using the given Agent before
// returning the error (or nil in case of success).
func wrapWithReporting(a *Agent, name string, srvc Func) Func {
	return func() error {
		err := srvc()
		a.report(name, err)
		return err
	}
}

// Noop (no operation) is a convenience function you can use in place of a
// step function for when you want a function that does nothing.
func Noop() error {
	return nil
}

// Errop (error operation) is a convenience function you can use in place of a
// step function for when you want a function that returns an error.
func Errop() error {
	return errStepFailure
}

// Panicop (panic operation) is a convenience function you can use in place of a
// step function for when you want a function that panics.
func Panicop() error {
	panic(errStepFailure.Error())
}

// Sleepop (sleep operation) is a convenience function you can use in place of a
// step function for when you want a function that sleeps for a short while.
func Sleepop() error {
	time.Sleep(250 * time.Millisecond)
	return nil
}
