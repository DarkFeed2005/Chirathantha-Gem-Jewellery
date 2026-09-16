package notifier

import (
	"fmt"
	"strings"

	"gemstore/internal/models"
)

// BuildOrderDecisionMessage composes the subject/body for a custom-order
// confirm or decline notification (Section 8: "the system automatically
// sends an email or SMS to the customer with the decision and next
// steps"). The same body is used for both channels; SMS implementations
// should trim it to fit provider limits rather than this function
// producing two variants.
func BuildOrderDecisionMessage(order models.Order, decision models.ApprovalDecision, notes *string) (subject, body string) {
	ref := order.ID.String()[:8]

	var sb strings.Builder

	if decision == models.DecisionConfirmed {
		subject = fmt.Sprintf("Your custom order #%s has been confirmed", ref)
		sb.WriteString("Good news — your custom jewelry order has been reviewed and confirmed. ")
		sb.WriteString("It now moves into production and we'll notify you again once it ships.\n\n")
	} else {
		subject = fmt.Sprintf("Update on your custom order #%s", ref)
		sb.WriteString("We're sorry — after review, we're unable to proceed with your custom order as specified.\n\n")
	}

	sb.WriteString(fmt.Sprintf("Order reference: #%s\n", ref))
	sb.WriteString(fmt.Sprintf("Order total: $%.2f\n", order.TotalAmount))

	if notes != nil && strings.TrimSpace(*notes) != "" {
		sb.WriteString(fmt.Sprintf("\nNote from our team: %s\n", strings.TrimSpace(*notes)))
	}

	sb.WriteString("\nQuestions? Just reply to this message.")

	return subject, sb.String()
}
