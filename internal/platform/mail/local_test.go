package mail

import "testing"

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
