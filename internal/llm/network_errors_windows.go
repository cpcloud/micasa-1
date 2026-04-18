// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// connectionErrnos lists the WSA error values that mean "I could not
// reach the server" on Windows. The standard syscall.ECONNREFUSED on
// Windows is an APPLICATION_ERROR-prefixed invented constant that
// never matches what ConnectEx returns; the real values come from
// winsock and are exposed via golang.org/x/sys/windows. errors.Is
// unwraps net.OpError -> os.SyscallError{Syscall: "connectex"} ->
// syscall.Errno, so these match what net/http surfaces.
var connectionErrnos = []syscall.Errno{
	windows.WSAECONNREFUSED,
	windows.WSAENETUNREACH,
	windows.WSAEHOSTUNREACH,
}
