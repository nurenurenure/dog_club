package mail

import (
	"context"
	"testing"
)

func TestLocalSMTPRejectsExternalEndpointsAndHeaderInjection(t *testing.T) {
	for _, test := range []struct{ addr, from string }{
		{"smtp.example.com:587", "club@example.com"}, {"203.0.113.1:25", "club@example.com"},
		{"127.0.0.1:1025", "club@example.com\r\nBcc: another@example.com"},
	} {
		if _, err := NewLocalSMTP(test.addr, test.from); err == nil {
			t.Fatal("unsafe SMTP configuration accepted")
		}
	}
	if _, err := NewLocalSMTP("127.0.0.1:1025", "club@dogclub.test"); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationRejectsInjectedMessageIDBeforeConnecting(t *testing.T) {
	m, err := NewLocalSMTP("127.0.0.1:1", "club@dogclub.test")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SendNotification(context.Background(), "owner@dogclub.test", "Subject", "Body", "<id@dogclub.local>\r\nBcc: other@dogclub.test"); err == nil {
		t.Fatal("message ID header injection accepted")
	}
}

func TestComposeSMTPAllowsOnlyExplicitLocalMailpit(t *testing.T) {
	for _, address := range []string{"mailpit:1025", "mailpit:25", "smtp.example.com:1025"} {
		_, err := NewLocalSMTP(address, "club@dogclub.test", true)
		if (err == nil) != (address == "mailpit:1025") {
			t.Fatal("Compose SMTP boundary failed")
		}
	}
	if _, err := NewLocalSMTP("mailpit:1025", "club@dogclub.test"); err == nil {
		t.Fatal("implicit Compose mode")
	}
}
