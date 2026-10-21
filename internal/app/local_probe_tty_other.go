//go:build !((darwin || linux) && cgo)

package app

// Without cgo there is no openpty(3); local probes fall back to plain pipes.
// Output still streams whenever the child flushes, exactly like before the
// pty improvement.
func openLocalProbeTTY() (*probeTTY, error) { return nil, nil }
