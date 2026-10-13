package runner

import (
	"context"
	"io"
	"testing"

	"air-automation/internal/action"
	"air-automation/internal/audit"
	"air-automation/internal/executor"
	"air-automation/internal/policy"
	"air-automation/internal/site"
)

type fakeExecutor struct {
	visible  map[string]bool
	executed []action.Kind
}

func (f *fakeExecutor) Execute(a action.Action) (executor.Result, error) {
	f.executed = append(f.executed, a.Kind)
	return executor.Result{Kind: a.Kind}, nil
}

func (f *fakeExecutor) IsVisible(selector string) (bool, error) {
	return f.visible[selector], nil
}

type fakeApprover struct{ calls int }

func (f *fakeApprover) Approve(int, action.Action) (bool, error) {
	f.calls++
	return true, nil
}

type fakeHandoff struct {
	waits    int
	messages []string
}

func (f *fakeHandoff) Wait(message string) error {
	f.waits++
	f.messages = append(f.messages, message)
	return nil
}

func newRunner(ex *fakeExecutor, ap *fakeApprover, ho *fakeHandoff, profile *site.Profile) *Runner {
	return &Runner{
		Policy:   policy.New([]string{"acme.example.com"}, 10),
		Executor: ex,
		Approver: ap,
		Handoff:  ho,
		Auditor:  audit.NewLogger(io.Discard),
		Site:     profile,
		Out:      io.Discard,
	}
}

func TestRunPausesOnChallenge(t *testing.T) {
	ex := &fakeExecutor{visible: map[string]bool{"input#otp": true}}
	ap := &fakeApprover{}
	ho := &fakeHandoff{}
	profile := &site.Profile{Name: "acme", MFA: &site.Challenge{Detect: "input#otp", Prompt: "do 2fa"}}
	r := newRunner(ex, ap, ho, profile)

	plan := action.Plan{Steps: []action.Action{
		{Kind: action.Navigate, URL: "https://acme.example.com/login"},
		{Kind: action.Click, Selector: "#submit", Consequential: true},
	}}
	if _, err := r.Run(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if ap.calls != 1 {
		t.Fatalf("expected 1 approval, got %d", ap.calls)
	}
	if ho.waits != 2 {
		t.Fatalf("expected a handoff after each step, got %d", ho.waits)
	}
	if ho.messages[0] != "do 2fa" {
		t.Fatalf("unexpected message: %q", ho.messages[0])
	}
}

func TestRunAwaitUserStepDoesNotTouchBrowser(t *testing.T) {
	ex := &fakeExecutor{}
	ho := &fakeHandoff{}
	r := newRunner(ex, &fakeApprover{}, ho, nil)

	plan := action.Plan{Steps: []action.Action{
		{Kind: action.Navigate, URL: "https://acme.example.com/"},
		{Kind: action.AwaitUser, Prompt: "finish verification"},
	}}
	if _, err := r.Run(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if len(ex.executed) != 1 || ex.executed[0] != action.Navigate {
		t.Fatalf("await_user should not reach the executor: %v", ex.executed)
	}
	if ho.waits != 1 || ho.messages[0] != "finish verification" {
		t.Fatalf("expected one await_user handoff, got %+v", ho)
	}
}

func TestRunDeniedConsequentialStops(t *testing.T) {
	ex := &fakeExecutor{}
	denier := &denyApprover{}
	r := newRunner(ex, nil, nil, nil)
	r.Approver = denier
	r.Handoff = &fakeHandoff{}

	plan := action.Plan{Steps: []action.Action{
		{Kind: action.Navigate, URL: "https://acme.example.com/"},
		{Kind: action.Click, Selector: "#danger", Consequential: true},
	}}
	if _, err := r.Run(context.Background(), plan); err == nil {
		t.Fatal("expected run to stop when approval is denied")
	}
	for _, kind := range ex.executed {
		if kind == action.Click {
			t.Fatal("denied click should not execute")
		}
	}
}

type denyApprover struct{}

func (denyApprover) Approve(int, action.Action) (bool, error) { return false, nil }
