package bootseq_test

import (
	"context"
	"fmt"
	"github.com/mkock/bootseq"
	"strings"
)

func Example_progress() {
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
	seq.Register("welcome", add("Welcome"), rm)
	seq.Register("to", add("to"), rm).After("welcome")
	seq.Register("my", add("my"), rm).After("to")
	seq.Register("world", add("world!"), rm).After("my")
	agent, _ := seq.Agent()

	// Startup sequence.
	_ = agent.Up(context.Background())
	prog, _ := agent.Progress()

	for p := range prog {
		fmt.Println(p.Service)
	}

	fmt.Println(strings.Join(words, " "))

	// Shutdown sequence.
	_ = agent.Down(context.Background())
	prog, _ = agent.Progress()

	for p := range prog {
		fmt.Println(p.Service)
	}

	fmt.Println(strings.Join(words, " "))

	// Output:
	// welcome
	// to
	// my
	// world
	//
	// Welcome to my world!
	// world
	// my
	// to
	// welcome
	//
}
