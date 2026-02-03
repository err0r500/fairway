package automate

import (
	"context"

	"github.com/err0r500/fairway"
)

// AllDeps contains all services needed by automations
type AllDeps struct {
	EmailSender EmailSender
}

// EmailSender interface
type EmailSender interface {
	SendWelcomeEmail(ctx context.Context, email, name string) error
}

var Registry = fairway.AutomationRegistry[AllDeps]{}
