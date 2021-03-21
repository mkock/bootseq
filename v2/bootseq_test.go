package bootseq

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestUnorderedServicesSetPriority(t *testing.T) {
	cases := []struct {
		name               string
		services           unorderedServices
		expectedPriorities map[string]uint16
	}{
		{
			"empty case",
			unorderedServices{},
			map[string]uint16{"": 0},
		},
		{
			"base case",
			unorderedServices{"one": {name: "one", after: ""}},
			map[string]uint16{"one": 1},
		},
		{
			"simple case",
			unorderedServices{"one": {name: "one", after: ""}, "two": {name: "two", after: ""}},
			map[string]uint16{"one": 1, "two": 1},
		},
		{
			"stair case",
			unorderedServices{
				"one":   {name: "one", after: ""},
				"two":   {name: "two", after: "one"},
				"three": {name: "two", after: "two"},
				"four":  {name: "two", after: "three"},
				"five":  {name: "two", after: "four"},
				"six":   {name: "two", after: "five"},
			},
			map[string]uint16{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6},
		},
		{
			"even case",
			unorderedServices{
				"one":   {name: "one", after: ""},
				"two":   {name: "two", after: ""},
				"three": {name: "two", after: ""},
				"four":  {name: "two", after: ""},
				"five":  {name: "two", after: ""},
				"six":   {name: "two", after: ""},
			},
			map[string]uint16{"one": 1, "two": 1, "three": 1, "four": 1, "five": 1, "six": 1},
		},
		{
			"mixed case",
			unorderedServices{
				"one":   {name: "one", after: ""},
				"two":   {name: "two", after: "one"},
				"three": {name: "two", after: "two"},
				"four":  {name: "two", after: "two"},
				"five":  {name: "two", after: "four"},
				"six":   {name: "two", after: "five"},
			},
			map[string]uint16{"one": 1, "two": 2, "three": 3, "four": 3, "five": 4, "six": 5},
		},
		{
			"complex case",
			unorderedServices{
				"one":   {name: "one", after: ""},
				"two":   {name: "two", after: ""},
				"three": {name: "two", after: ""},
				"four":  {name: "two", after: "three"},
				"five":  {name: "two", after: "two"},
				"six":   {name: "two", after: "five"},
				"seven": {name: "two", after: "five"},
				"eight": {name: "two", after: "seven"},
				"nine":  {name: "two", after: "eight"},
				"ten":   {name: "two", after: "nine"},
			},
			map[string]uint16{
				"one": 1, "two": 1, "three": 1, "four": 2, "five": 2, "six": 3, "seven": 3, "eight": 4, "nine": 5, "ten": 6,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			for name, expectedPriority := range tt.expectedPriorities {
				actual := tt.services.setPriority(name)
				if actual != expectedPriority {
					t.Fatalf("expected a priority of %d for service %q, got %d", expectedPriority, name, actual)
				}
			}
		})
	}
}

func TestOrderedServicesLength(t *testing.T) {
	cases := []struct {
		name           string
		services       orderedServices
		expectedLength int
	}{
		{
			"empty case",
			orderedServices{},
			0,
		},
		{
			"base case",
			orderedServices{1: []Service{}},
			0,
		},
		{
			"another base case",
			orderedServices{
				1: []Service{},
				2: []Service{},
			},
			0,
		},
		{
			"simple case",
			orderedServices{
				1: []Service{{}},
				2: []Service{{}},
			},
			2,
		},
		{
			"long case",
			orderedServices{
				1: []Service{{}, {}, {}},
				2: []Service{{}, {}},
				3: []Service{{}, {}, {}, {}, {}},
				4: []Service{{}, {}},
				5: []Service{{}, {}},
			},
			14,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.services.length()
			if actual != tt.expectedLength {
				t.Fatalf("expected a length of %d, got %d", tt.expectedLength, actual)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Run("returns a manager with correct name and no services", func(t *testing.T) {
		mgr := New("My Boot Sequence")
		if mgr.name != "My Boot Sequence" {
			t.Fatalf("expected name %q, got %q", "My Boot Sequence", mgr.name)
		}
		verifyCountEq(t, uint32(len(mgr.services)), 0)
	})
}

func TestService(t *testing.T) {
	t.Run("it panics for unknown state arguments", func(t *testing.T) {
		defer verifyPanicWithMsg(t, panicUnknownState)

		s := Service{"", 0, ErrOp, ErrOp, ""}
		fn := s.byState(state(8))
		_ = fn()

		t.Fatal("expected a panic") // Never called if panic is triggered.
	})

	t.Run("it returns the correct function by state", func(t *testing.T) {
		s := Service{"", 0, NoOp, ErrOp, ""}
		fn := s.byState(stateUp)
		err := fn()
		verifyNilErr(t, err)

		fn = s.byState(stateDown)
		err = fn()
		if err == nil || err != errService {
			t.Fatalf("expected down function to return error value %q, got %v", errService, err)
		}
	})

	t.Run("it sets correct reference name", func(t *testing.T) {
		s := Service{"", 0, NoOp, ErrOp, ""}
		s.After("other")
		if s.after != "other" {
			t.Fatalf("expected reference to %q, got %q", "other", s.after)
		}
	})
}

func TestManagerAdd(t *testing.T) {
	t.Run("adds a service with matching name", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		ns := mgr.ServiceNames()
		if len(ns) > 1 || ns[0] != "one" {
			t.Fatalf("expected one service named %q, got %v", "one", ns)
		}
	})

	t.Run("returns service names in correct order", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp)
		mgr.Register("three", NoOp, NoOp)
		mgr.Register("four", NoOp, NoOp)
		mgr.Register("five", NoOp, NoOp)
		actual := mgr.ServiceNames()
		expected := []string{"one", "two", "three", "four", "five"}
		verifyIdenticalSets(t, expected, actual)
	})

	t.Run("re-uses existing service if name is taken", func(t *testing.T) {
		mgr := New("Start")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("one", NoOp, NoOp)
		ns := mgr.ServiceNames()
		if len(ns) != 1 || ns[0] != "one" {
			t.Fatalf("expected one service named %q, got %v", "one", ns)
		}
	})

	t.Run("panics if more than 65535 services are registered", func(t *testing.T) {
		mgr := New("Big one")

		for i := 1; i <= 65535; i++ {
			mgr.Register("Service #"+strconv.Itoa(i), NoOp, NoOp)
		}

		defer verifyPanicWithMsg(t, panicServiceLimit)
		mgr.Register("Service #65536", NoOp, NoOp)

		t.Fatal("expected to panic on the 65536th service")
	})
}

func TestManagerValidate(t *testing.T) {
	t.Run("returns an error for an empty sequence", func(t *testing.T) {
		mgr := New("Empty")
		err := mgr.Validate()
		verifyErrorType(t, err, EmptySequenceError("Empty"))
	})

	t.Run("returns an error for a service with nil Funcs", func(t *testing.T) {
		mgr := New("Invalid #1")
		mgr.Register("oops", nil, NoOp)
		err := mgr.Validate()
		verifyErrorType(t, err, NilFuncError("oops"))
	})

	t.Run("returns an error for a self-referencing service", func(t *testing.T) {
		mgr := New("Invalid #2")
		mgr.Register("selfie", NoOp, NoOp).After("selfie")
		err := mgr.Validate()
		verifyErrorType(t, err, SelfReferenceError("selfie"))
	})

	t.Run("returns an error for an unregistered service", func(t *testing.T) {
		mgr := New("Invalid #3")
		mgr.Register("first_service", NoOp, NoOp)
		mgr.Register("second_service", NoOp, NoOp).After("nobody")
		err := mgr.Validate()
		verifyErrorType(t, err, UnregisteredServiceError("nobody"))
	})

	t.Run("returns an error when there are cyclic references", func(t *testing.T) {
		mgr := New("Very Invalid Boot Sequence")
		mgr.Register("first_service", NoOp, NoOp).After("third_service")
		mgr.Register("second_service", NoOp, NoOp).After("first_service")
		mgr.Register("third_service", NoOp, NoOp).After("first_service")
		err := mgr.Validate()
		if err == nil {
			t.Fatal("expected an error")
		}
		if err.Error() != "cyclic reference: first_service" && err.Error() != "cyclic reference: third_service" {
			t.Fatalf("expected error to equal %q or %q, got %q", "first_service", "third_service", err.Error())
		}
	})

	t.Run("succeeds when registering same service twice", func(t *testing.T) {
		mgr := New("Acceptable Boot Sequence")
		mgr.Register("duplicate_service", NoOp, NoOp)
		mgr.Register("duplicate_service", NoOp, NoOp)
		err := mgr.Validate()
		verifyNilErr(t, err)
	})

	t.Run("succeeds (base case)", func(t *testing.T) {
		mgr := New("My Boot Sequence")
		mgr.Register("first_service", NoOp, NoOp)
		err := mgr.Validate()
		verifyNilErr(t, err)
	})

	t.Run("succeeds (simple case)", func(t *testing.T) {
		mgr := New("My Boot Sequence")
		mgr.Register("first_service", NoOp, NoOp)
		mgr.Register("second_service", NoOp, NoOp)
		mgr.Register("third_service", NoOp, NoOp)
		err := mgr.Validate()
		verifyNilErr(t, err)
	})

	t.Run("succeeds (complex case)", func(t *testing.T) {
		mgr := New("My Boot Sequence")
		mgr.Register("first_service", NoOp, NoOp).After("second_service")
		mgr.Register("second_service", NoOp, NoOp)
		mgr.Register("third_service", NoOp, NoOp).After("second_service")
		mgr.Register("fourth_service", NoOp, NoOp).After("second_service")
		mgr.Register("fifth_service", NoOp, NoOp).After("first_service")
		mgr.Register("sixth_service", NoOp, NoOp).After("first_service")
		mgr.Register("seventh_service", NoOp, NoOp).After("fifth_service")
		mgr.Register("eighth_service", NoOp, NoOp).After("sixth_service")
		mgr.Register("ninth_service", NoOp, NoOp).After("fifth_service")
		mgr.Register("tenth_service", NoOp, NoOp).After("sixth_service")
		err := mgr.Validate()
		verifyNilErr(t, err)
	})
}

func TestManagerServiceCount(t *testing.T) {
	mgr := New("A Boot Sequence")
	mgr.Register("one", NoOp, NoOp)

	verifyCountEq(t, 1, uint32(mgr.ServiceCount()))

	mgr.Register("two", NoOp, NoOp)
	mgr.Register("three", NoOp, NoOp).After("two")

	verifyCountEq(t, 3, uint32(mgr.ServiceCount()))

	mgr.Register("four", NoOp, NoOp).After("three")
	mgr.Register("five", NoOp, NoOp).After("three")

	verifyCountEq(t, 5, uint32(mgr.ServiceCount()))
}

func TestAgentNilFunc(t *testing.T) {
	mgr := New("Nil Func")
	mgr.Register("one", nil, nil)

	agent, err := mgr.Agent()
	if agent != nil {
		t.Fatalf("expected agent to be nil, got %T", agent)
	}
	verifyErrorType(t, err, NilFuncError("one"))
}

func TestAgent(t *testing.T) {
	mgr := New("Dynamic boot sequence")
	mgr.Register("one", NoOp, NoOp)
	mgr.Register("two", NoOp, NoOp)
	mgr.Register("three", NoOp, NoOp)

	updater1 := newIndexUpdater(4)
	updater2 := newIndexUpdater(5)

	// First agent.
	agent, err := mgr.Agent()
	verifyNilErr(t, err)
	err = agent.Up(context.Background(), updater1.progress())
	verifyNilErr(t, err)
	verifyStringsEqual(t, []string{"one", "two", "three", ""}, updater1.actual)

	// Second agent.
	mgr.Register("four", NoOp, NoOp)
	agent, err = mgr.Agent()
	verifyNilErr(t, err)
	err = agent.Up(context.Background(), updater2.progress())
	verifyNilErr(t, err)
	verifyStringsEqual(t, []string{"one", "two", "three", "four", ""}, updater2.actual)
}

func TestAgentServiceCount(t *testing.T) {
	mgr := New("A Boot Sequence")
	mgr.Register("one", NoOp, NoOp)

	agent, err := mgr.Agent()
	verifyNilErr(t, err)
	verifyCountEq(t, 1, uint32(agent.ServiceCount()))

	mgr.Register("two", NoOp, NoOp)
	mgr.Register("three", NoOp, NoOp).After("two")

	agent, err = mgr.Agent()
	verifyNilErr(t, err)
	verifyCountEq(t, 3, uint32(agent.ServiceCount()))

	mgr.Register("four", NoOp, NoOp).After("three")
	mgr.Register("five", NoOp, NoOp).After("three")

	agent, err = mgr.Agent()
	verifyNilErr(t, err)
	verifyCountEq(t, 5, uint32(agent.ServiceCount()))
}

func TestAgentUp(t *testing.T) {
	t.Run("it runs all services", func(t *testing.T) {
		mgr := New("Three-service boot sequence")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp)
		mgr.Register("three", NoOp, NoOp)
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		updater := newIndexUpdater(4)
		err = agent.Up(context.Background(), updater.progress())
		verifyNilErr(t, err)
		verifyStringsEqual(t, []string{"one", "two", "three", ""}, updater.actual)
	})

	t.Run("it runs dependent services in chronological order", func(t *testing.T) {
		mgr := New("Three-service boot sequence")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp).After("one")
		mgr.Register("three", NoOp, NoOp).After("two")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		updater := newIndexUpdater(4)
		err = agent.Up(context.Background(), updater.progress())
		verifyNilErr(t, err)
		orderPreserved := verifyStringsEqual(t, []string{"one", "two", "three", ""}, updater.actual)
		verifyOrderPreserved(t, orderPreserved)
	})

	t.Run("it runs services in chronological order (advanced case)", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, ErrOp).After("one")
		mgr.Register("three", NoOp, NoOp).After("two")
		mgr.Register("four", NoOp, NoOp).After("three")
		mgr.Register("five", ErrOp, NoOp).After("four")  // Fails on fifth "up" Service.
		mgr.Register("six", PanicOp, NoOp).After("five") // PanicOp should never execute.
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		updater := newIndexUpdater(7)

		err = agent.Up(context.Background(), updater.progress())
		verifyErrorType(t, err, errService)
		orderPreserved := verifyStringsEqual(t, []string{"one", "two", "three", "four", "five"}, updater.actual)
		verifyOrderPreserved(t, orderPreserved)
	})

	t.Run("it fails if called while booting up", func(t *testing.T) {
		mgr := New("Three-service boot sequence")
		mgr.Register("one", SleepOp, NoOp)
		mgr.Register("two", SleepOp, NoOp).After("one")
		mgr.Register("three", SleepOp, NoOp).After("two")
		mgr.Register("four", SleepOp, NoOp).After("three")
		mgr.Register("five", SleepOp, NoOp).After("four")
		mgr.Register("six", SleepOp, NoOp).After("five")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		var (
			err1, err2 error
			wg         sync.WaitGroup
		)

		wg.Add(2)
		go func() {
			err1 = agent.Up(context.Background(), nil)
			wg.Done()
		}()

		go func() {
			err2 = agent.Up(context.Background(), nil)
			wg.Done()
		}()

		wg.Wait()
		if err1 == nil && err2 == nil {
			t.Error("expected err1 or err2 to be non-nil")
		}
	})
}

func TestAgentDown(t *testing.T) {
	t.Run("it runs all services", func(t *testing.T) {
		mgr := New("Three-service boot sequence")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp)
		mgr.Register("three", NoOp, NoOp)
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		updater1 := newIndexUpdater(4)
		err = agent.Up(context.Background(), updater1.progress())
		verifyNilErr(t, err)

		updater2 := newIndexUpdater(4)
		err = agent.Down(context.Background(), updater2.progress())
		verifyNilErr(t, err)
		verifyStringsEqual(t, []string{"one", "two", "three", ""}, updater2.actual)
	})

	t.Run("it runs services in reverse chronological order", func(t *testing.T) {
		mgr := New("Three-service boot sequence")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp).After("one")
		mgr.Register("three", NoOp, NoOp).After("two")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		updater1 := newIndexUpdater(4)
		err = agent.Up(context.Background(), updater1.progress())
		verifyNilErr(t, err)

		updater2 := newIndexUpdater(4)
		err = agent.Down(context.Background(), updater2.progress())
		verifyNilErr(t, err)
		orderPreserved := verifyStringsEqual(t, []string{"three", "two", "one", ""}, updater2.actual)
		verifyOrderPreserved(t, orderPreserved)
	})

	t.Run("it runs services in reverse order (advanced case)", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, PanicOp)            // PanicOp should never execute.
		mgr.Register("two", NoOp, ErrOp).After("one") // Fails on fifth Down service.
		mgr.Register("three", NoOp, NoOp).After("two")
		mgr.Register("four", NoOp, NoOp).After("three")
		mgr.Register("five", NoOp, NoOp).After("four")
		mgr.Register("six", NoOp, NoOp).After("five")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		updater1 := newIndexUpdater(7)
		err = agent.Up(context.Background(), updater1.progress())
		verifyNilErr(t, err)

		updater2 := newIndexUpdater(7)
		err = agent.Down(context.Background(), updater2.progress())
		verifyErrorType(t, err, errService)
		orderPreserved := verifyStringsEqual(t, []string{"six", "five", "four", "three", "two"}, updater2.actual)
		verifyOrderPreserved(t, orderPreserved)
	})

	t.Run("it fails if called while booting up", func(t *testing.T) {
		mgr := New("Three-service boot sequence")
		mgr.Register("one", SleepOp, NoOp)
		mgr.Register("two", SleepOp, NoOp)
		mgr.Register("three", SleepOp, NoOp)
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		var (
			err1, err2 error
			wg         sync.WaitGroup
		)

		wg.Add(2)
		func() {
			err1 = agent.Up(context.Background(), nil)
			wg.Done()
		}()

		func() {
			err2 = agent.Up(context.Background(), nil)
			wg.Done()
		}()

		wg.Wait()
		if err1 == nil && err2 == nil {
			t.Error("expected err1 or err2 to be non-nil")
		}
	})
}

func TestAgentCancel(t *testing.T) {
	t.Run("it stops before executing all services", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", SleepOp, NoOp)
		mgr.Register("two", SleepOp, NoOp).After("one")
		mgr.Register("three", SleepOp, NoOp).After("two")
		mgr.Register("four", SleepOp, NoOp).After("three")
		mgr.Register("five", SleepOp, NoOp).After("four")
		mgr.Register("six", PanicOp, NoOp).After("five")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		updater := newIndexUpdater(7)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err = agent.Up(ctx, updater.progress())
			verifyErrorType(t, err, context.Canceled)
			wg.Done()
		}()
		cancel()

		wg.Wait()
		for _, a := range updater.actual {
			if a == "five" {
				// Execution should stop long before reaching the fifth service.
				t.Error("did not expect to encounter service five due to cancellation")
			}
		}
	})
}

func TestAgentTimeout(t *testing.T) {
	t.Run("it stops before executing all services", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", SleepOp, NoOp)
		mgr.Register("two", SleepOp, NoOp).After("one")
		mgr.Register("three", SleepOp, NoOp).After("two")
		mgr.Register("four", SleepOp, NoOp).After("three")
		mgr.Register("five", SleepOp, NoOp).After("four")
		mgr.Register("six", PanicOp, NoOp).After("five")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		updater := newIndexUpdater(7)
		err = agent.Up(ctx, updater.progress())
		verifyErrorType(t, err, context.DeadlineExceeded)

		for _, a := range updater.actual {
			if a == "five" {
				// Execution should stop long before reaching the fifth service.
				t.Fatal("did not expect to encounter service five due to cancellation")
			}
		}
	})
}

func TestAgentString(t *testing.T) {
	t.Run("simple case", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		actual := agent.String()
		expected := "(one)"
		verifyStringEquals(t, expected, actual)
	})

	t.Run("edge case", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("", NoOp, NoOp)
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		actual := agent.String()
		expected := "()"
		verifyStringEquals(t, expected, actual)
	})

	t.Run("nested case", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp).After("one")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		actual := agent.String()
		expected := "(one) > (two)"
		verifyStringEquals(t, expected, actual)
	})

	t.Run("sequential case", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp).After("one")
		mgr.Register("three", NoOp, NoOp).After("two")
		mgr.Register("four", NoOp, NoOp).After("three")
		mgr.Register("five", NoOp, NoOp).After("four")
		mgr.Register("six", NoOp, NoOp).After("five")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		actual := agent.String()
		expected := "(one) > (two) > (three) > (four) > (five) > (six)"
		verifyStringEquals(t, expected, actual)
	})

	t.Run("parallel case", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp)
		mgr.Register("three", NoOp, NoOp)
		mgr.Register("four", NoOp, NoOp)
		mgr.Register("five", NoOp, NoOp)
		mgr.Register("six", NoOp, NoOp)
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		actual := agent.String()
		// expected := "(one : two : three : four : five : six)"
		expected := regexp.MustCompile(`^one|two|three|four|five|six|seven \(\)$`)
		if !expected.MatchString(actual) {
			t.Fatalf("unexpected String response, got %q", actual)
		}
	})

	t.Run("grouped case", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp)
		mgr.Register("three", NoOp, NoOp).After("one")
		mgr.Register("four", NoOp, NoOp).After("one")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		actual := agent.String()
		expected := "(one : two) > (four : three)"
		verifyStringEquals(t, expected, actual)
	})

	t.Run("doubly grouped case", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp)
		mgr.Register("three", NoOp, NoOp).After("one")
		mgr.Register("four", NoOp, NoOp).After("one")
		mgr.Register("five", NoOp, NoOp).After("three")
		mgr.Register("six", NoOp, NoOp).After("three")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		actual := agent.String()
		expected := "(one : two) > (four : three) > (five : six)"
		verifyStringEquals(t, expected, actual)
	})

	t.Run("mixed complex case", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Register("one", NoOp, NoOp)
		mgr.Register("two", NoOp, NoOp)
		mgr.Register("three", NoOp, NoOp).After("one")
		mgr.Register("four", NoOp, NoOp).After("one")
		mgr.Register("five", NoOp, NoOp).After("four")
		mgr.Register("six", NoOp, NoOp).After("five")
		mgr.Register("seven", NoOp, NoOp).After("five")
		mgr.Register("eight", NoOp, NoOp).After("five")
		mgr.Register("nine", NoOp, NoOp).After("one")
		mgr.Register("ten", NoOp, NoOp).After("one")
		agent, err := mgr.Agent()
		verifyNilErr(t, err)

		actual := agent.String()
		expected := "(one : two) > (four : nine : ten : three) > (five) > (eight : seven : six)"
		verifyStringEquals(t, expected, actual)
	})
}
