package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc"
	"github.com/nazarioz/oidc-radius-bridge/config"
	"github.com/nazarioz/oidc-radius-bridge/pkg/logger"
	"golang.org/x/oauth2"
)

type OIDCProvider struct {
	provider  *oidc.Provider
	config    *oauth2.Config
	logger    *logger.Logger
	appConfig *config.Config
}

func NewOIDCProvider(cfg *config.Config, log *logger.Logger) (*OIDCProvider, error) {
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, cfg.OIDCProviderURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %v", err)
	}

	oauthConfig := &oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	return &OIDCProvider{
		provider:  provider,
		config:    oauthConfig,
		logger:    log,
		appConfig: cfg,
	}, nil
}

func (p *OIDCProvider) Authenticate(username, password string) (*oauth2.Token, error) {
	ctx := context.Background()

	token, err := p.config.PasswordCredentialsToken(ctx, username, password)
	if err != nil {
		p.logger.Error("OIDC authentication failed: %v", err)
		return nil, fmt.Errorf("authentication failed: %v", err)
	}

	p.logger.Info("Successfully authenticated user: %s", username)
	return token, nil
}
