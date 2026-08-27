package googleoauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestAuthorizationURLRequiresConfiguration(t *testing.T) {
	if _, err := New("", "", "").AuthorizationURL("state", "challenge"); err != ErrUnavailable {
		t.Fatalf("AuthorizationURL() error = %v, want ErrUnavailable", err)
	}
}

func TestExchangeReturnsVerifiedGoogleIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			if request.FormValue("code_verifier") != "verifier" {
				t.Errorf("code_verifier = %q", request.FormValue("code_verifier"))
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"access_token":"access-secret","token_type":"Bearer"}`))
		case "/userinfo":
			if request.Header.Get("Authorization") != "Bearer access-secret" {
				t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"sub":"google-1","email":"dev@example.com","email_verified":true,"name":"Developer"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := &Client{
		config: oauth2.Config{
			ClientID: "client", ClientSecret: "secret", RedirectURL: "http://localhost/callback",
			Endpoint: oauth2.Endpoint{AuthURL: server.URL + "/auth", TokenURL: server.URL + "/token"},
		},
		userinfo: server.URL + "/userinfo",
	}
	identity, err := client.Exchange(context.Background(), "code", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Subject != "google-1" || identity.Email != "dev@example.com" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
}

func TestExchangeRejectsUnverifiedEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(request.URL.Path, "/token") {
			_, _ = response.Write([]byte(`{"access_token":"access-secret","token_type":"Bearer"}`))
			return
		}
		_, _ = response.Write([]byte(`{"sub":"google-1","email":"dev@example.com","email_verified":false}`))
	}))
	defer server.Close()
	client := &Client{
		config:   oauth2.Config{ClientID: "client", ClientSecret: "secret", RedirectURL: "http://localhost/callback", Endpoint: oauth2.Endpoint{TokenURL: server.URL + "/token"}},
		userinfo: server.URL + "/userinfo",
	}
	if _, err := client.Exchange(context.Background(), "code", "verifier"); err == nil {
		t.Fatal("Exchange() unexpectedly accepted unverified email")
	}
}
