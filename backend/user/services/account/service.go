package account

import (
	"context"
	"errors"

	"noplanner/backend/user/domains/authtransaction"
	userdomain "noplanner/backend/user/domains/user"
	"noplanner/backend/user/modules/googleoauth"
	"noplanner/backend/user/modules/rpcerror"
	"noplanner/backend/user/modules/token"

	"google.golang.org/protobuf/types/known/timestamppb"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	userv1 "noplanner/backend/proto/dist/golang/user/v1"
)

type googleProvider interface {
	AuthorizationURL(state, challenge string) (string, error)
	Exchange(ctx context.Context, code, verifier string) (googleoauth.Identity, error)
}

type Service struct {
	userv1.UnimplementedUserServiceServer
	name         string
	version      string
	transactions *authtransaction.Store
	google       googleProvider
	tokens       *token.Manager
}

func New(name, version string, transactions *authtransaction.Store, google googleProvider, tokens *token.Manager) *Service {
	return &Service{name: name, version: version, transactions: transactions, google: google, tokens: tokens}
}

func (s *Service) GetStatus(context.Context, *commonv1.StatusRequest) (*commonv1.StatusResponse, error) {
	return &commonv1.StatusResponse{Service: s.name, Version: s.version, State: "ready"}, nil
}

func (s *Service) BeginGoogleLogin(ctx context.Context, _ *userv1.BeginGoogleLoginRequest) (*userv1.BeginGoogleLoginResponse, error) {
	state, _, challenge, err := s.transactions.Begin()
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "로그인을 시작하지 못했습니다.", true)
	}
	authorizationURL, err := s.google.AuthorizationURL(state, challenge)
	if errors.Is(err, googleoauth.ErrUnavailable) {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_CONFIGURATION_REQUIRED, "Google 로그인이 설정되지 않았습니다.", false)
	}
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_DEPENDENCY_UNAVAILABLE, "Google 로그인을 시작하지 못했습니다.", true)
	}
	return &userv1.BeginGoogleLoginResponse{AuthorizationUrl: authorizationURL}, nil
}

func (s *Service) CompleteGoogleLogin(ctx context.Context, request *userv1.CompleteGoogleLoginRequest) (*userv1.CompleteGoogleLoginResponse, error) {
	if request.GetCode() == "" || request.GetState() == "" {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "인증 코드와 state가 필요합니다.", false)
	}
	verifier, err := s.transactions.Consume(request.GetState())
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "유효하지 않거나 만료된 로그인 요청입니다.", false)
	}
	identity, err := s.google.Exchange(ctx, request.GetCode(), verifier)
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "Google 인증에 실패했습니다.", false)
	}
	user := userdomain.GoogleIdentity{Subject: identity.Subject, Email: identity.Email, Name: identity.Name, PictureURL: identity.PictureURL}
	accessToken, expiresAt, err := s.tokens.Issue(user.Subject)
	if errors.Is(err, token.ErrUnavailable) {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_CONFIGURATION_REQUIRED, "JWT 발급 설정이 필요합니다.", false)
	}
	if err != nil {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_INTERNAL, "인증 토큰을 발급하지 못했습니다.", true)
	}
	return &userv1.CompleteGoogleLoginResponse{User: &userv1.GoogleUser{
		Subject: user.Subject, Email: user.Email, Name: user.Name, PictureUrl: user.PictureURL,
	}, AccessToken: accessToken, ExpiresAt: timestamppb.New(expiresAt)}, nil
}

func (s *Service) GetCurrentUser(ctx context.Context, _ *userv1.GetCurrentUserRequest) (*userv1.GetCurrentUserResponse, error) {
	identity, ok := token.IdentityFromContext(ctx)
	if !ok {
		return nil, rpcerror.New(ctx, commonv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "인증이 필요합니다.", false)
	}
	return &userv1.GetCurrentUserResponse{Subject: identity.Subject}, nil
}
