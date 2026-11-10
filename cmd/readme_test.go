package cmd

import (
	"os"
	"strings"
	"testing"
)

// The command list at the top of README.md must name every top-level command.
func TestReadmeListsEveryCommand(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, list, ok := strings.Cut(string(data), "Available Commands (aliases):")
	if !ok {
		t.Fatal("README.md has no command list")
	}
	list, _, _ = strings.Cut(list, "Flags:")

	listed := map[string]string{}
	for _, line := range strings.Split(list, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			listed[fields[0]] = line
		}
	}
	for _, c := range rootCmd.Commands() {
		line, ok := listed[c.Name()]
		if !ok {
			t.Errorf("README.md command list is missing %q", c.Name())
			continue
		}
		for _, alias := range c.Aliases {
			if !strings.Contains(line, alias) {
				t.Errorf("README.md lists %q without its alias %q", c.Name(), alias)
			}
		}
		delete(listed, c.Name())
	}
	// cobra adds these two itself when the program runs.
	delete(listed, "help")
	delete(listed, "completion")
	for name := range listed {
		t.Errorf("README.md lists %q, which is not a command", name)
	}
}
