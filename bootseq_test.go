package bootseq

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("returns a manager with correct name and no services", func(t *testing.T) {
		mgr := New("My Boot Sequence")
		if mgr.Name != "My Boot Sequence" {
			t.Fatalf("expected name %q, got %q", "My Boot Sequence", mgr.Name)
		}
		verifyCountEq(t, uint32(mgr.ServiceCount()), 0)
	})
}

func TestService(t *testing.T) {
	t.Run("it panics for unknown phase arguments", func(t *testing.T) {
		defer verifyPanicWithMsg(t, panicUnknownPhase)

		s := service{Errop, Errop}
		fn := s.byPhase(phase(8))
		_ = fn()

		t.Fatal("expected a panic") // Never called if panic is triggered.
	})

	t.Run("it returns the correct function by phase", func(t *testing.T) {
		s := service{Noop, Errop}
		fn := s.byPhase(phaseUp)
		err := fn()
		verifyNilErr(t, err)

		fn = s.byPhase(phaseDown)
		err = fn()
		if err == nil || err != errStepFailure {
			t.Fatalf("expected down function to return error value %q, got %v", errStepFailure, err)
		}
	})
}

func TestStep(t *testing.T) {
	t.Run("it resets to the correct step", func(t *testing.T) {
		s := newStep("test")
		s.append(newStep("one"))
		s.append(newStep("two"))
		s.append(newStep("three"))
		s.seq.mode = serial
		f := s.seq.first(phaseUp)
		if f.srvc != "one" {
			t.Fatalf("expected step.head to point at step with service name %q, got %q", "one", s.seq.head.srvc)
		}

		f = s.seq.first(phaseDown)
		if f.srvc != "three" {
			t.Fatalf("expected step.head to point at step with service name %q, got %q", "three", s.seq.head.srvc)
		}
	})

	t.Run("it returns the correct step on calls to next", func(t *testing.T) {
		s := newStep("test")
		s.append(newStep("one"))
		s.append(newStep("two"))
		s.append(newStep("three"))
		s.seq.mode = serial

		names := make([]string, 0, 3)
		for curr := s.seq.first(phaseUp); curr != nil; curr = s.seq.next(phaseUp) {
			names = append(names, curr.srvc)
		}
		actual := strings.Join(names, ",")
		expected := "one,two,three"
		if actual != expected {
			t.Fatalf("expected sequence.next() to result in %q, got %q", expected, actual)
		}
	})

	t.Run("it tracks the correct number of steps", func(t *testing.T) {
		s := newStep("test")
		s.append(newStep("one"))
		s.append(newStep("two"))
		s.append(newStep("three"))

		actual := s.seq.count
		expected := uint8(3)
		if expected != actual {
			t.Fatalf("expected sequence to track %q steps, got %q", expected, actual)
		}
	})

	t.Run("it panics when appending 256 steps", func(t *testing.T) {
		s := newStep("test")
		for i := 1; i <= 255; i++ {
			s.append(newStep("Step #" + strconv.Itoa(i)))
		}

		defer verifyPanicWithMsg(t, panicStepLimit)
		s.append(newStep("Step #256"))

		t.Fatal("expected to panic before the 256th step")
	})
}

func TestManager_Add(t *testing.T) {
	t.Run("adds a service with matching name", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Add("one", Noop, Noop)
		ns := mgr.ServiceNames()
		if len(ns) > 1 || ns[0] != "one" {
			t.Fatalf("expected one service named %q, got %v", "one", ns)
		}
	})

	t.Run("returns service names in correct order", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		mgr.Add("four", Noop, Noop)
		mgr.Add("five", Noop, Noop)
		actual := mgr.ServiceNames()
		expected := []string{"one", "two", "three", "four", "five"}
		verifyIdenticalSets(t, expected, actual)
	})

	t.Run("re-uses existing service if name is taken", func(t *testing.T) {
		mgr := New("Start")
		mgr.Add("one", Noop, Noop)
		mgr.Add("one", Noop, Noop)
		ns := mgr.ServiceNames()
		if len(ns) != 1 || ns[0] != "one" {
			t.Fatalf("expected one step named %q, got %v", "one", ns)
		}
	})

	t.Run("panics if more than 65535 services are registered", func(t *testing.T) {
		mgr := New("Big one")

		for i := 1; i <= 65535; i++ {
			mgr.Add("Service #"+strconv.Itoa(i), Noop, Noop)
		}

		defer verifyPanicWithMsg(t, panicServiceLimit)
		mgr.Add("Service #65536", Noop, Noop)

		t.Fatal("expected to panic on the 65536th service")
	})
}

func TestManager_Sequence(t *testing.T) {
	t.Run("returns an error for an empty sequence", func(t *testing.T) {
		mgr := New("Empty")
		_, err := mgr.Sequence("")
		verifyParseError(t, err, "empty sequence")
	})

	t.Run("returns an ErrParsingFormula error for an invalid sequence", func(t *testing.T) {
		mgr := New("Invalid #1")
		_, err := mgr.Sequence("invalid")
		verifyParseError(t, err, "unknown service: \"invalid\"")
	})

	t.Run("returns an ErrParsingFormula error for a buried invalid sequence", func(t *testing.T) {
		mgr := New("Invalid #2")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		_, err := mgr.Sequence("one>two>(three:four)")
		verifyParseError(t, err, "unknown service: \"three\"")
	})

	t.Run("handles unmatched opening parenthesis", func(t *testing.T) {
		mgr := New("Invalid #3")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		_, err := mgr.Sequence("one>(two:three")
		verifyParseError(t, err, "parse error: unmatched parenthesis")
	})

	t.Run("handles unmatched closing parenthesis", func(t *testing.T) {
		mgr := New("Invalid #3")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		_, err := mgr.Sequence("one>(two:three))")
		verifyParseError(t, err, "parse error: unmatched parenthesis")
	})

	t.Run("calls repeated service names the correct number of times", func(t *testing.T) {
		var called uint8
		incop := func() error {
			called++
			return nil
		}
		mgr := New("Invalid #4")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", incop, incop)
		mgr.Add("three", Noop, Noop)
		i, err := mgr.Sequence("one>(two:two)>three")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		if err = up.Wait(); err != nil {
			t.Fatalf("failed waiting for bootup sequence: %s", err.Error())
		}

		expected := uint8(2)
		if called != expected {
			t.Fatalf("expected step two to increment the counter to %d, got %d", expected, called)
		}
	})

	t.Run("can be called more than once", func(t *testing.T) {
		mgr := New("Invalid #5")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		_, err := mgr.Sequence("one>(two:##)")
		verifyParseError(t, err, "parse error: invalid character(s) in service name")
		i, err := mgr.Sequence("one>(two:three)")
		verifyNilErr(t, err)
		expected := uint8(3)
		actual := i.CountSteps()
		if actual != expected {
			t.Fatalf("expected %d steps, got %d", expected, actual)
		}
	})
}

func TestInstance_CountSteps(t *testing.T) {
	t.Run("returns the correct step count (simple case)", func(t *testing.T) {
		mgr := New("Count Test Simple")
		mgr.Add("one", Noop, Noop)
		i, err := mgr.Sequence("one")
		verifyNilErr(t, err)
		verifyCountEq(t, uint32(i.CountSteps()), 1)
	})

	t.Run("returns the correct step count for a valid sequence using brackets", func(t *testing.T) {
		mgr := New("Count Test Brackets")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)

		i, err := mgr.Sequence("one > two > three")
		verifyNilErr(t, err)

		c := uint32(i.CountSteps())
		verifyCountEq(t, c, 3)
	})

	t.Run("returns the correct step count for a valid sequence using colons", func(t *testing.T) {
		mgr := New("Count Test Colons")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)

		i, err := mgr.Sequence("one : two : three")
		verifyNilErr(t, err)

		c := uint32(i.CountSteps())
		verifyCountEq(t, c, 3)
	})

	t.Run("returns the correct step count for repeated steps", func(t *testing.T) {
		mgr := New("Count Test Repeated")
		mgr.Add("one", Noop, Noop)
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)

		i, err := mgr.Sequence("one > two")
		verifyNilErr(t, err)

		c := uint32(i.CountSteps())
		verifyCountEq(t, c, 2)
	})

	t.Run("returns the correct step count for grouped steps", func(t *testing.T) {
		mgr := New("Count Test Groups")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		mgr.Add("four", Noop, Noop)

		i, err := mgr.Sequence("(one : two) > (three : four)")
		verifyNilErr(t, err)

		c := uint32(i.CountSteps())
		verifyCountEq(t, c, 4)
	})
}

func TestAgent_Up(t *testing.T) {
	t.Run("it returns a channel with capacity matching step count", func(t *testing.T) {
		mgr := New("Three-step boot sequence")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		i, err := mgr.Sequence("one > two > three")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		verifyChannelCap(t, up.Progress(), 3)
	})

	t.Run("it runs steps in chronological order", func(t *testing.T) {
		mgr := New("Three-step boot sequence")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		i, err := mgr.Sequence("one > two > three")
		verifyNilErr(t, err)

		up := i.Up(context.Background())

		pp := up.Progress()
		names := make([]string, 0, 3)
		for p := range pp {
			msg := p.Service
			if p.Err != nil {
				msg = p.Err.Error()
			}
			names = append(names, msg)
		}
		actual := strings.Join(names, ",")
		expected := "one,two,three"
		if actual != expected {
			t.Fatalf("expected Agent.Up() to result in %q, got %q", expected, actual)
		}
	})
}

func TestAgent_Down(t *testing.T) {
	t.Run("returns channel with capacity matching step count", func(t *testing.T) {
		mgr := New("Three-step boot sequence")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		i, err := mgr.Sequence("one > two > three")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		_ = up.Wait()

		down := up.Down(context.Background())

		p := down.Progress()
		verifyChannelCap(t, p, 3)
	})

	t.Run("it runs steps in reverse order", func(t *testing.T) {
		mgr := New("Three-step boot sequence")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		i, err := mgr.Sequence("one > two > three")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		_ = up.Wait()

		down := up.Down(context.Background())

		pp := down.Progress()
		names := make([]string, 0, 3)
		for p := range pp {
			msg := p.Service
			if p.Err != nil {
				msg = p.Err.Error()
			}
			names = append(names, msg)
		}
		actual := strings.Join(names, ",")
		expected := "three,two,one"
		if actual != expected {
			t.Fatalf("expected Agent.Down() to result in %q, got %q", expected, actual)
		}
	})

	t.Run("it runs steps in reverse order (advanced case)", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Add("one", Noop, Panicop) // Panicop should never execute.
		mgr.Add("two", Noop, Errop)   // Fails on fifth Down step.
		mgr.Add("three", Noop, Noop)
		mgr.Add("four", Noop, Noop)
		mgr.Add("five", Noop, Noop)
		mgr.Add("six", Noop, Noop)
		i, err := mgr.Sequence("one > two > (three : four : five) > six")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		_ = up.Wait()

		down := up.Down(context.Background())

		pp := down.Progress()
		actual := make([]string, 0, 5)
		for p := range pp {
			msg := p.Service
			if p.Err != nil {
				msg = p.Err.Error()
			}
			actual = append(actual, msg)
		}

		expected := []string{errStepFailure.Error(), "three", "four", "five", "six"}
		verifyStringSlicesEqual(t, expected, actual)
	})

	t.Run("it panics if called while booting up", func(t *testing.T) {
		mgr := New("Three-step boot sequence")
		mgr.Add("one", Sleepop, Noop)
		mgr.Add("two", Sleepop, Noop)
		mgr.Add("three", Sleepop, Noop)
		i, err := mgr.Sequence("one > two > three")
		verifyNilErr(t, err)

		up := i.Up(context.Background())

		defer verifyPanicWithMsg(t, panicUp)
		_ = up.Down(context.Background())
		t.Fatal("expected to panic")
	})
}

func TestAgent_Cancel(t *testing.T) {
	t.Run("it stops before executing all steps", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Add("one", Sleepop, Noop)
		mgr.Add("two", Sleepop, Noop)
		mgr.Add("three", Sleepop, Noop)
		mgr.Add("four", Sleepop, Noop)
		mgr.Add("five", Sleepop, Noop)
		mgr.Add("six", Panicop, Noop)
		i, err := mgr.Sequence("one > two > (three : four : five) > six")
		verifyNilErr(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		up := i.Up(ctx)

		cancel()

		for p := range up.Progress() {
			if p.Service == "five" {
				// Execution should stop long before reaching the fifth step.
				t.Fatal("did not expect to encounter step five due to cancellation")
			}
		}
	})
}

func TestUnspace(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"one two three": "onetwothree",
		"one > two":     "one>two",
		"one	>\n two": "one>two",
		"one  :two (three)":             "one:two(three)",
		"one  :two (three > f_o_u_r  )": "one:two(three>f_o_u_r)",
		"123æøå>>:":                     "123æøå>>:",
	}

	var out string
	for in, expected := range cases {
		out = unspace(in)

		if out != expected {
			t.Fatalf("expected unspace(%q) to match %q, got %q", in, expected, out)
		}
	}
}

func TestParseFormula(t *testing.T) {
	t.Run("it returns a child-less step for the base case", func(t *testing.T) {
		st, err := parseFormula([]rune("one"))

		verifyNilErr(t, err)
		if st.seq.count > 0 {
			t.Fatalf("expected one step with %d children, got %d children", 0, st.seq.count)
		}
	})

	t.Run("it returns steps with correct parent refs", func(t *testing.T) {
		st, err := parseFormula([]rune("(one>two)"))

		verifyNilErr(t, err)
		if st.parent != nil {
			t.Error("expected root step to have parent == nil")
		}
		if st.seq.head.parent == nil {
			t.Error("expected head of sequence to point at root step")
		}
		if st.seq.tail.parent == nil {
			t.Error("expected head of sequence to point at root step")
		}
	})

	t.Run("it returns an error for invalid characters", func(t *testing.T) {
		_, err := parseFormula([]rune("o=ne>t#wo"))
		verifyParseError(t, err, "invalid character(s) in service name")
	})

	t.Run("it allows underscore, dash and digits", func(t *testing.T) {
		st, err := parseFormula([]rune("one>tw_o>3>fo-ur"))

		verifyNilErr(t, err)
		if st.seq.count != 4 {
			t.Errorf("expected sequence with four steps, got %d", st.seq.count)
		}
		if st.seq.head.srvc != "one" {
			t.Errorf("expected first step name to be %q, got %q", "one", st.seq.head.srvc)
		}
		if st.seq.head.next.srvc != "tw_o" {
			t.Errorf("expected second step name to be %q, got %q", "tw_o", st.seq.head.next.srvc)
		}
		if st.seq.head.next.next.srvc != "3" {
			t.Errorf("expected third step name to be %q, got %q", "3", st.seq.head.next.next.srvc)
		}
		if st.seq.tail.srvc != "fo-ur" {
			t.Errorf("expected fourth step name to be %q, got %q", "fo-ur", st.seq.tail.srvc)
		}
	})
}

func TestStepString(t *testing.T) {
	t.Run("simple case", func(t *testing.T) {
		st := newStep("aaa")

		actual := st.String()
		expected := "(aaa)"
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	})

	t.Run("edge case", func(t *testing.T) {
		st := newStep("")

		actual := st.String()
		expected := ""
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	})

	t.Run("nested case", func(t *testing.T) {
		st := newStepPtr("")
		st.append(newStep(""))
		st.seq.head.append(newStep(""))
		actual := st.String()
		expected := ""
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	})

	t.Run("sequential case", func(t *testing.T) {
		st := newStepPtr("")
		st.append(newStep("aaa"))
		st.append(newStep("bbb"))
		st.append(newStep("ccc"))
		st.append(newStep("ddd"))
		st.append(newStep("eee"))
		st.seq.mode = serial

		actual := st.String()
		expected := "(aaa>bbb>ccc>ddd>eee)"
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	})

	t.Run("parallel case", func(t *testing.T) {
		st := newStepPtr("")
		st.append(newStep("aaa"))
		st.append(newStep("bbb"))
		st.append(newStep("ccc"))
		st.append(newStep("ddd"))
		st.append(newStep("eee"))
		st.seq.mode = parallel

		actual := st.String()
		expected := "(aaa:bbb:ccc:ddd:eee)"
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	})

	t.Run("grouped case", func(t *testing.T) {
		st := newStepPtr("")
		st.append(newStep("aaa"))
		st.append(newStep("bbb"))
		st.seq.mode = parallel

		actual := st.String()
		expected := "(aaa:bbb)"
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	})

	t.Run("doubly grouped case", func(t *testing.T) {
		st := newStepPtr("")
		st.append(newStep(""))
		st.append(newStep(""))
		st.seq.mode = serial

		st.seq.head.append(newStep("aaa"))
		st.seq.head.append(newStep("bbb"))
		st.seq.head.seq.mode = parallel

		st.seq.tail.append(newStep("ccc"))
		st.seq.tail.append(newStep("ddd"))
		st.seq.tail.seq.mode = parallel

		actual := st.String()
		expected := "((aaa:bbb)>(ccc:ddd))"
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	})

	t.Run("mixed serial/parallel case", func(t *testing.T) {
		st := newStepPtr("")
		st.append(newStep("aaa"))
		st.append(newStep("bbb"))
		st.append(newStep("ccc"))
		st.append(newStep(""))
		st.seq.tail.append(newStep("ddd"))
		st.seq.tail.append(newStep("eee"))
		st.seq.tail.append(newStep("fff"))
		st.seq.mode = parallel

		actual := st.String()
		expected := "(aaa:bbb:ccc:(ddd>eee>fff))"
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	})
}

func TestAgent_Panics(t *testing.T) {
	t.Run("panics when Agent.Wait() is called after Agent.Progress()", func(t *testing.T) {
		mgr := New("Single-step boot sequence")
		mgr.Add("one", Noop, Noop)
		i, err := mgr.Sequence("one")
		verifyNilErr(t, err)

		defer verifyPanicWithMsg(t, panicCallee)

		up := i.Up(context.Background())
		_ = up.Progress()
		_ = up.Wait()

		t.Fatal("expected Agent.Wait() to panic") // Never called if panic is triggered.
	})

	t.Run("panics when Agent.Progress() is called after Agent.Wait()", func(t *testing.T) {
		mgr := New("Single-step boot sequence")
		mgr.Add("one", Noop, Noop)
		i, err := mgr.Sequence("one")
		verifyNilErr(t, err)

		defer verifyPanicWithMsg(t, panicCallee)

		up := i.Up(context.Background())
		_ = up.Wait()
		_ = up.Progress()

		t.Fatal("expected Agent.Progress() to panic")
	})
}

func TestProgress(t *testing.T) {
	t.Run("returns one Progress report per step (simple case)", func(t *testing.T) {
		mgr := New("One-step boot sequence")
		mgr.Add("one", Noop, Noop)
		i, err := mgr.Sequence("one")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		pp := up.Progress()

		var ok bool
		for p := range pp {
			if p.Service != "one" {
				t.Fatalf("expected progress report with Service = %q, got %q", "one", p.Service)
			}
			if p.Err != nil {
				verifyNilErr(t, err)
			}
			ok = true
		}

		if !ok {
			t.Fatalf("expected one progress report, got none")
		}
	})

	t.Run("returns one Progress report per step", func(t *testing.T) {
		mgr := New("Three-step boot sequence")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		i, err := mgr.Sequence("one > two > three")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		pp := up.Progress()

		names := make([]string, 0, 3)
		for p := range pp {
			msg := p.Service
			if p.Err != nil {
				msg = p.Err.Error()
			}
			names = append(names, msg)
		}

		expected := "one,two,three"
		actual := strings.Join(names, ",")
		if actual != expected {
			t.Fatalf("expected progress chan to generate string %q, got %q", expected, actual)
		}
	})

	t.Run("returns one Progress report per step (advanced case)", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		mgr.Add("four", Noop, Noop)
		mgr.Add("five", Noop, Noop)
		mgr.Add("six", Noop, Noop)
		i, err := mgr.Sequence("one > two > (three : four : five) > six")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		pp := up.Progress()

		actual := make([]string, 0, 6)
		for p := range pp {
			msg := p.Service
			if p.Err != nil {
				msg = p.Err.Error()
			}
			actual = append(actual, msg)
		}

		expected := []string{"one", "two", "three", "four", "five", "six"}
		verifyStringSlicesEqual(t, expected, actual)
	})

	t.Run("returns one Progress report per step (very advanced case)", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Noop, Noop)
		mgr.Add("four", Noop, Noop)
		mgr.Add("five", Noop, Noop)
		mgr.Add("six", Noop, Noop)
		mgr.Add("seven", Noop, Noop)
		mgr.Add("eight", Noop, Noop)
		mgr.Add("nine", Noop, Noop)
		mgr.Add("ten", Noop, Noop)
		i, err := mgr.Sequence("one > two > (three : four : (five > six > (seven:eight)) : nine) > ten")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		pp := up.Progress()

		actual := make([]string, 0, 10)
		for p := range pp {
			msg := p.Service
			if p.Err != nil {
				msg = p.Err.Error()
			}
			actual = append(actual, msg)
		}

		expected := []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
		verifyStringSlicesEqual(t, expected, actual)
	})

	t.Run("returns one Progress report per step up until a step error", func(t *testing.T) {
		mgr := New("Boot it!")
		mgr.Add("one", Noop, Noop)
		mgr.Add("two", Noop, Noop)
		mgr.Add("three", Errop, Errop)
		mgr.Add("four", Noop, Noop)
		i, err := mgr.Sequence("one > two > three > four")
		verifyNilErr(t, err)

		up := i.Up(context.Background())
		pp := up.Progress()

		actual := make([]string, 0, 3)
		for p := range pp {
			msg := p.Service
			if p.Err != nil {
				msg = p.Err.Error()
			}
			actual = append(actual, msg)
		}

		expected := []string{"one", "two", errStepFailure.Error()}
		verifyStringSlicesEqual(t, expected, actual)
	})
}

func newStepPtr(name string) *step {
	st := newStep(name)
	return &st
}

func verifyNilErr(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func verifyCountEq(t *testing.T, c uint32, expected uint32) {
	t.Helper()

	if c != expected {
		t.Fatalf("expected count to equal %d, got %d", expected, c)
	}
}

func verifyPanicWithMsg(t *testing.T, expected string) {
	t.Helper()

	err := recover()
	if err == nil {
		t.Fatal("expected a panic")
	}
	actual, ok := err.(string)
	if !ok {
		t.Fatalf("expected to panic with string, got %v", reflect.TypeOf(err).String())
	}
	if actual != expected {
		t.Fatalf("expected panic message to equal %q, got %q", expected, actual)
	}
}

func verifyParseError(t *testing.T, err error, match string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	pe, ok := err.(ErrParsingFormula)
	if !ok {
		t.Fatalf("expected ErrParsingFormula, got %v", err)
	}

	if match != "" && !strings.Contains(pe.Error(), match) {
		t.Fatalf("expected error to match %q, got %q", match, pe.Error())
	}
}

func verifyChannelCap(t *testing.T, ch chan Progress, capacity int) {
	t.Helper()

	actualCap := cap(ch)
	if actualCap != capacity {
		t.Fatalf("expected channel with capacity %d, got %d", capacity, actualCap)
	}
}

func verifyIdenticalSets(t *testing.T, aa, bb []string) {
	t.Helper()

	if len(aa) != len(bb) {
		t.Fatalf("sets have different lengths, len(aa) == %d and len(bb) == %d", len(aa), len(bb))
	}

	var found bool
	for _, a := range aa {
		found = false
		for _, b := range bb {
			if b == a {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("second set does not contain value %q", a)
		}
	}
}

func verifyStringSlicesEqual(t *testing.T, aa, bb []string) {
	t.Helper()

	if len(aa) != len(bb) {
		t.Fatalf("expected two string slices of equal length, got %q and %q", strings.Join(aa, ","), strings.Join(bb, ","))
	}

	// An n^2 algorithm is fine for testing purposes.
	for _, a := range aa {
		found := false
		for _, b := range bb {
			if b == a {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected second slice to contain %q", a)
		}
	}
}
