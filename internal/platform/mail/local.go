// Package mail implements local SMTP delivery. External SMTP credentials and
// production delivery are intentionally outside stage 04.
package mail

import (
	"context"
	"fmt"
	"mime"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
	"time"
)

type LocalSMTP struct{ Address, From string }

func NewLocalSMTP(address, from string) (*LocalSMTP, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return nil, fmt.Errorf("SMTP_ADDR must use a loopback IP")
	}
	a, err := netmail.ParseAddress(from)
	if err != nil || a.Address != from || strings.ContainsAny(from, "\r\n") {
		return nil, fmt.Errorf("MAIL_FROM must be a plain email address")
	}
	return &LocalSMTP{Address: address, From: from}, nil
}

func (m *LocalSMTP) Send(ctx context.Context, to, subject, body string) error {
	a, err := netmail.ParseAddress(to)
	if err != nil || a.Address != to || strings.ContainsAny(to, "\r\n") || strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("invalid mail envelope")
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", m.Address)
	if err != nil {
		return fmt.Errorf("local SMTP unavailable")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("local SMTP unavailable")
	}
	client, err := smtp.NewClient(conn, "localhost")
	if err != nil {
		return fmt.Errorf("local SMTP unavailable")
	}
	defer client.Close()
	if err := client.Mail(m.From); err != nil {
		return fmt.Errorf("local SMTP rejected message")
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("local SMTP rejected message")
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("local SMTP rejected message")
	}
	_, err = fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s", m.From, to, mime.QEncoding.Encode("UTF-8", subject), strings.ReplaceAll(body, "\n", "\r\n"))
	if err != nil {
		_ = w.Close()
		return fmt.Errorf("local SMTP delivery failed")
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("local SMTP delivery failed")
	}
	// DATA's successful final reply confirms acceptance. A failed QUIT must not
	// misreport an already accepted message as unsent.
	_ = client.Quit()
	return nil
}
