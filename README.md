# bootseq

[![GoDoc](https://godoc.org/github.com/mkock/bootseq?status.svg)](https://godoc.org/github.com/mkock/bootseq)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![GoReportCard](https://goreportcard.com/badge/github.com/mkock/bootseq)](https://goreportcard.com/report/github.com/mkock/bootseq)

Package bootseq provides a light-weight, general-purpose boot sequence manager with separate startup/shutdown phases,
real-time progress reports, cancellation and a simple mechanism that allows for easy control over execution order and
concurrency.

_Compatibility: Go 1.7+_

## Installation

This package supports Go Modules. Simply run:

`go get github.com/mkock/bootseq/v2`

to get the latest tagged version.

## Quickstart

```go
sequence := bootseq.New("My Boot Sequence")
sequence.Register("some-service", someServiceUp, someServiceDown)
sequence.Register("other-service", otherServiceUp, otherServiceDown)
sequence.Register("third-service", thirdServiceUp, thirdServiceDown).After("other-service")
agent, err := sequence.Agent()
up := agent.Up(context.Background())
up.Wait()

// Your application is now ready!

down := agent.Down(context.Background())
down.Wait()
```

## Preamble

This README describes v2.

Although v1 is still available, it has been deprecated due to its quirky syntax for formulating execution order which,
to be honest, is much better handled via simple method calls. V2 greatly improves upon this. 

## Introduction

A "boot sequence" is one or more _Services_ to execute. Each service is a name and two functions, one for
booting up (the _startup_ phase) and one for shutting down (the _shutdown_ phase).
These are registered with the _Manager_.

The practicalities of executing the two up/down phases is handled
by an _Agent_, which can be instantiated directly from the _Manager_ once _Service_ registration has completed.

## Usage

Start by analyzing your boot sequence. Some services _must_ run in a certain order (ie. you must establish
a database connection before you can preload data for an in-memory cache), and some services can run concurrently
when they don't depend on each other.

Next, write two functions per service: a startup function, and a shutdown function. If you don't need both, you can
use `bootseq.Noop` as a replacement, it simply does nothing.

Once your functions are ready, you just need to register them under a meaningful name and then define their order of
execution by calling `Service.After("name-of-dependent-service")`.

If your application has multiple packages that need to do some processing during the startup and shutdown phases of
the boot sequence, you can register them in the `init` functions of the respective packages.

## Details

### Progress reports

Once a boot sequence is in progress, you may call either `Agent.Wait()` or
`Agent.Progress()` on the agent. You'll get an error if you call both.

`Agent.Wait()` will block while the boot sequence is running. It returns when the sequence has completed, or if
an error was raised during execution.

`Agent.Progress()`, on the other hand, will return a channel of `Progress` structs. You can then iterate over
each `Progress` struct to receive progress updates as they occur. Although these progress reports don't tell you how
far you are in the boot sequence, you can easily devise your own progress indicator by getting the total number of
services from `Manager.ServiceCount()` combined with a counter that is incremented by one for every _Progress_
report received. _Progress_ reports also serve as error handlers.

_Progress_ reports are simple structs containing the name of the executed service
and an error (which is nil for successful execution):

```go
type Progress struct {
	Service string
	Err     error
}
```

Due to the fact that execution steps may be cancelled or time out due to their associated context, the reported
error can be of type `context.Canceled` or `context.DeadlineExceeded`. It can also be of any type returned by your
_Service_ functions.

Note: you will receive exactly one _Progress_ report per registered _Service_, plus one extra that indicates the end
of the boot sequence. The last _Progress_ report will always have an empty _Service_ name.

### Cancellation and Errors

Any boot sequence can be cancelled by calling `Agent.Up()` with a context that can be cancelled.
A call to `Context.Done()` is checked before each non-concurrent _Service_ execution, so a _Service_ that is
in progress will finish before stopping. For sequences where multiple _Services_ are running concurrently, the _Agent_
will wait for all of them to finish before stopping. 

If, for example, Service A, B and C run sequentially - one by one - and there is an error in B, that means that A would
still have completed, but as the _Agent_ is not waiting for any other concurrent _Services_, execution can
halt immediately and C won't run.

In another example, _Service_ A, B and C run concurrently, and _Service_ D runs after C. If B fails, A and C will
continue to run to completion, but execution stops afterwards, and D won't run.

### Builtin Limitations

- A _Manager_ cannot contain more than 65535 _Services_
- An _Agent_ cannot contain more than 65535 priorities

A panic is raised if any of these limitations are breached.

## Feature Wish List

- Optional injection of logger after instantiation
- Proper shutdown on panics and timeouts
- Method for unregistering services

## FAQ

**What happens if I register Service B before Service A, when B depends on A?**

No problem. Service dependencies aren't resolved until _Agent_ instantiation. You are free to register _Services_ via the `init` functions of your various packages - in fact, this is the intended use case.

**Can more Services be added to the Manager after Agent instantiation?**

While it's possible to continue adding more _Services_ to the _Manager_ after _Agent_ instantiation, they will not be part of the _Agent's_ boot sequence. You'll have to re-instantiate it.

**What if registered Services have cyclic dependencies?**

These will be detected during _Agent_ instantiation, and you'll receive an error if this happens.

**What are the dependencies for this package?**

None. Just stdlib and golang.org/x/sync.

## Contributing

Contributions are welcome in the form of well-explained PR's along with some
beautiful unit tests ;-)

## About v1

The v1 version of this package is deprecated and unsupported.
In case you need to use it anyway, [the documentation for v1 is here](README_v1.md).

## License

This software package is released under the MIT License.
