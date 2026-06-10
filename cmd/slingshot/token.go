package main

import (
	"fmt"

	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
)

// loadToken loads the config and returns a WeChat access token.
func loadToken() (string, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return "", fmt.Errorf("getting access token: %w", err)
	}
	return token, nil
}
