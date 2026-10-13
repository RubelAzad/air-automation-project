package executor

import (
	"fmt"
	"strings"

	"github.com/mxschmitt/playwright-go"

	"air-automation/internal/action"
	"air-automation/internal/secret"
)

type Options struct {
	Headless   bool
	TimeoutMS  float64
	ProfileDir string
	SlowMoMS   float64
}

type Executor struct {
	playwright *playwright.Playwright
	browser    playwright.Browser
	context    playwright.BrowserContext
	page       playwright.Page
}

type Result struct {
	Kind   action.Kind
	URL    string
	Output string
}

func New(opts Options) (*Executor, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("start playwright (run the install command if needed): %w", err)
	}

	var (
		browser playwright.Browser
		context playwright.BrowserContext
	)
	if opts.ProfileDir != "" {
		context, err = pw.Chromium.LaunchPersistentContext(opts.ProfileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
			Headless: playwright.Bool(opts.Headless),
			SlowMo:   playwright.Float(opts.SlowMoMS),
		})
		if err != nil {
			_ = pw.Stop()
			return nil, fmt.Errorf("launch persistent context at %s: %w", opts.ProfileDir, err)
		}
	} else {
		browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(opts.Headless),
			SlowMo:   playwright.Float(opts.SlowMoMS),
		})
		if err != nil {
			_ = pw.Stop()
			return nil, fmt.Errorf("launch chromium: %w", err)
		}
		context, err = browser.NewContext()
		if err != nil {
			_ = browser.Close()
			_ = pw.Stop()
			return nil, fmt.Errorf("create context: %w", err)
		}
	}

	page, err := context.NewPage()
	if err != nil {
		_ = context.Close()
		if browser != nil {
			_ = browser.Close()
		}
		_ = pw.Stop()
		return nil, fmt.Errorf("create page: %w", err)
	}
	if opts.TimeoutMS > 0 {
		page.SetDefaultTimeout(opts.TimeoutMS)
	}
	return &Executor{playwright: pw, browser: browser, context: context, page: page}, nil
}

func (e *Executor) Close() {
	if e.context != nil {
		_ = e.context.Close()
	}
	if e.browser != nil {
		_ = e.browser.Close()
	}
	if e.playwright != nil {
		_ = e.playwright.Stop()
	}
}

func (e *Executor) IsVisible(selector string) (bool, error) {
	return e.page.Locator(selector).IsVisible()
}

// locate finds a.Selector on the page, or inside the a.Frame iframe when set.
func (e *Executor) locate(a action.Action) playwright.Locator {
	if a.Frame != "" {
		return e.page.FrameLocator(a.Frame).Locator(a.Selector)
	}
	return e.page.Locator(a.Selector)
}

func (e *Executor) Execute(a action.Action) (Result, error) {
	result := Result{Kind: a.Kind}
	switch a.Kind {
	case action.Navigate:
		url, err := secret.Resolve(a.URL)
		if err != nil {
			return result, err
		}
		if _, err := e.page.Goto(url, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateLoad,
		}); err != nil {
			return result, fmt.Errorf("navigate to %s: %w", secret.Redact(a.URL), err)
		}
	case action.Click:
		if err := e.locate(a).Click(); err != nil {
			return result, fmt.Errorf("click %s: %w", a.Selector, err)
		}
	case action.Fill:
		value, err := secret.Resolve(a.Value)
		if err != nil {
			return result, err
		}
		if err := e.locate(a).Fill(value); err != nil {
			return result, fmt.Errorf("fill %s: %w", a.Selector, err)
		}
	case action.Read:
		text, err := e.locate(a).InnerText()
		if err != nil {
			return result, fmt.Errorf("read %s: %w", a.Selector, err)
		}
		result.Output = strings.TrimSpace(text)
	case action.Wait:
		if a.Selector != "" {
			if err := e.locate(a).WaitFor(); err != nil {
				return result, fmt.Errorf("wait for %s: %w", a.Selector, err)
			}
		} else {
			e.page.WaitForTimeout(float64(a.DurationMS))
		}
	case action.Press:
		if err := e.locate(a).Press(a.Key); err != nil {
			return result, fmt.Errorf("press %q on %s: %w", a.Key, a.Selector, err)
		}
	case action.Screenshot:
		if _, err := e.page.Screenshot(playwright.PageScreenshotOptions{
			Path:     playwright.String(a.Path),
			FullPage: playwright.Bool(true),
		}); err != nil {
			return result, fmt.Errorf("screenshot to %s: %w", a.Path, err)
		}
	default:
		return result, fmt.Errorf("unsupported action %q", a.Kind)
	}
	result.URL = e.page.URL()
	return result, nil
}
