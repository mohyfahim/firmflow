package service

import "context"

type SSOProvider interface {
	Name() string
	BuildLoginURL(state string) (string, error)
	HandleCallback(ctx context.Context, code string) (providerUserID string, email string, err error)
}
