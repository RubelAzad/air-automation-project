package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"air-automation/internal/action"
	"air-automation/internal/approval"
	"air-automation/internal/audit"
	"air-automation/internal/executor"
	"air-automation/internal/otp"
	"air-automation/internal/planner"
	"air-automation/internal/policy"
	"air-automation/internal/runner"
	"air-automation/internal/site"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		planPath   = flag.String("plan", "", "path to a JSON plan file")
		task       = flag.String("task", "", "task description for the AI planner")
		siteName   = flag.String("site", "", "site profile name from the sites config")
		sitesPath  = flag.String("sites", "sites.yaml", "path to the sites config file")
		domains    = flag.String("domains", "", "extra comma-separated allowed domains")
		profileDir = flag.String("profile", "", "browser user-data dir for persistent sessions (e.g. .browser-profile)")
		yes        = flag.Bool("yes", true, "auto-approve consequential steps; -yes=false to prompt before each")
		dryRun     = flag.Bool("dry-run", false, "validate and print the plan without running it")
		headless   = flag.Bool("headless", false, "run the browser headless (default visible for 2FA/CAPTCHA)")
		auditPath  = flag.String("audit", "audit.log", "audit log path, or - for stderr")
		timeoutSec = flag.Int("timeout", 30, "per-action timeout in seconds")
		maxSteps   = flag.Int("max-steps", policy.DefaultMaxSteps, "maximum number of steps allowed")
		listAction = flag.Bool("list-actions", false, "list allowed action kinds and exit")
		listSites  = flag.Bool("list-sites", false, "list configured site names and exit")
		slowMo     = flag.Int("slowmo", 500, "delay in ms between browser operations so a human can follow along")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] <site> <booking-id> <operation>\n  e.g. %[1]s tripgic 11201791798473311051FPQG cancel\n\nflags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	loadDotEnv(".env")

	var bookingID string
	if args := flag.Args(); len(args) > 0 {
		if len(args) != 3 {
			flag.Usage()
			return fmt.Errorf("expected <site> <booking-id> <operation>, got %d argument(s)", len(args))
		}
		if !safeArg.MatchString(args[1]) || !safeArg.MatchString(args[2]) {
			return fmt.Errorf("booking id and operation may only contain letters, digits, - and _")
		}
		*siteName, bookingID = args[0], args[1]
		*planPath = filepath.Join("plans", args[0], args[2]+".json")
		if _, err := os.Stat(*planPath); err != nil {
			return fmt.Errorf("site %q has no operation %q (expected %s)", args[0], args[2], *planPath)
		}
	}

	if *listAction {
		for _, k := range action.Kinds() {
			fmt.Println(k)
		}
		return nil
	}

	var (
		profile *site.Profile
		config  *site.Config
		err     error
	)
	if *listSites {
		config, err = site.Load(*sitesPath)
		if err != nil {
			return err
		}
		for _, name := range config.Names() {
			fmt.Println(name)
		}
		return nil
	}
	if *siteName != "" {
		config, err = site.Load(*sitesPath)
		if err != nil {
			return err
		}
		profile, err = config.Get(*siteName)
		if err != nil {
			return err
		}
	}

	allowedDomains := splitDomains(*domains)
	if profile != nil {
		allowedDomains = append(profile.Domains, allowedDomains...)
	}
	pol := policy.New(allowedDomains, *maxSteps)
	if len(pol.AllowedDomains) == 0 {
		return fmt.Errorf("provide -site (with domains in the sites config) or -domains")
	}

	auditor, closeAudit, err := audit.Open(*auditPath)
	if err != nil {
		return err
	}
	defer closeAudit()

	plan, err := buildPlan(*planPath, *task, profile)
	if err != nil {
		return err
	}
	if bookingID != "" {
		fillBookingID(&plan, bookingID)
	}
	if profile != nil {
		if err := profile.Resolve(&plan); err != nil {
			return err
		}
	}

	r := &runner.Runner{
		Policy:  pol,
		Auditor: auditor,
		Site:    profile,
		DryRun:  *dryRun,
		Out:     os.Stdout,
	}
	if mailbox, ok, err := otp.FromEnv(); err != nil {
		return err
	} else if ok {
		if profile != nil && profile.OTPEmail != nil {
			if err := mailbox.ForSite(profile.OTPEmail.From, profile.OTPEmail.Pattern); err != nil {
				return err
			}
		}
		r.OTP = mailbox.Fetch
	} else if profile != nil && profile.OTPEmail != nil {
		fmt.Println("note: no OTP mailbox in .env (OTP_SOURCE=applemail, or OTP_EMAIL+OTP_APP_PASSWORD); you will be asked to type the login code by hand")
	}
	// OTP/CAPTCHA always wait for the human; -yes only skips approval prompts.
	interactive := approval.NewStdin(os.Stdin, os.Stdout)
	r.Approver, r.Handoff = interactive, interactive
	if *yes || *dryRun {
		r.Approver = approval.Auto{Allow: true}
	}

	if !*dryRun {
		exec, err := executor.New(executor.Options{
			Headless:   *headless,
			TimeoutMS:  float64(*timeoutSec) * 1000,
			ProfileDir: *profileDir,
			SlowMoMS:   float64(*slowMo),
		})
		if err != nil {
			return err
		}
		defer exec.Close()
		r.Executor = exec
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	results, err := r.Run(ctx, plan)
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Println("dry-run complete: plan is valid and was not executed")
		return nil
	}
	fmt.Printf("completed %d step(s)\n", len(results))
	return nil
}

func buildPlan(planPath, task string, profile *site.Profile) (action.Plan, error) {
	req := planner.Request{Task: task, Site: profile}
	if planPath != "" {
		return planner.File{Path: planPath}.Plan(context.Background(), req)
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if task == "" || apiKey == "" {
		return action.Plan{}, fmt.Errorf("provide --plan, or both --task and OPENAI_API_KEY for AI planning")
	}
	return planner.LLM{
		APIKey:  apiKey,
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:   os.Getenv("OPENAI_MODEL"),
	}.Plan(context.Background(), req)
}

func splitDomains(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if d := strings.TrimSpace(p); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// safeArg keeps CLI input from escaping the plans dir or injecting into URLs.
var safeArg = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// today matches {{TODAY+N}}: the date N days from now, as dd/mm/yyyy.
// ponytail: one date format for all sites; make it per-site when a site needs another.
var today = regexp.MustCompile(`\{\{TODAY\+(\d+)\}\}`)

func fillBookingID(plan *action.Plan, id string) {
	fill := func(v string) string {
		v = strings.ReplaceAll(v, "{{BOOKING_ID}}", id)
		return today.ReplaceAllStringFunc(v, func(m string) string {
			days, _ := strconv.Atoi(today.FindStringSubmatch(m)[1])
			return time.Now().AddDate(0, 0, days).Format("02/01/2006")
		})
	}
	plan.Task = fill(plan.Task)
	for i := range plan.Steps {
		s := &plan.Steps[i]
		s.URL, s.Selector, s.Value, s.Path, s.Description = fill(s.URL), fill(s.Selector), fill(s.Value), fill(s.Path), fill(s.Description)
	}
}

// loadDotEnv sets KEY=VALUE lines from path without overriding the real environment.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, value)
		}
	}
}
