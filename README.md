# bootseq

[![GoDoc](https://godoc.org/github.com/mkock/bootseq?status.svg)](https://godoc.org/github.com/mkock/bootseq)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![GoReportCard](https://goreportcard.com/badge/github.com/mkock/bootseq)](https://goreportcard.com/report/github.com/mkock/bootseq)

Package bootseq provides a general-purpose boot sequence manager with separate
startup/shutdown phases, cancellation and a simple abstraction that allows easy
control over execution order and concurrency.

## Installation

This package supports Go Modules. Simply run:

`go get github.com/mkock/bootseq`

to get the latest tagged version.

## Quickstart

```go
seq := bootseq.New("My Boot Sequence")
seq.Add("some-service", someServiceUp, someServiceDown)
seq.Add("other-service", otherServiceUp, otherServiceDown)
seq.Add("third-service", thirdServiceUp, thirdServiceDown)
seq.Sequence("(some-service : other-service) > third-service")
up := seq.Up(context.Background())
up.Wait()

// Your application is now ready!

down := up.Down(context.Background())
down.Wait()
```

## Introduction

A "boot sequence" is one or more _steps_ to execute. Each step is a name and
two functions, one for booting up (the _up_ phase) and one for shutting down
(the _down_ phase).

A _sequence_ is a predefined order of steps, in the form of step names
that can be grouped by parentheses and separated by two different characters,
`>` for serial execution, and `:` for parallel execution. A concrete sequence
of a specific set of steps is referred to as a _formula_.

The formula should be described as the boot sequence would look for the _up_
phase. The _down_ phase is the same formula, but executed in reverse.

The practical considerations of executing the two up/down phases is handled
by an _agent_, with each agent being in charge of exactly one phase.

## Usage

Start by analyzing your boot sequence. Some services _must_ run in a certain
order (ie. you must establish a database connection before you can preload data
for an in-memory cache), and some services can run concurrently when they don't
depend on each other.

Next, write two functions per service: a bootup function and a shutdown function.
If you don't need both, you can use `bootseq.Noop` as a replacement, it simply
does nothing. 

Once your functions are ready, you just need to register them under a meaningful
name and then write a formula of the boot sequence. This is a simple string that
describes the flow with a couple of special characters for instructing bootseq
on which services to execute in-order, and which to execute concurrently.

## Syntax

In order to define the formula, a little background knowledge of the syntax is
in order.

- Any word must match one of your registered service names. You may use underscore
  and dash as well as uppercase letters in your names, but no spacing or any
  other character. It's recommended to stick with lowercase letters only and to
  use short names, ie. "mysql" rather than "database_manager".
- Separate words by the character `>` for sequences, ie. where services must be
  executed in chronological order, and `:` for when services can be executed
  concurrently.
- Use parenthesis to group services whenever there are changes to the execution
  order. The parser is not sophisticated and may need some help figuring out
  the service groupings.

Make sure to register all services before defining your formula. Errors will be
raised when a word is encountered that doesn't match a service name.

## Examples

```go
// Run services "mysql", "aerospike" and "cache_stuff" in chronological order.
seq.Sequence("mysql > aerospike > cache_stuff")

// Run service "load_config" first, then "mysql" and "aerospike" concurrently,
// followed by "cache_stuff" after both "mysql" and "aerospike" are finished. 
seq.Sequence("load_config > (mysql : aerospike) > cache_stuff")

// Run service "logging", followed by "error_handling" and then three other
// services that can run concurrently. 
seq.Sequence("logging > error_handling > (mysql : aerospike : kafka)")
```

## Details

### Progress reports

Once a boot sequence is in progress, you may call either `Agent.Wait()` or
`Agent.Progress()` on the agent. It's a panic if you call both.

`Agent.Wait()` will block while listening to a special channel on which progress
reports are sent at the end of each step's execution. It returns when the sequence
has completed, or if an error was raised during execution.

`Agent.Progress()`, on the other hand, will return the very same progress channel
to you. You can then range over each element to receive progress updates as they
happen. Although these progress reports don't tell you how far you are in the
execution sequence, you can easily devise your own progress indicator by getting
the total number of steps from `Instance.CountSteps()` combined with a counter
that is incremented with one for every progress report received.

Progress reports are simple structs containing the name of the executed service
and an error (which is nil for successful execution):

```go
type Progress struct {
	Service string
	Err     error
}
```

Due to the fact that execution steps may be cancelled or time out due to their
associated context, the reported error can be of type `context.Canceled` or
`context.DeadlineExceeded`. It can also be of any type returned by your service
functions.

### Cancellation

Any boot sequence can be cancelled by calling `Agent.Up()` with a context that
can be cancelled. A call to `Context.Cancel()` is checked before each step, so
a step that is in progress, will finish before stopping. For steps that contain
multiple concurrent steps, the agent will wait for each one to finish before
stopping. 

## Builtin Limitations

- Any manager cannot contain more than 65535 services
- Any sequence cannot contain more than 256 steps

A panic is raised if any of these limitations are breached.

## Feature Wish List

- Optional injection of logger during instantiation
- Proper shutdown on panics and cancellations

## Contributing

Contributions are welcome in the form of well-explained PR's along with some
beautiful unit tests ;-)

## License

This software package is released under the MIT License.
