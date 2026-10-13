package secret

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var pattern = regexp.MustCompile(`\{\{ENV:([A-Za-z_][A-Za-z0-9_]*)\}\}`)

func Refs(value string) []string {
	matches := pattern.FindAllStringSubmatch(value, -1)
	refs := make([]string, 0, len(matches))
	for _, m := range matches {
		refs = append(refs, m[1])
	}
	return refs
}

func resolve(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return "", fmt.Errorf("missing environment variable: %s", name)
	}
	return value, nil
}

func Resolve(value string) (string, error) {
	var err error
	out := pattern.ReplaceAllStringFunc(value, func(match string) string {
		name := pattern.FindStringSubmatch(match)[1]
		var resolved string
		resolved, err = resolve(name)
		if err != nil {
			return match
		}
		return resolved
	})
	if err != nil {
		return "", err
	}
	return out, nil
}

func Ensure(value string) error {
	for _, name := range Refs(value) {
		if _, err := resolve(name); err != nil {
			return err
		}
	}
	return nil
}

func Redact(text string) string {
	return pattern.ReplaceAllString(text, "[redacted]")
}

func Contains(value string) bool {
	return strings.Contains(value, "{{ENV:")
}
