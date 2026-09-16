package notifier

import (
	"context"
	"log/slog"
)

// LogNotifier is a NotificationService that writes to structured logs
// instead of calling a real provider. It's the default wiring for local
// development and for exercising the approval workflow end-to-end before
// provider credentials exist.
//
// To go live: implement NotificationService against a real provider —
// SMTP/Resend/SendGrid for SendEmail, Twilio/SNS for SendSMS (both
// suggested in the proposal's tech stack) — and swap the constructor
// call in cmd/api/main.go. Nothing in the service or handler layers
// needs to change; they only know about the interface.
type LogNotifier struct{}

func NewLogNotifier() *LogNotifier {
	return &LogNotifier{}
}

func (n *LogNotifier) SendEmail(ctx context.Context, to, subject, body string) error {
	slog.InfoContext(ctx, "notifier: email (logged, not sent)",
		"to", to, "subject", subject, "body_preview", preview(body))
	return nil
}

func (n *LogNotifier) SendSMS(ctx context.Context, to, body string) error {
	slog.InfoContext(ctx, "notifier: sms (logged, not sent)",
		"to", to, "body_preview", preview(body))
	return nil
}

func preview(s string) string {
	const max = 120
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
