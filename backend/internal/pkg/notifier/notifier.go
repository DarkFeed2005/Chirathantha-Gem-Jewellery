// Package notifier defines the outbound customer-notification contract
// (Section 6 of the proposal: "Notification Service ... integrates with
// an email service and/or SMS gateway") and a default implementation.
//
// The interface is deliberately about channels (email, SMS), not about
// order-approval specifically, so it stays reusable for any future
// transactional message — shipping updates, password resets, etc.
// Order-decision message composition lives in message.go, one layer up
// from the raw send calls.
package notifier

import "context"

// NotificationService sends a message to a customer over a given
// channel. Implementations must be safe for concurrent use — the
// approval service invokes these from a goroutine detached from the
// originating HTTP request, and multiple approvals can be in flight at
// once.
type NotificationService interface {
	// SendEmail delivers subject/body to the given address.
	SendEmail(ctx context.Context, to, subject, body string) error

	// SendSMS delivers body as a text message to the given phone number.
	// Implementations backed by a real gateway (Twilio, SNS, ...) are
	// responsible for any provider-specific length/segment limits.
	SendSMS(ctx context.Context, to, body string) error
}
