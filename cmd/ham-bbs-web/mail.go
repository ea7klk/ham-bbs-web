package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

// Outbound email.
//
// The classic SSH BBS sends three kinds of transactional mail: the activation
// code at registration, a password-reset link, and the two links that confirm an
// email change. With this front end able to register and reset accounts on its
// own, it has to send the same mail, so it reads the same environment variables
// with the same defaults — a deployment configures one set of SMTP values and
// both applications use them.
//
// Sending is synchronous with a short timeout. A registration is not the place
// to run a queue, and a hung SMTP conversation must not hold a request open.

const (
	mailDialTimeout  = 10 * time.Second
	mailTotalTimeout = 20 * time.Second
	// mailBodyWidth keeps message bodies readable in monospace and plain-text
	// clients without hard-wrapping URLs, which are appended on their own line.
	mailBodyWidth = 72
)

type mailConfig struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
	// security is "starttls", "tls" or "none".
	security string
	// webBaseURL is the public base URL of this application. Tokenised links
	// point here, never at the SSH service, because the click lands in a browser.
	webBaseURL string
}

type mailMessage struct {
	To      string
	Subject string
	Body    string
}

type mailer struct {
	cfg  mailConfig
	now  func() time.Time
	send func(msg mailMessage) error
}

func newMailerFromEnv() *mailer {
	port, _ := strconv.Atoi(env("BBS_SMTP_PORT", "587"))
	m := &mailer{cfg: mailConfig{
		host:       strings.TrimSpace(os.Getenv("BBS_SMTP_HOST")),
		port:       port,
		username:   strings.TrimSpace(os.Getenv("BBS_SMTP_USERNAME")),
		password:   os.Getenv("BBS_SMTP_PASSWORD"),
		from:       strings.TrimSpace(env("BBS_SMTP_FROM", os.Getenv("BBS_SMTP_USERNAME"))),
		fromName:   env("BBS_SMTP_FROM_NAME", "HAMNET Radio BBS"),
		security:   strings.ToLower(strings.TrimSpace(env("BBS_SMTP_SECURITY", "starttls"))),
		webBaseURL: strings.TrimRight(env("BBS_WEB_BASE_URL", "https://bbs.ea7klk.es"), "/"),
	}, now: time.Now}
	m.send = m.smtpSend
	return m
}

// Problem explains why mail cannot be used, or returns "" when it can.
//
// A deployment that declares an SMTP host but leaves the password at a
// placeholder would otherwise consider itself enabled and then fail every send,
// which is worse than not enabling mail at all: registrations would be blocked
// at activation with no code ever arriving.
func (m *mailer) Problem() string {
	if m == nil {
		return "mailer not initialised"
	}
	if strings.TrimSpace(m.cfg.host) == "" {
		return "BBS_SMTP_HOST is not set"
	}
	if strings.TrimSpace(m.cfg.from) == "" {
		return "BBS_SMTP_FROM is not set"
	}
	if !strings.Contains(m.cfg.from, "@") {
		return "BBS_SMTP_FROM is not an email address"
	}
	if m.cfg.port <= 0 || m.cfg.port > 65535 {
		return "BBS_SMTP_PORT is not a valid port"
	}
	switch m.cfg.security {
	case "starttls", "tls", "none":
	default:
		return "BBS_SMTP_SECURITY must be starttls, tls or none"
	}
	// Authentication is optional (a local relay may not need it), but when a
	// username is given the password must be real.
	if strings.TrimSpace(m.cfg.username) != "" && isPlaceholderValue(m.cfg.password) {
		return "BBS_SMTP_PASSWORD is empty or still a placeholder"
	}
	if url := strings.TrimSpace(m.cfg.webBaseURL); url != "" {
		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
			return "BBS_WEB_BASE_URL must be an absolute http(s) URL"
		}
	}
	return ""
}

// Enabled reports whether outbound mail is usable. Without a usable mailer the
// account flows fall back to the behaviour the SSH BBS uses: activation is
// skipped, and a reset request reports that mail is not configured.
func (m *mailer) Enabled() bool {
	return m.Problem() == ""
}

// WebURL builds an absolute link for a tokenised mail.
func (m *mailer) WebURL(path string) string {
	base := "https://bbs.ea7klk.es"
	if m != nil && strings.TrimSpace(m.cfg.webBaseURL) != "" {
		base = m.cfg.webBaseURL
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

func (m *mailer) Send(msg mailMessage) error {
	if !m.Enabled() {
		return errors.New("mail is not configured")
	}
	msg.To = strings.TrimSpace(msg.To)
	if msg.To == "" {
		return errors.New("mail: empty recipient")
	}
	if m.send != nil {
		return m.send(msg)
	}
	return m.smtpSend(msg)
}

func (m *mailer) smtpSend(msg mailMessage) error {
	address := net.JoinHostPort(m.cfg.host, strconv.Itoa(m.cfg.port))
	dialer := &net.Dialer{Timeout: mailDialTimeout}
	deadline := m.now().Add(mailTotalTimeout)
	tlsConfig := &tls.Config{ServerName: m.cfg.host, MinVersion: tls.VersionTLS12}

	conn, err := dialer.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("mail: dial %s: %w", address, err)
	}
	// One deadline for the whole conversation: a server that accepts the
	// connection and then stalls must not hold a request open indefinitely.
	_ = conn.SetDeadline(deadline)
	if m.cfg.security == "tls" {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			_ = conn.Close()
			return fmt.Errorf("mail: tls handshake with %s: %w", address, err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, m.cfg.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: greet %s: %w", address, err)
	}
	defer func() { _ = client.Close() }()

	if m.cfg.security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("mail: server does not offer STARTTLS; set BBS_SMTP_SECURITY=tls or none")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if m.cfg.username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.cfg.username, m.cfg.password, m.cfg.host)); err != nil {
			return fmt.Errorf("mail: auth as %s: %w", m.cfg.username, err)
		}
	}
	if err := client.Mail(m.cfg.from); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("mail: RCPT TO %s: %w", msg.To, err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := writer.Write([]byte(m.render(msg))); err != nil {
		_ = writer.Close()
		return fmt.Errorf("mail: write body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("mail: finish body: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("mail: quit: %w", err)
	}
	return nil
}

func (m *mailer) render(msg mailMessage) string {
	var b strings.Builder
	b.WriteString("From: " + formatAddress(m.cfg.fromName, m.cfg.from) + "\r\n")
	b.WriteString("To: " + formatAddress("", msg.To) + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject) + "\r\n")
	b.WriteString("Date: " + m.now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("\r\n")
	// SMTP requires CRLF line endings and strips bare dots at line starts.
	for _, line := range strings.Split(strings.ReplaceAll(msg.Body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, ".") {
			line = "." + line
		}
		b.WriteString(line + "\r\n")
	}
	return b.String()
}

func formatAddress(name, address string) string {
	name = strings.TrimSpace(sanitizeSingleLineText(name))
	if name == "" {
		return "<" + address + ">"
	}
	return mime.QEncoding.Encode("utf-8", name) + " <" + address + ">"
}

// isPlaceholderValue reports whether a configured secret is empty or still the
// value shipped in .env.example.
func isPlaceholderValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"change-this", "changeme", "placeholder", "your-password"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// wrapBody re-flows a message body to width, leaving blank lines as paragraph
// breaks. A URL longer than the width stays on its own line rather than being
// split, so a reset link in a plain-text client is still clickable.
func wrapBody(text string, width int) string {
	if width < 20 {
		width = 20
	}
	var out []string
	for _, paragraph := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(paragraph) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(paragraph) {
			switch {
			case line == "":
				line = word
			case len([]rune(line))+1+len([]rune(word)) > width:
				out = append(out, line)
				line = word
			default:
				line += " " + word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
