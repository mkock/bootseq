package bootseq

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

type indexUpdater []bool

func newIndexUpdater(capacity int) *indexUpdater {
	u := make(indexUpdater, capacity)
	return &u
}

func (i *indexUpdater) index(j int) func() error {
	return func() error {
		(*i)[j] = true
		return nil
	}
}

func verifyIndexUpdater(t *testing.T, i *indexUpdater) {
	t.Helper()

	for j, value := range *i {
		if !value {
			t.Fatalf("expected index %d to be true", j)
		}
	}
}

var errService = errors.New("service has failed")

// ErrOp (error operation) is a convenience function you can use in place of a
// step function for when you want a function that returns an error.
func ErrOp() error {
	return errService
}

// PanicOp (panic operation) is a convenience function you can use in place of a
// step function for when you want a function that panics.
func PanicOp() error {
	panic(errService.Error())
}

// SleepOp (sleep operation) is a convenience function you can use in place of a
// step function for when you want a function that sleeps for a short while.
func SleepOp() error {
	time.Sleep(250 * time.Millisecond)
	return nil
}

func verifyNilErr(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func progressChannelAsStrings(pChan chan Progress) []string {
	names := make([]string, 0)
	for p := range pChan {
		msg := p.Service
		if p.Err != nil {
			msg = p.Err.Error()
		}
		names = append(names, msg)
	}
	return names
}

func verifyErrorType(t *testing.T, actual, expected error) {
	t.Helper()

	if actual != expected {
		t.Fatalf("expected error of type %T(%s), got %T(%s)", expected, expected.Error(), actual, actual.Error())
	}
}

func verifyStringEquals(t *testing.T, expected, actual string) {
	t.Helper()

	if expected != actual {
		t.Fatalf("expected %q to equal %q", actual, expected)
	}
}

func verifyStringsEqual(t *testing.T, expected, actual []string) bool {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(actual))
	}

	if len(actual) == 0 {
		return true
	}

	isOrderPreserved := true
	for i := range expected {
		found := false
		for j := range actual {
			if actual[j] == expected[i] {
				found = true
				if i != j {
					isOrderPreserved = false
				}
				break
			}
		}
		if !found {
			t.Fatalf("expected actual to contain %q", expected[i])
		}
	}

	return isOrderPreserved
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
