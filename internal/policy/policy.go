package policy

import (
	"fmt"
	"net/url"
	"strings"

	"air-automation/internal/action"
	"air-automation/internal/secret"
)

const DefaultMaxSteps = 25

type Policy struct {
	AllowedDomains []string
	AllowedActions map[action.Kind]bool
	MaxSteps       int
	RequireHTTPS   bool
}

func New(domains []string, maxSteps int) Policy {
	allowed := make(map[action.Kind]bool, len(action.Kinds()))
	for _, k := range action.Kinds() {
		allowed[k] = true
	}
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	cleaned := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(d, ".")
		if d != "" {
			cleaned = append(cleaned, d)
		}
	}
	return Policy{
		AllowedDomains: cleaned,
		AllowedActions: allowed,
		MaxSteps:       maxSteps,
		RequireHTTPS:   true,
	}
}

func (p Policy) Validate(plan action.Plan) error {
	if len(p.AllowedDomains) == 0 {
		return fmt.Errorf("no allowed domains configured; refusing to run")
	}
	if len(plan.Steps) == 0 {
		return fmt.Errorf("plan contains no steps")
	}
	if len(plan.Steps) > p.MaxSteps {
		return fmt.Errorf("plan has %d steps, exceeding the limit of %d", len(plan.Steps), p.MaxSteps)
	}
	for i, a := range plan.Steps {
		if err := p.validateAction(a); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
}

func (p Policy) validateAction(a action.Action) error {
	if !a.Kind.Valid() {
		return fmt.Errorf("unknown action kind %q", a.Kind)
	}
	if !p.AllowedActions[a.Kind] {
		return fmt.Errorf("action %q is not allowed", a.Kind)
	}
	switch a.Kind {
	case action.Navigate:
		raw, err := secret.Resolve(a.URL)
		if err != nil {
			return err
		}
		if raw == "" {
			return fmt.Errorf("navigate requires a url")
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("invalid url %q: %w", raw, err)
		}
		if parsed.Scheme != "https" && (parsed.Scheme != "http" || p.RequireHTTPS) {
			return fmt.Errorf("url scheme %q is not allowed", parsed.Scheme)
		}
		if !p.domainAllowed(parsed.Hostname()) {
			return fmt.Errorf("domain %q is not in the allowlist", parsed.Hostname())
		}
	case action.Click, action.Read:
		if strings.TrimSpace(a.Selector) == "" {
			return fmt.Errorf("%s requires a selector", a.Kind)
		}
	case action.Fill:
		if strings.TrimSpace(a.Selector) == "" {
			return fmt.Errorf("fill requires a selector")
		}
		if err := secret.Ensure(a.Value); err != nil {
			return err
		}
	case action.Press:
		if strings.TrimSpace(a.Selector) == "" || strings.TrimSpace(a.Key) == "" {
			return fmt.Errorf("press requires a selector and a key")
		}
	case action.Wait:
		if strings.TrimSpace(a.Selector) == "" && a.DurationMS <= 0 {
			return fmt.Errorf("wait requires a selector or a positive duration_ms")
		}
	case action.Screenshot:
		if strings.TrimSpace(a.Path) == "" {
			return fmt.Errorf("screenshot requires a path")
		}
		if strings.HasPrefix(a.Path, "/") || strings.Contains(a.Path, "..") {
			return fmt.Errorf("screenshot path must be relative and stay within the working directory")
		}
	}
	return nil
}

func (p Policy) domainAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, domain := range p.AllowedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
