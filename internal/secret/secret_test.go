package secret

import "testing"

func TestResolveSubstitutesEnv(t *testing.T) {
	t.Setenv("SITE_PASSWORD", "s3cret")
	got, err := Resolve("prefix-{{ENV:SITE_PASSWORD}}-suffix")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "prefix-s3cret-suffix" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveMissingEnv(t *testing.T) {
	if _, err := Resolve("{{ENV:DEFINITELY_MISSING}}"); err == nil {
		t.Fatal("expected error for missing env")
	}
}

func TestRedactRemovesReferences(t *testing.T) {
	if got := Redact("value {{ENV:SECRET}} here"); got != "value [redacted] here" {
		t.Fatalf("got %q", got)
	}
}
