package googleoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var ErrUnavailable = errors.New("google oauth is not configured")

type Identity struct {
	Subject    string
	Email      string
	Name       string
	PictureURL string
}

type Client struct {
	config   oauth2.Config
	userinfo string
}

func New(clientID, clientSecret, redirectURL string) *Client {
	return &Client{
		config: oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Endpoint: google.Endpoint,
			Scopes:   []string{"openid", "email", "profile"},
		},
		userinfo: "https://openidconnect.googleapis.com/v1/userinfo",
	}
}

func (c *Client) AuthorizationURL(state, challenge string) (string, error) {
	if !c.configured() {
		return "", ErrUnavailable
	}
	return c.config.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "select_account"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	), nil
}

func (c *Client) Exchange(ctx context.Context, code, verifier string) (Identity, error) {
	if !c.configured() {
		return Identity{}, ErrUnavailable
	}
	token, err := c.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("exchange google authorization code: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userinfo, nil)
	if err != nil {
		return Identity{}, fmt.Errorf("create google userinfo request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	response, err := c.config.Client(ctx, token).Do(request)
	if err != nil {
		return Identity{}, fmt.Errorf("request google userinfo: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("google userinfo returned status %d", response.StatusCode)
	}
	var payload struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Identity{}, fmt.Errorf("decode google userinfo: %w", err)
	}
	if payload.Subject == "" || payload.Email == "" || !payload.EmailVerified {
		return Identity{}, errors.New("google identity is incomplete or email is unverified")
	}
	if _, err := url.ParseRequestURI(payload.Picture); payload.Picture != "" && err != nil {
		payload.Picture = ""
	}
	return Identity{Subject: payload.Subject, Email: payload.Email, Name: payload.Name, PictureURL: payload.Picture}, nil
}

func (c *Client) configured() bool {
	return c.config.ClientID != "" && c.config.ClientSecret != "" && c.config.RedirectURL != ""
}
