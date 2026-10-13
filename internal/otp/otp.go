// Package otp reads a one-time login code from an IMAP mailbox.
package otp

import (
	"context"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
)

// Placeholder in a fill value that is replaced with the code from email.
const Placeholder = "{{EMAIL_OTP}}"

var defaultPattern = regexp.MustCompile(`(?i)(?:code|otp|pin)\D{0,40}?\b(\d{4,8})\b`)

type Mailbox struct {
	Addr     string // host:port, implicit TLS
	User     string
	Password string // Google: an app password, not the account password
	From     string // optional sender filter
	Pattern  *regexp.Regexp
	Timeout  time.Duration
	// AppleMail reads from the macOS Mail app (already signed in) instead of IMAP.
	AppleMail bool
}

// FromEnv builds a Mailbox from OTP_EMAIL and OTP_APP_PASSWORD (plus optional
// OTP_IMAP_ADDR, OTP_FROM, OTP_PATTERN). ok is false when no mailbox is configured.
func FromEnv() (m *Mailbox, ok bool, err error) {
	user, pass := os.Getenv("OTP_EMAIL"), os.Getenv("OTP_APP_PASSWORD")
	appleMail := os.Getenv("OTP_SOURCE") == "applemail"
	if !appleMail && (user == "" || pass == "") {
		return nil, false, nil
	}
	m = &Mailbox{Addr: "imap.gmail.com:993", User: user, Password: pass, From: os.Getenv("OTP_FROM"), Pattern: defaultPattern, Timeout: 2 * time.Minute, AppleMail: appleMail}
	if v := os.Getenv("OTP_IMAP_ADDR"); v != "" {
		m.Addr = v
	}
	if v := os.Getenv("OTP_PATTERN"); v != "" {
		if m.Pattern, err = regexp.Compile(v); err != nil {
			return nil, false, fmt.Errorf("OTP_PATTERN: %w", err)
		}
	}
	return m, true, nil
}

// ForSite applies a site's sender filter and code pattern, when set.
func (m *Mailbox) ForSite(from, pattern string) error {
	if from != "" {
		m.From = from
	}
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("otp_email pattern: %w", err)
		}
		m.Pattern = re
	}
	return nil
}

// Fetch polls until an email received after since contains a code.
func (m *Mailbox) Fetch(ctx context.Context, since time.Time) (string, error) {
	deadline := time.Now().Add(m.Timeout)
	for {
		code, err := m.check(since)
		if err != nil || code != "" {
			return code, err
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("no OTP email arrived in %s within %s", m.User, m.Timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (m *Mailbox) check(since time.Time) (string, error) {
	if m.AppleMail {
		return m.checkAppleMail(since)
	}
	c, err := client.DialTLS(m.Addr, nil)
	if err != nil {
		return "", fmt.Errorf("connect %s: %w", m.Addr, err)
	}
	defer c.Logout()
	if err := c.Login(m.User, m.Password); err != nil {
		return "", fmt.Errorf("mailbox login %s: %w", m.User, err)
	}
	if _, err := c.Select("INBOX", true); err != nil {
		return "", fmt.Errorf("open inbox: %w", err)
	}

	crit := imap.NewSearchCriteria()
	crit.Since = since.Add(-24 * time.Hour) // IMAP SINCE is date-only; exact filter below
	if m.From != "" {
		crit.Header.Add("From", m.From)
	}
	uids, err := c.UidSearch(crit)
	if err != nil || len(uids) == 0 {
		return "", err
	}

	// Newest first, only mail that arrived after since.
	var recent []*imap.Message
	err = fetch(c, uids, []imap.FetchItem{imap.FetchUid, imap.FetchInternalDate}, func(msg *imap.Message) {
		if !msg.InternalDate.Before(since) {
			recent = append(recent, msg)
		}
	})
	if err != nil {
		return "", err
	}
	sort.Slice(recent, func(i, j int) bool { return recent[i].InternalDate.After(recent[j].InternalDate) })

	section := &imap.BodySectionName{Peek: true}
	for _, msg := range recent {
		var code string
		err := fetch(c, []uint32{msg.Uid}, []imap.FetchItem{section.FetchItem()}, func(full *imap.Message) {
			if body := full.GetBody(section); body != nil {
				code = Extract(plainText(body), m.Pattern)
			}
		})
		if err != nil || code != "" {
			return code, err
		}
	}
	return "", nil
}

// appleMailScript returns subject+content of the newest inbox message from
// argv[1] received in the last argv[2] seconds, or "".
const appleMailScript = `on run argv
	set fromAddr to item 1 of argv
	set cutoff to (current date) - ((item 2 of argv) as integer)
	tell application "Mail"
		check for new mail
		set newest to missing value
		repeat with m in (messages of inbox whose sender contains fromAddr and date received > cutoff)
			if newest is missing value or (date received of m) > (date received of newest) then set newest to m
		end repeat
		if newest is missing value then return ""
		return (subject of newest) & linefeed & (content of newest)
	end tell
end run`

func (m *Mailbox) checkAppleMail(since time.Time) (string, error) {
	if m.From == "" {
		return "", fmt.Errorf("Apple Mail OTP needs a sender filter (otp_email.from in sites.yaml or OTP_FROM)")
	}
	secs := fmt.Sprint(int(time.Since(since).Seconds()) + 1)
	cmd := exec.Command("osascript", "-", m.From, secs)
	cmd.Stdin = strings.NewReader(appleMailScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read Apple Mail (allow Automation access to Mail if macOS asks): %v: %s", err, strings.TrimSpace(string(out)))
	}
	return Extract(string(out), m.Pattern), nil
}

func fetch(c *client.Client, uids []uint32, items []imap.FetchItem, each func(*imap.Message)) error {
	seq := new(imap.SeqSet)
	seq.AddNum(uids...)
	msgs := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(seq, items, msgs) }()
	for msg := range msgs {
		each(msg)
	}
	return <-done
}

var tags = regexp.MustCompile(`<[^>]*>`)

// plainText returns the subject and all text parts of a MIME message, with HTML tags stripped.
func plainText(r io.Reader) string {
	mr, err := mail.CreateReader(r)
	if err != nil {
		return ""
	}
	var b strings.Builder
	subject, _ := mr.Header.Subject()
	b.WriteString(subject + "\n")
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		if _, ok := part.Header.(*mail.InlineHeader); ok {
			data, _ := io.ReadAll(part.Body)
			b.WriteString(html.UnescapeString(tags.ReplaceAllString(string(data), " ")) + "\n")
		}
	}
	return b.String()
}

// Extract returns the first capture group of pattern in text, or "".
func Extract(text string, pattern *regexp.Regexp) string {
	if pattern == nil {
		pattern = defaultPattern
	}
	if m := pattern.FindStringSubmatch(text); len(m) > 1 {
		return m[1]
	}
	return ""
}
