package approval

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"air-automation/internal/action"
)

type Approver interface {
	Approve(step int, a action.Action) (bool, error)
}

type Handoff interface {
	Wait(message string) error
}

type Auto struct {
	Allow bool
}

func (a Auto) Approve(int, action.Action) (bool, error) {
	return a.Allow, nil
}

func (a Auto) Wait(message string) error {
	return fmt.Errorf("%s (cannot wait for manual completion in non-interactive mode)", message)
}

type Stdin struct {
	scanner *bufio.Scanner
	out     io.Writer
}

func NewStdin(in io.Reader, out io.Writer) *Stdin {
	return &Stdin{scanner: bufio.NewScanner(in), out: out}
}

func (s *Stdin) Approve(step int, a action.Action) (bool, error) {
	fmt.Fprintf(s.out, "\nApproval required for step %d: %s\n", step+1, a.Describe())
	fmt.Fprintf(s.out, "This action may change important data. Approve? [y/N]: ")
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(s.scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

func (s *Stdin) Wait(message string) error {
	fmt.Fprintf(s.out, "\n%s\nPress Enter to continue once you are done in the browser... ", message)
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return err
		}
		return fmt.Errorf("input closed before confirmation")
	}
	return nil
}
