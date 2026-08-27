package account

import (
	"context"
	"net/url"
	"testing"
	"time"

	"noplanner/backend/user/domains/authtransaction"
	"noplanner/backend/user/modules/googleoauth"
	"noplanner/backend/user/modules/token"

	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	userv1 "noplanner/backend/proto/dist/golang/user/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeGoogle struct {
	state     string
	challenge string
	verifier  string
	identity  googleoauth.Identity
	err       error
}

func testTokens() *token.Manager {
	return token.New("0123456789abcdef0123456789abcdef", "test-issuer", "test-audience", time.Hour)
}

func (f *fakeGoogle) AuthorizationURL(state, challenge string) (string, error) {
	f.state, f.challenge = state, challenge
	if f.err != nil {
		return "", f.err
	}
	return "https://accounts.example/auth?state=" + url.QueryEscape(state), nil
}

func (f *fakeGoogle) Exchange(_ context.Context, _ string, verifier string) (googleoauth.Identity, error) {
	f.verifier = verifier
	return f.identity, f.err
}

func TestGoogleLoginSuccess(t *testing.T) {
	provider := &fakeGoogle{identity: googleoauth.Identity{Subject: "google-1", Email: "dev@example.com", Name: "Developer"}}
	service := New("user", "0.1.0", authtransaction.NewStore(10*time.Minute), provider, testTokens())
	begin, err := service.BeginGoogleLogin(context.Background(), &userv1.BeginGoogleLoginRequest{})
	if err != nil || begin.AuthorizationUrl == "" || provider.state == "" || provider.challenge == "" {
		t.Fatalf("BeginGoogleLogin() = %#v, %v", begin, err)
	}
	complete, err := service.CompleteGoogleLogin(context.Background(), &userv1.CompleteGoogleLoginRequest{Code: "code", State: provider.state})
	if err != nil {
		t.Fatal(err)
	}
	if provider.verifier == "" || complete.User.Email != "dev@example.com" || complete.User.Subject != "google-1" || complete.AccessToken == "" || complete.ExpiresAt == nil {
		t.Fatalf("unexpected login result: %#v", complete)
	}
}

func TestGoogleLoginRejectsInvalidAndReplayedState(t *testing.T) {
	provider := &fakeGoogle{identity: googleoauth.Identity{Subject: "google-1", Email: "dev@example.com"}}
	service := New("user", "0.1.0", authtransaction.NewStore(10*time.Minute), provider, testTokens())
	request := &userv1.CompleteGoogleLoginRequest{Code: "code", State: "unknown"}
	if _, err := service.CompleteGoogleLogin(context.Background(), request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("invalid state code = %s", status.Code(err))
	}
	_, _ = service.BeginGoogleLogin(context.Background(), &userv1.BeginGoogleLoginRequest{})
	request.State = provider.state
	if _, err := service.CompleteGoogleLogin(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteGoogleLogin(context.Background(), request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("replayed state code = %s", status.Code(err))
	}
}

func TestGoogleLoginReportsMissingConfiguration(t *testing.T) {
	service := New("user", "0.1.0", authtransaction.NewStore(10*time.Minute), googleoauth.New("", "", ""), testTokens())
	_, err := service.BeginGoogleLogin(context.Background(), &userv1.BeginGoogleLoginRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("missing configuration code = %s", status.Code(err))
	}
	detail, ok := status.Convert(err).Details()[0].(*commonv1.ErrorDetail)
	if !ok || detail.Code != commonv1.ErrorCode_ERROR_CODE_CONFIGURATION_REQUIRED {
		t.Fatalf("unexpected error detail: %#v", detail)
	}
}

func TestGoogleLoginReportsMissingJWTConfiguration(t *testing.T) {
	provider := &fakeGoogle{identity: googleoauth.Identity{Subject: "google-1", Email: "dev@example.com"}}
	service := New("user", "0.1.0", authtransaction.NewStore(10*time.Minute), provider, token.New("", "", "", 0))
	_, _ = service.BeginGoogleLogin(context.Background(), &userv1.BeginGoogleLoginRequest{})
	_, err := service.CompleteGoogleLogin(context.Background(), &userv1.CompleteGoogleLoginRequest{Code: "code", State: provider.state})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("missing JWT configuration code = %s", status.Code(err))
	}
}
