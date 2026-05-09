package mailer

import "context"

type Mailer interface {
	SendEmailVerification(ctx context.Context, toEmail string, token string) error
	SendPasswordReset(ctx context.Context, toEmail string, token string) error
}

type NoopMailer struct{}

func (NoopMailer) SendEmailVerification(_ context.Context, _ string, _ string) error { return nil }
func (NoopMailer) SendPasswordReset(_ context.Context, _ string, _ string) error     { return nil }
