// Package bootseq provides a general-purpose boot sequence manager with separate
// startup/shutdown phases, cancellation and a simple abstraction that allows easy
// control over execution order and concurrency.
//
// Quick Start
//
// 	seq := bootseq.New("My Boot Sequence")
// 	seq.Add("some-service", someServiceUp, someServiceDown)
// 	seq.Add("other-service", otherServiceUp, otherServiceDown)
// 	seq.Add("third-service", thirdServiceUp, thirdServiceDown)
//
//	// Execute "some-service" and "other-service" concurrently, followed by "third-service".
// 	seq.Sequence("(some-service : other-service) > third-service")
// 	up := seq.Up(context.Background())
// 	up.Wait()
//
// 	// Your application is now ready!
//
// 	down := up.Down(context.Background())
// 	down.Wait()
//
// Please refer to the enclosed README for more details.
package bootseq
