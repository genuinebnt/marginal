package authrest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	"marginal/api-gateway/internal/authmw"
	authv1 "marginal/auth-service/genproto/authv1"

	"marginal/api-gateway/internal/authrest"
)

// fakeAuthService is a small hand-written AuthService implementation —
// this package tests REST↔gRPC translation, not auth-service's own
// business logic (already covered by that service's own integration
// tests).
type fakeAuthService struct {
	authv1.UnimplementedAuthServiceServer

	gotActorID string

	registerFn func(*authv1.RegisterRequest) (*authv1.TokenPair, error)
	loginFn    func(*authv1.AuthenticateRequest) (*authv1.TokenPair, error)
	getUserFn  func(*authv1.GetUserRequest) (*authv1.User, error)
	refreshFn  func(*authv1.RefreshRequest) (*authv1.TokenPair, error)
	revokeFn   func(*authv1.RevokeRequest) (*emptypb.Empty, error)
}

func (f *fakeAuthService) Register(_ context.Context, req *authv1.RegisterRequest) (*authv1.TokenPair, error) {
	return f.registerFn(req)
}
func (f *fakeAuthService) Authenticate(_ context.Context, req *authv1.AuthenticateRequest) (*authv1.TokenPair, error) {
	return f.loginFn(req)
}
func (f *fakeAuthService) GetUser(_ context.Context, req *authv1.GetUserRequest) (*authv1.User, error) {
	return f.getUserFn(req)
}
func (f *fakeAuthService) Refresh(_ context.Context, req *authv1.RefreshRequest) (*authv1.TokenPair, error) {
	return f.refreshFn(req)
}
func (f *fakeAuthService) Revoke(_ context.Context, req *authv1.RevokeRequest) (*emptypb.Empty, error) {
	return f.revokeFn(req)
}
func (f *fakeAuthService) RevokeAll(ctx context.Context, _ *authv1.RevokeAllRequest) (*emptypb.Empty, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("actor-id"); len(vals) > 0 {
			f.gotActorID = vals[0]
		}
	}
	return &emptypb.Empty{}, nil
}

func newTestServer(t *testing.T, fake *fakeAuthService) *httptest.Server {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	authv1.RegisterAuthServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	h := authrest.NewHandler(authv1.NewAuthServiceClient(conn))
	r := chi.NewRouter()
	// The REAL middleware, with a verifier that treats the token as the
	// subject. The handler's contract is "forward the VERIFIED actor", and
	// a test that injected an id straight into the context would skip the
	// half that makes it verified.
	r.Use(authmw.Middleware(tokenIsTheSubject{}))
	h.Mount(r)
	httpSrv := httptest.NewServer(r)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

// tokenIsTheSubject lets a test say "call as this actor" in one argument
// while still going through the real verification path. Refuses anything
// that is not a uuid, so an unauthenticated request is still unauthenticated.
type tokenIsTheSubject struct{}

func (tokenIsTheSubject) Subject(_ context.Context, token string) (uuid.UUID, error) {
	id, err := uuid.Parse(token)
	if err != nil {
		return uuid.Nil, errors.New("authrest_test: not a subject")
	}
	return id, nil
}

func jsonBody(t *testing.T, v any) io.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewReader(data)
}

func TestRegisterReturnsTokenPair(t *testing.T) {
	fake := &fakeAuthService{
		registerFn: func(req *authv1.RegisterRequest) (*authv1.TokenPair, error) {
			assert.Equal(t, "a@b.com", req.Email)
			return &authv1.TokenPair{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 900}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Post(srv.URL+"/auth/register", "application/json", jsonBody(t, map[string]any{
		"email": "a@b.com", "password": "hunter22222", "display_name": "A",
	}))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "access", body["access_token"])
	assert.Equal(t, "refresh", body["refresh_token"])
	assert.EqualValues(t, 900, body["expires_in"])
}

func TestLoginUnauthenticatedMapsTo401WithSameShapeForBothCauses(t *testing.T) {
	fake := &fakeAuthService{
		loginFn: func(*authv1.AuthenticateRequest) (*authv1.TokenPair, error) {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Post(srv.URL+"/auth/login", "application/json", jsonBody(t, map[string]any{
		"email": "nobody@b.com", "password": "wrong",
	}))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "unauthenticated", body["error"])
	assert.Equal(t, "invalid credentials", body["message"])
}

func TestGetUserOmitsPasswordHash(t *testing.T) {
	fake := &fakeAuthService{
		getUserFn: func(req *authv1.GetUserRequest) (*authv1.User, error) {
			return &authv1.User{Id: req.Id, Email: "a@b.com", DisplayName: "A", CursorColor: "#fff"}, nil
		},
	}
	srv := newTestServer(t, fake)

	// GetUser is NOT on authmw's public allowlist — only register, login
	// and refresh are, and refresh only because the whole reason to call it
	// is that your access token has expired.
	getReq, err := http.NewRequest(http.MethodGet, srv.URL+"/auth/users/some-id", nil)
	require.NoError(t, err)
	getReq.Header.Set("Authorization", "Bearer 018f2b1c-0000-7000-8000-00000000000a")
	resp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "a@b.com", body["email"])
	_, hasPassword := body["password_hash"]
	assert.False(t, hasPassword)
	_, hasPassword2 := body["password"]
	assert.False(t, hasPassword2)
}

func TestRefreshReturnsNewTokenPair(t *testing.T) {
	fake := &fakeAuthService{
		refreshFn: func(req *authv1.RefreshRequest) (*authv1.TokenPair, error) {
			assert.Equal(t, "old-refresh", req.RefreshToken)
			return &authv1.TokenPair{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 900}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Post(srv.URL+"/auth/refresh", "application/json", jsonBody(t, map[string]any{"refresh_token": "old-refresh"}))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "new-access", body["access_token"])
}

func TestRevokeAllForwardsActorIDAndReturnsNoContent(t *testing.T) {
	fake := &fakeAuthService{}
	srv := newTestServer(t, fake)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/revoke-all", nil)
	require.NoError(t, err)
	// A bearer token now, not a claimed id (ADR-013 §1). The gateway does
	// not read X-Actor-Id at all — sending it would be sending nothing.
	req.Header.Set("Authorization", "Bearer 018f2b1c-0000-7000-8000-000000000009")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "018f2b1c-0000-7000-8000-000000000009", fake.gotActorID)
}
