package otp

import (
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	cases := map[string]string{
		"Your verification code is: 482913\n© 2026 US-Bangla": "482913",
		"Votre code de vérification : <b>7741</b>":            "7741",
		"OTP 123456 valid for 5 minutes":                      "123456",
		"Copyright 2026, no code here":                        "",
	}
	for text, want := range cases {
		if got := Extract(tags.ReplaceAllString(text, " "), nil); got != want {
			t.Errorf("Extract(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestPlainTextDecodesHTML(t *testing.T) {
	raw := "Subject: Login\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n<p>Your code=3A <b>905112</b></p>\r\n"
	if got := Extract(plainText(strings.NewReader(raw)), nil); got != "905112" {
		t.Fatalf("got %q", got)
	}
}

func TestBSEmail(t *testing.T) {
	raw := "Subject: Verification Code for Login ID#Atik01 at 12/10/2026 05:40 LT\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		"<h2 style=\"color:red\">Votre code de v&eacute;rification</h2><div class=\"box\">JMBERM</div><p>Ce code sera valable 5 minutes.</p>\r\n"
	m := &Mailbox{}
	if err := m.ForSite("no-reply@usbair.com", bsPattern); err != nil {
		t.Fatal(err)
	}
	if got := Extract(plainText(strings.NewReader(raw)), m.Pattern); got != "JMBERM" {
		t.Fatalf("got %q", got)
	}
}

// Keep in sync with sites.yaml / sites.example.yaml (bs.otp_email.pattern).
const bsPattern = `(?i:code de v[ée]rification|verification code)\W+([A-Z0-9]{6})\b`
