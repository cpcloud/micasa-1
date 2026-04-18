// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

//go:build !windows

package llm

import "syscall"

// connectionErrnos lists the syscall errors that mean "I could not reach
// the server" on POSIX platforms. errors.Is unwraps net.OpError ->
// os.SyscallError -> syscall.Errno, so these match the values net/http
// surfaces when a connect(2) fails.
var connectionErrnos = []syscall.Errno{
	syscall.ECONNREFUSED,
	syscall.ENETUNREACH,
	syscall.EHOSTUNREACH,
}
