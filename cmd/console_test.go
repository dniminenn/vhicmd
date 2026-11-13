package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestConsoleExecLine(t *testing.T) {
	line := consoleExecLine("ab12", "/dev/ttyS0", "ls /root")
	want := `echo VHICMD""BEGIN-ab12 >/dev/ttyS0; { ls /root; } >/dev/ttyS0 2>&1; echo VHICMD""END-ab12 $? >/dev/ttyS0`
	if line != want {
		t.Errorf("line = %s", line)
	}
	// The typed line must not look like a finished command if the VM echoes it to the log.
	if _, _, _, done := consoleExecResult(line+"\n", "ab12"); done {
		t.Error("the typed line matches its own markers")
	}
}

func TestConsoleExecResult(t *testing.T) {
	cases := []struct {
		name, log, output string
		status            int
		truncated, done   bool
	}{
		{"finished", "boot\r\nVHICMDBEGIN-id\r\none\r\ntwo\r\nVHICMDEND-id 0\r\n", "one\ntwo\n", 0, false, true},
		{"failed", "VHICMDBEGIN-id\nno such file\nVHICMDEND-id 2\n", "no such file\n", 2, false, true},
		{"no output", "VHICMDBEGIN-id\nVHICMDEND-id 0\n", "", 0, false, true},
		{"kernel line between", "VHICMDBEGIN-id\n[ 12.3] eth0 up\nout\nVHICMDEND-id 0\n", "[ 12.3] eth0 up\nout\n", 0, false, true},
		{"still running", "VHICMDBEGIN-id\npartial\n", "", 0, false, false},
		{"status half logged", "VHICMDBEGIN-id\nVHICMDEND-id 1", "", 0, false, false},
		{"other command", "VHICMDBEGIN-zz\nx\nVHICMDEND-zz 0\n", "", 0, false, false},
		{"start scrolled away", "tail of output\nVHICMDEND-id 0\n", "tail of output\n", 0, true, true},
		{"earlier run of same id ignored", "VHICMDBEGIN-id\nold\nVHICMDEND-id 0\nVHICMDBEGIN-id\nnew\nVHICMDEND-id 0\n", "new\n", 0, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, status, truncated, done := consoleExecResult(tc.log, "id")
			if output != tc.output || status != tc.status || truncated != tc.truncated || done != tc.done {
				t.Errorf("got %q, %d, %v, %v", output, status, truncated, done)
			}
		})
	}
}

func TestExitStatus(t *testing.T) {
	if got := exitStatus(fmt.Errorf("wrapped: %w", exitCodeError{3})); got != 3 {
		t.Errorf("exit status = %d, want 3", got)
	}
	if got := exitStatus(errors.New("plain")); got != 1 {
		t.Errorf("exit status = %d, want 1", got)
	}
	if !strings.Contains(exitCodeError{3}.Error(), "3") {
		t.Error("error text lacks the status")
	}
}
