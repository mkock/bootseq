package bootseq_test

import (
	"context"
	"fmt"
	"github.com/mkock/bootseq"
	"strings"
)

func Example_basic() {
	// Let's use a boot sequence to construct a sentence!
	// For the shutdown sequence, we'll "deconstruct" it by removing each word.
	var words []string

	add := func(word string) func() error {
		return func() error {
			words = append(words, word)
			return nil
		}
	}

	rm := func() error {
		words = words[:len(words)-1]
		return nil
	}

	seq := bootseq.New("Basic Example")
	seq.Add("welcome", add("Welcome"), rm)
	seq.Add("to", add("to"), rm)
	seq.Add("my", add("my"), rm)
	seq.Add("world", add("world!"), rm)
	i, err := seq.Sequence("welcome > to > my > world")
	if err != nil {
		panic(err)
	}

	// Bootup sequence.
	up := i.Up(context.Background())
	up.Wait()

	fmt.Println(strings.Join(words, " "))

	// Shutdown sequence.
	down := up.Down(context.Background())
	down.Wait()

	fmt.Println(strings.Join(words, " "))

	// Output:
	// Welcome to my world!
	//
}
