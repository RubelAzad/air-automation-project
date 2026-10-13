package policy

import (
	"testing"

	"air-automation/internal/action"
)

func TestValidateAcceptsAllowedPlan(t *testing.T) {
	t.Setenv("SITE_URL", "https://app.example.com/login")
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{
		Task: "sign in",
		Steps: []action.Action{
			{Kind: action.Navigate, URL: "{{ENV:SITE_URL}}"},
			{Kind: action.Fill, Selector: "input[name=user]", Value: "{{ENV:USER}}"},
			{Kind: action.Click, Selector: "button", Consequential: true},
			{Kind: action.Wait, Selector: ".done"},
			{Kind: action.Press, Selector: "input", Key: "Enter"},
			{Kind: action.Read, Selector: "body"},
			{Kind: action.Screenshot, Path: "out/page.png"},
		},
	}
	if err := p.Validate(plan); err != nil {
		t.Fatalf("expected plan to validate, got %v", err)
	}
}

func TestValidateRejectsForeignDomain(t *testing.T) {
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{Steps: []action.Action{{Kind: action.Navigate, URL: "https://evil.test/login"}}}
	if err := p.Validate(plan); err == nil {
		t.Fatal("expected domain rejection")
	}
}

func TestValidateRejectsSubdomainSuffixTrick(t *testing.T) {
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{Steps: []action.Action{{Kind: action.Navigate, URL: "https://notexample.com/login"}}}
	if err := p.Validate(plan); err == nil {
		t.Fatal("expected notexample.com to be rejected")
	}
}

func TestValidateAcceptsSubdomain(t *testing.T) {
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{Steps: []action.Action{{Kind: action.Navigate, URL: "https://app.example.com/login"}}}
	if err := p.Validate(plan); err != nil {
		t.Fatalf("expected subdomain to be allowed, got %v", err)
	}
}

func TestValidateRejectsInsecureScheme(t *testing.T) {
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{Steps: []action.Action{{Kind: action.Navigate, URL: "http://example.com"}}}
	if err := p.Validate(plan); err == nil {
		t.Fatal("expected http scheme to be rejected")
	}
}

func TestValidateRejectsTooManySteps(t *testing.T) {
	p := New([]string{"example.com"}, 1)
	plan := action.Plan{Steps: []action.Action{
		{Kind: action.Navigate, URL: "https://example.com"},
		{Kind: action.Read, Selector: "body"},
	}}
	if err := p.Validate(plan); err == nil {
		t.Fatal("expected max steps rejection")
	}
}

func TestValidateRejectsUnknownKind(t *testing.T) {
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{Steps: []action.Action{{Kind: "execute_js"}}}
	if err := p.Validate(plan); err == nil {
		t.Fatal("expected unknown kind rejection")
	}
}

func TestValidateRejectsEmptyWait(t *testing.T) {
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{Steps: []action.Action{{Kind: action.Wait}}}
	if err := p.Validate(plan); err == nil {
		t.Fatal("expected wait rejection")
	}
}

func TestValidateRejectsScreenshotTraversal(t *testing.T) {
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{Steps: []action.Action{{Kind: action.Screenshot, Path: "../escape.png"}}}
	if err := p.Validate(plan); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestValidateRejectsMissingSecret(t *testing.T) {
	p := New([]string{"example.com"}, 10)
	plan := action.Plan{Steps: []action.Action{{Kind: action.Fill, Selector: "input", Value: "{{ENV:DEFINITELY_MISSING}}"}}}
	if err := p.Validate(plan); err == nil {
		t.Fatal("expected missing secret rejection")
	}
}
