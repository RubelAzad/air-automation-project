package runner

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"air-automation/internal/action"
	"air-automation/internal/approval"
	"air-automation/internal/audit"
	"air-automation/internal/executor"
	"air-automation/internal/otp"
	"air-automation/internal/policy"
	"air-automation/internal/site"
)

type StepExecutor interface {
	Execute(a action.Action) (executor.Result, error)
	IsVisible(selector string) (bool, error)
}

type Runner struct {
	Policy   policy.Policy
	Executor StepExecutor
	Approver approval.Approver
	Handoff  approval.Handoff
	Auditor  *audit.Logger
	Site     *site.Profile
	DryRun   bool
	Out      io.Writer
	// OTP fetches a login code sent by email after since; nil means a human types it.
	OTP func(ctx context.Context, since time.Time) (string, error)
}

func (r *Runner) Run(ctx context.Context, plan action.Plan) ([]executor.Result, error) {
	if err := r.Policy.Validate(plan); err != nil {
		r.record(-1, "plan", "", "invalid", err.Error())
		return nil, err
	}
	r.record(-1, "plan", "", "validated", fmt.Sprintf("%d steps", len(plan.Steps)))

	if r.DryRun {
		for i, a := range plan.Steps {
			fmt.Fprintf(r.Out, "[dry-run] step %d: %s\n", i+1, a.Describe())
			r.record(i, string(a.Kind), a.Describe(), "skipped", "dry-run")
		}
		return nil, nil
	}
	if r.Executor == nil {
		return nil, fmt.Errorf("no executor configured")
	}

	// Codes emailed before this run (minus clock skew) belong to someone else's login.
	started := time.Now().Add(-30 * time.Second)
	var results []executor.Result
	for i, a := range plan.Steps {
		if err := ctx.Err(); err != nil {
			return results, err
		}

		if a.Kind == action.AwaitUser {
			message := a.Prompt
			if message == "" {
				message = "Complete the required verification in the browser window."
			}
			if err := r.wait(i, a.Describe(), message); err != nil {
				return results, err
			}
			continue
		}

		if strings.Contains(a.Value, otp.Placeholder) {
			if r.OTP == nil {
				if err := r.wait(i, "otp", "Type the code from the email into the browser (don't submit it), then press Enter here."); err != nil {
					return results, err
				}
				continue
			}
			fmt.Fprintf(r.Out, "step %d: waiting for the OTP email...\n", i+1)
			code, err := r.OTP(ctx, started)
			if err != nil {
				r.record(i, "otp", a.Describe(), "error", err.Error())
				return results, fmt.Errorf("step %d: read OTP from email: %w", i+1, err)
			}
			r.record(i, "otp", a.Describe(), "fetched", "code read from email")
			a.Value = strings.ReplaceAll(a.Value, otp.Placeholder, code)
		}

		if a.Consequential {
			approved, err := r.Approver.Approve(i, a)
			if err != nil {
				r.record(i, string(a.Kind), a.Describe(), "error", "approval prompt failed")
				return results, fmt.Errorf("step %d approval: %w", i+1, err)
			}
			if !approved {
				r.record(i, string(a.Kind), a.Describe(), "denied", "user declined")
				return results, fmt.Errorf("step %d (%s) declined by user", i+1, a.Kind)
			}
			r.record(i, string(a.Kind), a.Describe(), "approved", "")
		}

		result, err := r.Executor.Execute(a)
		if err != nil {
			r.record(i, string(a.Kind), a.Describe(), "error", err.Error())
			return results, fmt.Errorf("step %d (%s) failed: %w", i+1, a.Kind, err)
		}
		results = append(results, result)
		r.record(i, string(a.Kind), a.Describe(), "ok", "")
		if result.Output != "" {
			fmt.Fprintf(r.Out, "step %d output: %s\n", i+1, result.Output)
		}

		if err := r.handleChallenge(i); err != nil {
			return results, err
		}
	}
	return results, nil
}

func (r *Runner) handleChallenge(step int) error {
	if r.Site == nil {
		return nil
	}
	for _, challenge := range r.Site.ChallengeDetectors() {
		visible, err := r.Executor.IsVisible(challenge.Detect)
		if err != nil {
			return fmt.Errorf("step %d: detect challenge %s: %w", step+1, challenge.Detect, err)
		}
		if !visible {
			continue
		}
		message := challenge.Prompt
		if message == "" {
			message = "A verification step (2FA or CAPTCHA) is required. Complete it in the browser window."
		}
		return r.wait(step, "challenge", message)
	}
	return nil
}

func (r *Runner) wait(step int, kind, message string) error {
	if r.Handoff == nil {
		return fmt.Errorf("step %d requires manual confirmation but no handoff is configured", step+1)
	}
	r.record(step, kind, message, "awaiting-user", "")
	if err := r.Handoff.Wait(message); err != nil {
		r.record(step, kind, message, "error", err.Error())
		return fmt.Errorf("step %d: %w", step+1, err)
	}
	r.record(step, kind, message, "resumed", "")
	return nil
}

func (r *Runner) record(step int, kind, target, outcome, detail string) {
	r.Auditor.Record(audit.Entry{
		Step:    step,
		Kind:    kind,
		Target:  target,
		Outcome: outcome,
		Detail:  detail,
	})
}
