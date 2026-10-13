package site

import (
	"os"
	"path/filepath"
	"testing"

	"air-automation/internal/action"
)

const sample = `
sites:
  acme:
    domains:
      - acme.example.com
    selectors:
      username: "#user"
      submit: "#go"
    menus:
      billing: "a#billing"
    mfa:
      prompt: "do 2fa"
      detect: "input#otp"
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sites.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAndGet(t *testing.T) {
	cfg, err := Load(writeConfig(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := cfg.Get("acme")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "acme" {
		t.Fatalf("expected name acme, got %q", profile.Name)
	}
	if _, err := cfg.Get("missing"); err == nil {
		t.Fatal("expected unknown site error")
	}
}

func TestLoadRejectsEmptyConfig(t *testing.T) {
	if _, err := Load(writeConfig(t, "sites: {}\n")); err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestResolveMenuAndField(t *testing.T) {
	cfg, _ := Load(writeConfig(t, sample))
	profile, _ := cfg.Get("acme")
	plan := action.Plan{Steps: []action.Action{
		{Kind: action.Click, Menu: "billing"},
		{Kind: action.Fill, Field: "username"},
		{Kind: action.Click, Field: "submit"},
		{Kind: action.Click, Selector: "#explicit"},
	}}
	if err := profile.Resolve(&plan); err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Selector != "a#billing" {
		t.Fatalf("menu not resolved: %q", plan.Steps[0].Selector)
	}
	if plan.Steps[1].Selector != "#user" {
		t.Fatalf("field not resolved: %q", plan.Steps[1].Selector)
	}
	if plan.Steps[2].Selector != "#go" {
		t.Fatalf("field on click not resolved: %q", plan.Steps[2].Selector)
	}
	if plan.Steps[3].Selector != "#explicit" {
		t.Fatalf("explicit selector overwritten: %q", plan.Steps[3].Selector)
	}
}

func TestResolveUnknownNames(t *testing.T) {
	cfg, _ := Load(writeConfig(t, sample))
	profile, _ := cfg.Get("acme")
	bad := action.Plan{Steps: []action.Action{{Kind: action.Click, Menu: "nope"}}}
	if err := profile.Resolve(&bad); err == nil {
		t.Fatal("expected unknown menu error")
	}
}

func TestChallengeDetectors(t *testing.T) {
	cfg, _ := Load(writeConfig(t, sample))
	profile, _ := cfg.Get("acme")
	detectors := profile.ChallengeDetectors()
	if len(detectors) != 1 || detectors[0].Detect != "input#otp" {
		t.Fatalf("unexpected detectors: %+v", detectors)
	}
}
