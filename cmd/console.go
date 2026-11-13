package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var (
	flagConsoleLines    int
	flagConsoleURL      bool
	flagConsoleNoEnter  bool
	flagConsoleKeys     []string
	flagConsoleKeyDelay time.Duration
	flagConsoleTTY      string
	flagConsoleWait     time.Duration
)

// exitCodeError makes vhicmd exit with the status of a command run inside a VM.
type exitCodeError struct{ code int }

func (e exitCodeError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.code)
}

var consoleCmd = &cobra.Command{
	Use:     "console <vm-id|vm-name>",
	Aliases: []string{"console-log"},
	Short:   "Print a VM's console log, or its console URL",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConsoleLines < 0 {
			return fmt.Errorf("--lines must not be negative")
		}
		if flagConsoleURL && cmd.Flags().Changed("lines") {
			return fmt.Errorf("--lines applies to the console log, not to --url")
		}

		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}

		vmID, err := api.GetVMIDByName(computeURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		if flagConsoleURL {
			consoleURL, err := api.GetConsoleURL(computeURL, tok.Value, vmID)
			if err != nil {
				return err
			}
			fmt.Println(consoleURL)
			return nil
		}

		output, err := api.GetConsoleOutput(computeURL, tok.Value, vmID, flagConsoleLines)
		if err != nil {
			return err
		}

		fmt.Print(output)
		if output != "" && !strings.HasSuffix(output, "\n") {
			fmt.Println()
		}
		return nil
	},
}

// openConsole starts a VNC session on a VM's console.
func openConsole(vm string) (*api.Console, string, string, error) {
	computeURL, err := validateTokenEndpoint(tok, "compute")
	if err != nil {
		return nil, "", "", err
	}
	vmID, err := api.GetVMIDByName(computeURL, tok.Value, vm)
	if err != nil {
		return nil, "", "", err
	}
	consoleURL, err := api.GetConsoleURL(computeURL, tok.Value, vmID)
	if err != nil {
		return nil, "", "", err
	}
	console, err := api.DialConsole(consoleURL)
	if err != nil {
		return nil, "", "", err
	}
	console.KeyDelay = flagConsoleKeyDelay
	return console, computeURL, vmID, nil
}

var consoleSendCmd = &cobra.Command{
	Use:   "send <vm-id|vm-name> [text]",
	Short: "Type text and keys on a VM's console keyboard",
	Long: `Types on the VM's keyboard through its VNC console, as a person at the noVNC page would.

The text is followed by Enter unless --no-enter is given. Keys named with --key are
pressed after the text, in order: enter, tab, esc, backspace, delete, up, down, left,
right, home, end, pageup, pagedown, f1 to f12, or a character, each optionally prefixed
with ctrl-, alt- or shift- (ctrl-c, ctrl-alt-delete).

Only printable ASCII can be typed, and the VM must use a US keyboard layout.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 && len(flagConsoleKeys) == 0 {
			return fmt.Errorf("nothing to send: give text, --key, or both")
		}
		console, _, _, err := openConsole(args[0])
		if err != nil {
			return err
		}
		defer console.Close()

		if len(args) == 2 {
			text := args[1]
			if !flagConsoleNoEnter {
				text += "\n"
			}
			if err := console.Type(text); err != nil {
				return err
			}
		}
		for _, key := range flagConsoleKeys {
			if err := console.PressKey(key); err != nil {
				return err
			}
		}
		return nil
	},
}

var consoleScreenshotCmd = &cobra.Command{
	Use:   "screenshot <vm-id|vm-name> <file.png>",
	Short: "Save a picture of a VM's console screen",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		console, _, _, err := openConsole(args[0])
		if err != nil {
			return err
		}
		defer console.Close()

		img, err := console.Screenshot()
		if err != nil {
			return err
		}
		f, err := os.Create(args[1])
		if err != nil {
			return err
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return fmt.Errorf("failed to write %s: %v", args[1], err)
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Saved the %dx%d console screen to %s\n", console.Width, console.Height, args[1])
		return nil
	},
}

// consoleExecLine is the shell line typed for "console exec". The markers are split by
// a pair of quotes so that the typed line itself can never match them.
func consoleExecLine(id, tty, command string) string {
	return fmt.Sprintf(`echo VHICMD""BEGIN-%s >%s; { %s; } >%s 2>&1; echo VHICMD""END-%s $? >%s`,
		id, tty, command, tty, id, tty)
}

// consoleExecResult finds the output of the command marked id in a console log.
// done is false until the end marker has been logged.
func consoleExecResult(log, id string) (output string, status int, truncated, done bool) {
	log = strings.ReplaceAll(log, "\r", "")
	end := strings.LastIndex(log, "VHICMDEND-"+id+" ")
	if end < 0 {
		return "", 0, false, false
	}
	rest := log[end+len("VHICMDEND-"+id+" "):]
	if line, _, found := strings.Cut(rest, "\n"); found {
		rest = line
	} else {
		// The status has not been fully logged yet.
		return "", 0, false, false
	}
	status, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return "", 0, false, false
	}

	body := log[:end]
	begin := strings.LastIndex(body, "VHICMDBEGIN-"+id)
	if begin < 0 {
		// The log only keeps its tail; the start of the output has scrolled out of it.
		return body, status, true, true
	}
	body = body[begin:]
	if _, after, found := strings.Cut(body, "\n"); found {
		body = after
	} else {
		body = ""
	}
	return body, status, false, true
}

var consoleExecCmd = &cobra.Command{
	Use:   "exec <vm-id|vm-name> <command>",
	Short: "Run a shell command in a VM through its console and print the output",
	Long: `Types the command at the VM's console and reads its output back from the console log.
vhicmd exits with the command's exit status.

The VM needs two things:
  - a root shell waiting at the VNC console (tty1), for example an autologin getty;
  - a serial port that the console log records, which is where the output is sent
    (--tty, default /dev/ttyS0).

The console log keeps only its tail (about 100 KB), so send large output to a file in
the VM and read the part you need. Only printable ASCII can be typed.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.ContainsAny(args[1], "\n\r") {
			return fmt.Errorf("the command must be a single line")
		}
		console, computeURL, vmID, err := openConsole(args[0])
		if err != nil {
			return err
		}
		defer console.Close()

		raw := make([]byte, 6)
		if _, err := rand.Read(raw); err != nil {
			return err
		}
		id := hex.EncodeToString(raw)
		if err := console.Type(consoleExecLine(id, flagConsoleTTY, args[1]) + "\n"); err != nil {
			return err
		}
		console.Close()

		deadline := time.Now().Add(flagConsoleWait)
		for {
			log, err := api.GetConsoleOutput(computeURL, tok.Value, vmID, 0)
			if err != nil {
				return err
			}
			output, status, truncated, done := consoleExecResult(log, id)
			if done {
				if truncated {
					fmt.Fprintln(os.Stderr, "warning: the start of the output is no longer in the console log")
				}
				fmt.Print(output)
				if status != 0 {
					cmd.SilenceErrors = true
					return exitCodeError{status}
				}
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("no result in the console log after %s: is a root shell waiting at the console, and does the VM log %s?", flagConsoleWait, flagConsoleTTY)
			}
			time.Sleep(time.Second)
		}
	},
}

// exitStatus is the process exit status for an error returned by the root command.
func exitStatus(err error) int {
	var code exitCodeError
	if errors.As(err, &code) && code.code > 0 && code.code < 256 {
		return code.code
	}
	return 1
}

func init() {
	consoleCmd.Flags().IntVarP(&flagConsoleLines, "lines", "n", 0, "Print only the last N lines (default: the whole log)")
	consoleCmd.Flags().BoolVar(&flagConsoleURL, "url", false, "Print the URL of the interactive (noVNC) console instead of the log")

	consoleSendCmd.Flags().BoolVar(&flagConsoleNoEnter, "no-enter", false, "Do not press Enter after the text")
	consoleSendCmd.Flags().StringArrayVar(&flagConsoleKeys, "key", nil, "Key to press after the text, e.g. enter, esc, ctrl-c (repeatable)")
	consoleExecCmd.Flags().StringVar(&flagConsoleTTY, "tty", "/dev/ttyS0", "Serial device in the VM that the console log records")
	consoleExecCmd.Flags().DurationVar(&flagConsoleWait, "wait", 2*time.Minute, "How long to wait for the command to finish")
	for _, c := range []*cobra.Command{consoleSendCmd, consoleExecCmd} {
		c.Flags().DurationVar(&flagConsoleKeyDelay, "key-delay", 20*time.Millisecond, "Pause after each key event; raise it if the VM drops characters")
	}

	consoleCmd.AddCommand(consoleSendCmd, consoleExecCmd, consoleScreenshotCmd)
	rootCmd.AddCommand(consoleCmd)
}
