package auth

import (
	"context"
)

// Service defines the authentication service interface
type Service interface {
	Authenticate(ctx context.Context, username, password string) error
}

// OIDCService implements the Service interface using OIDC
type OIDCService struct {
	provider *OIDCProvider
}

func NewOIDCService(provider *OIDCProvider) *OIDCService {
	return &OIDCService{
		provider: provider,
	}
}

func (s *OIDCService) Authenticate(ctx context.Context, username, password string) error {
	_, err := s.provider.Authenticate(username, password)
	return err
}
