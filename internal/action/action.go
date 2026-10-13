package action

import (
	"fmt"
	"strings"
)

type Kind string

const (
	Navigate   Kind = "navigate"
	Click      Kind = "click"
	Fill       Kind = "fill"
	Read       Kind = "read"
	Wait       Kind = "wait"
	Press      Kind = "press"
	Screenshot Kind = "screenshot"
	AwaitUser  Kind = "await_user"
)

func (k Kind) Valid() bool {
	switch k {
	case Navigate, Click, Fill, Read, Wait, Press, Screenshot, AwaitUser:
		return true
	}
	return false
}

func Kinds() []Kind {
	return []Kind{Navigate, Click, Fill, Read, Wait, Press, Screenshot, AwaitUser}
}

type Action struct {
	Kind          Kind   `json:"kind"`
	URL           string `json:"url,omitempty"`
	Selector      string `json:"selector,omitempty"`
	Frame         string `json:"frame,omitempty"` // optional iframe selector the step's selector lives in
	Menu          string `json:"menu,omitempty"`
	Field         string `json:"field,omitempty"`
	Value         string `json:"value,omitempty"`
	Key           string `json:"key,omitempty"`
	DurationMS    int    `json:"duration_ms,omitempty"`
	Path          string `json:"path,omitempty"`
	Prompt        string `json:"prompt,omitempty"`
	Consequential bool   `json:"consequential,omitempty"`
	Description   string `json:"description,omitempty"`
}

type Plan struct {
	Task  string   `json:"task"`
	Steps []Action `json:"steps"`
}

func (a Action) Describe() string {
	switch a.Kind {
	case Navigate:
		return fmt.Sprintf("navigate to %s", a.URL)
	case Click:
		if a.Menu != "" {
			return fmt.Sprintf("click menu %q", a.Menu)
		}
		return fmt.Sprintf("click %s", a.Selector)
	case Fill:
		target := a.Selector
		if a.Field != "" {
			target = fmt.Sprintf("field %q", a.Field)
		}
		return fmt.Sprintf("fill %s with %s", target, DisplayValue(a.Value))
	case Read:
		return fmt.Sprintf("read text from %s", a.Selector)
	case Wait:
		if a.Selector != "" {
			return fmt.Sprintf("wait for %s", a.Selector)
		}
		return fmt.Sprintf("wait %dms", a.DurationMS)
	case Press:
		return fmt.Sprintf("press %q on %s", a.Key, a.Selector)
	case Screenshot:
		return fmt.Sprintf("screenshot to %s", a.Path)
	case AwaitUser:
		if a.Prompt != "" {
			return fmt.Sprintf("await user: %s", a.Prompt)
		}
		return "await user"
	}
	return string(a.Kind)
}

func DisplayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return `""`
	}
	if strings.Contains(value, "{{ENV:") {
		return value
	}
	return "[redacted]"
}
