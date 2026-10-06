// Package mail sends transactional email (password resets, invitations,
// player notifications). Modules depend on the Mailer interface; the
// composition root picks the driver from config: "smtp" in production,
// "log" in development and tests (the message is logged, never sent).
package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
	"time"

	"go.uber.org/zap"

	"levelup/internal/shared/errs"
)

// Message is one email. Text is required; HTML is optional.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	// FromName optionally replaces the display name of the configured From
	// address; the address itself never changes (SPF/DKIM stay aligned).
	FromName string
}

// Mailer delivers a message or returns errs.Unavailable on transient
// failure (the caller's retry ladder handles it).
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// Config selects and configures the driver.
type Config struct {
	Driver   string // "log" | "smtp"
	From     string // "LevelUp <no-reply@levelupos.ge>"
	Host     string
	Port     int
	Username string
	Password string
}

// New returns the configured driver.
func New(cfg Config, log *zap.Logger) (Mailer, error) {
	switch cfg.Driver {
	case "", "log":
		return Log{log: log}, nil
	case "smtp":
		if cfg.Host == "" || cfg.From == "" {
			return nil, fmt.Errorf("mail: smtp driver needs MAIL_HOST and MAIL_FROM")
		}
		if cfg.Port == 0 {
			cfg.Port = 587
		}
		return SMTP{cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("mail: unknown driver %q (log | smtp)", cfg.Driver)
	}
}

// Log writes messages to the log instead of sending them.
type Log struct{ log *zap.Logger }

func (l Log) Send(_ context.Context, m Message) error {
	if l.log != nil {
		l.log.Info("mail (log driver, not sent)", zap.String("to", m.To), zap.String("subject", m.Subject), zap.String("text", m.Text))
	}
	return nil
}

// SMTP sends through an authenticated submission server (STARTTLS on 587,
// implicit TLS on 465).
type SMTP struct{ cfg Config }

func (s SMTP) Send(ctx context.Context, m Message) error {
	if strings.ContainsAny(m.To, "\r\n") || strings.ContainsAny(m.Subject, "\r\n") || strings.ContainsAny(m.FromName, "\r\n") {
		return errs.New(errs.Invalid, "mail: header injection")
	}
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))
	d := net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if s.cfg.Port == 465 {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return errs.Wrap(errs.Unavailable, "mail: dial", err)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return errs.Wrap(errs.Unavailable, "mail: smtp handshake", err)
	}
	defer func() { _ = c.Close() }()
	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return errs.Wrap(errs.Unavailable, "mail: starttls", err)
			}
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return errs.Wrap(errs.Unavailable, "mail: auth", err)
		}
	}
	if err := c.Mail(addressOf(s.cfg.From)); err != nil {
		return errs.Wrap(errs.Unavailable, "mail: MAIL FROM", err)
	}
	if err := c.Rcpt(m.To); err != nil {
		return errs.Wrap(errs.Invalid, "mail: recipient rejected", err)
	}
	w, err := c.Data()
	if err != nil {
		return errs.Wrap(errs.Unavailable, "mail: DATA", err)
	}
	if _, err := w.Write(build(fromHeader(s.cfg.From, m.FromName), m)); err != nil {
		return errs.Wrap(errs.Unavailable, "mail: write", err)
	}
	if err := w.Close(); err != nil {
		return errs.Wrap(errs.Unavailable, "mail: send", err)
	}
	return c.Quit()
}

func addressOf(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		return strings.TrimSuffix(from[i+1:], ">")
	}
	return from
}

// fromHeader renders the From header, swapping in name as the display name
// (RFC 2047-encoded by net/mail) when it is set.
func fromHeader(from, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return from
	}
	return (&netmail.Address{Name: name, Address: addressOf(from)}).String()
}

func build(from string, m Message) []byte {
	var b strings.Builder
	boundary := fmt.Sprintf("levelup-%d", time.Now().UnixNano())
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n", from, m.To, m.Subject)
	if m.HTML == "" {
		fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", m.Text)
		return []byte(b.String())
	}
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", boundary, m.Text)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n", boundary, m.HTML)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return []byte(b.String())
}

// Recorder is a test double that keeps sent messages.
type Recorder struct{ Sent []Message }

func (r *Recorder) Send(_ context.Context, m Message) error {
	r.Sent = append(r.Sent, m)
	return nil
}
