package otp

import (
	"os"
	"regexp"
	"testing"
	"time"
)

// Run with OTP_LIVE=1 to read the newest BS email from the last 6h in Apple Mail.
func TestAppleMailLive(t *testing.T) {
	if os.Getenv("OTP_LIVE") == "" {
		t.Skip("set OTP_LIVE=1")
	}
	m := &Mailbox{AppleMail: true, From: "no-reply@usbair.com", Pattern: regexp.MustCompile(bsPattern)}
	code, err := m.checkAppleMail(time.Now().Add(-6 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found code of length %d", len(code))
	if code == "" {
		t.Fatal("no code found")
	}
}
