package site

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"air-automation/internal/action"
)

type Challenge struct {
	Prompt string `yaml:"prompt"`
	Detect string `yaml:"detect"`
}

type Profile struct {
	Name      string            `yaml:"-"`
	Domains   []string          `yaml:"domains"`
	LoginURL  string            `yaml:"login_url"`
	Selectors map[string]string `yaml:"selectors"`
	Menus     map[string]string `yaml:"menus"`
	MFA       *Challenge        `yaml:"mfa"`
	Captcha   *Challenge        `yaml:"captcha"`
	OTPEmail  *OTPEmail         `yaml:"otp_email"`
}

// OTPEmail describes this site's login-code email: who sends it and a regex
// whose first capture group is the code.
type OTPEmail struct {
	From    string `yaml:"from"`
	Pattern string `yaml:"pattern"`
}

type Config struct {
	Sites map[string]*Profile `yaml:"sites"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sites config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse sites config %s: %w", path, err)
	}
	if len(cfg.Sites) == 0 {
		return nil, fmt.Errorf("sites config %s defines no sites", path)
	}
	for name, profile := range cfg.Sites {
		if profile == nil {
			return nil, fmt.Errorf("site %q is empty", name)
		}
		profile.Name = name
	}
	return &cfg, nil
}

func (c *Config) Get(name string) (*Profile, error) {
	profile, ok := c.Sites[name]
	if !ok {
		return nil, fmt.Errorf("unknown site %q (available: %s)", name, strings.Join(c.Names(), ", "))
	}
	return profile, nil
}

func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Sites))
	for name := range c.Sites {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *Profile) ChallengeDetectors() []Challenge {
	var out []Challenge
	if p.MFA != nil && p.MFA.Detect != "" {
		out = append(out, *p.MFA)
	}
	if p.Captcha != nil && p.Captcha.Detect != "" {
		out = append(out, *p.Captcha)
	}
	return out
}

func (p *Profile) MenuNames() []string {
	names := make([]string, 0, len(p.Menus))
	for name := range p.Menus {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *Profile) FieldNames() []string {
	names := make([]string, 0, len(p.Selectors))
	for name := range p.Selectors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *Profile) Resolve(plan *action.Plan) error {
	for i := range plan.Steps {
		step := &plan.Steps[i]
		if step.Selector != "" {
			continue
		}
		switch {
		case step.Menu != "":
			selector, ok := p.Menus[step.Menu]
			if !ok {
				return fmt.Errorf("step %d: site %q has no menu %q", i+1, p.Name, step.Menu)
			}
			step.Selector = selector
		case step.Field != "":
			selector, ok := p.Selectors[step.Field]
			if !ok {
				return fmt.Errorf("step %d: site %q has no field %q", i+1, p.Name, step.Field)
			}
			step.Selector = selector
		}
	}
	return nil
}
