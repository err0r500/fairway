package main

import (
	"context"
	"log/slog"
)

type LoggingEmailSender struct{}

func (s *LoggingEmailSender) SendWelcomeEmail(ctx context.Context, email, name string) error {
	slog.Info("sending welcome email", "email", email, "name", name)
	return nil
}
