package app

import (
	"fmt"
	"html"
	"strings"
	"time"

	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/mail"
)

// The account emails: plain text plus a minimal HTML alternative. Every
// interpolated value is escaped in the HTML part.

func passwordResetEmail(u domain.User, link string, ttl time.Duration) mail.Message {
	text := fmt.Sprintf(`Hi %s,

Someone asked to reset the password of your LevelUp account. To choose a new password, open this link:

%s

The link works once and expires in %s. If you did not ask for this, ignore this email: your password stays the same.
`, u.Name, link, humanDuration(ttl))
	body := fmt.Sprintf(`<p>Hi %s,</p>
<p>Someone asked to reset the password of your LevelUp account.</p>
<p><a href="%s">Choose a new password</a></p>
<p>The link works once and expires in %s. If you did not ask for this, ignore this email: your password stays the same.</p>`,
		html.EscapeString(u.Name), html.EscapeString(link), humanDuration(ttl))
	return mail.Message{To: u.Email, Subject: "Reset your LevelUp password", Text: text, HTML: wrapHTML(body)}
}

func verificationEmail(u domain.User, link string, ttl time.Duration) mail.Message {
	text := fmt.Sprintf(`Hi %s,

Please confirm that this is your email address by opening this link:

%s

The link expires in %s.
`, u.Name, link, humanDuration(ttl))
	body := fmt.Sprintf(`<p>Hi %s,</p>
<p>Please confirm that this is your email address.</p>
<p><a href="%s">Verify my email</a></p>
<p>The link expires in %s.</p>`,
		html.EscapeString(u.Name), html.EscapeString(link), humanDuration(ttl))
	return mail.Message{To: u.Email, Subject: "Verify your LevelUp email address", Text: text, HTML: wrapHTML(body)}
}

func invitationEmail(inv domain.Invitation, tenantName, inviterName, link string) mail.Message {
	who := "You have"
	if inviterName != "" {
		who = inviterName + " has"
	}
	expires := inv.ExpiresAt.UTC().Format("2 January 2006, 15:04 MST")
	text := fmt.Sprintf(`Hello,

%s invited you to join %s on LevelUp. To accept and choose your password, open this link:

%s

The invitation expires on %s.
`, who, tenantName, link, expires)
	body := fmt.Sprintf(`<p>Hello,</p>
<p>%s invited you to join <strong>%s</strong> on LevelUp.</p>
<p><a href="%s">Accept the invitation</a></p>
<p>The invitation expires on %s.</p>`,
		html.EscapeString(who), html.EscapeString(tenantName), html.EscapeString(link), html.EscapeString(expires))
	return mail.Message{
		To: inv.Email, Subject: "You are invited to " + oneLine(tenantName) + " on LevelUp",
		Text: text, HTML: wrapHTML(body),
	}
}

func wrapHTML(body string) string {
	return `<!doctype html><html><body style="font-family:sans-serif;line-height:1.5;color:#111">` + body + `</body></html>`
}

// oneLine keeps a user-supplied value out of the header grammar.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func humanDuration(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0 && d >= 24*time.Hour:
		return plural(int(d/(24*time.Hour)), "day")
	case d%time.Hour == 0 && d >= time.Hour:
		return plural(int(d/time.Hour), "hour")
	default:
		return plural(int(d.Round(time.Minute)/time.Minute), "minute")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
