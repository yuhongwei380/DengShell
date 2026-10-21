//go:build (darwin || linux) && cgo

package app

/*
#ifdef __APPLE__
#include <util.h>
#else
#include <pty.h>
#endif

#cgo linux LDFLAGS: -lutil
*/
import "C"

import (
	"errors"
	"os"
)

// openLocalProbeTTY wraps openpty(3). The desktop build on macOS always
// enables cgo (the native Cocoa backend needs it), so local probes reliably
// get a terminal there and traceroute output arrives hop by hop.
func openLocalProbeTTY() (*probeTTY, error) {
	var master, slave C.int
	if C.openpty(&master, &slave, nil, nil, nil) != 0 {
		return nil, errors.New("openpty failed")
	}
	return &probeTTY{
		master: os.NewFile(uintptr(master), "probe-tty-master"),
		slave:  os.NewFile(uintptr(slave), "probe-tty-slave"),
	}, nil
}
