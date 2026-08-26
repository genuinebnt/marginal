package pagesrest_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	documentv1 "marginal/document-service/genproto/documentv1"

	"marginal/api-gateway/internal/pagesrest"
)

// fakePageService is a small hand-written PageService implementation —
// this package tests REST↔gRPC translation, not document-service's own
// business logic (already covered by that service's own integration
// tests), so a real backend isn't needed here.
type fakePageService struct {
	documentv1.UnimplementedPageServiceServer

	gotActorID string // last actor-id metadata value seen, for the forwarding test

	createFn    func(*documentv1.CreatePageRequest) (*documentv1.Page, error)
	getFn       func(*documentv1.GetPageRequest) (*documentv1.Page, error)
	listFn      func(*documentv1.ListPagesRequest) (*documentv1.ListPagesResponse, error)
	deleteFn    func(*documentv1.DeletePageRequest) (*emptypb.Empty, error)
	backlinksFn func(*documentv1.ListBacklinksRequest) (*documentv1.ListBacklinksResponse, error)
}

func (f *fakePageService) CreatePage(ctx context.Context, req *documentv1.CreatePageRequest) (*documentv1.Page, error) {
	f.captureActorID(ctx)
	return f.createFn(req)
}
func (f *fakePageService) GetPage(ctx context.Context, req *documentv1.GetPageRequest) (*documentv1.Page, error) {
	f.captureActorID(ctx)
	return f.getFn(req)
}
func (f *fakePageService) ListPages(ctx context.Context, req *documentv1.ListPagesRequest) (*documentv1.ListPagesResponse, error) {
	return f.listFn(req)
}
func (f *fakePageService) RenamePage(_ context.Context, req *documentv1.RenamePageRequest) (*documentv1.Page, error) {
	return &documentv1.Page{Id: req.Id, Title: req.Title, LifecycleState: documentv1.LifecycleState_LIFECYCLE_STATE_ACTIVE}, nil
}
func (f *fakePageService) ReparentPage(_ context.Context, req *documentv1.ReparentPageRequest) (*documentv1.Page, error) {
	return &documentv1.Page{Id: req.Id, ParentId: req.ParentId, LifecycleState: documentv1.LifecycleState_LIFECYCLE_STATE_ACTIVE}, nil
}
func (f *fakePageService) DeletePage(_ context.Context, req *documentv1.DeletePageRequest) (*emptypb.Empty, error) {
	return f.deleteFn(req)
}
func (f *fakePageService) ListBacklinks(_ context.Context, req *documentv1.ListBacklinksRequest) (*documentv1.ListBacklinksResponse, error) {
	return f.backlinksFn(req)
}

func (f *fakePageService) captureActorID(ctx context.Context) {
	md, ok := metadataFrom(ctx)
	if ok {
		if vals := md.Get("actor-id"); len(vals) > 0 {
			f.gotActorID = vals[0]
		}
	}
}

func newTestHandler(t *testing.T, fake *fakePageService) *pagesrest.Handler {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	documentv1.RegisterPageServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return pagesrest.NewHandler(documentv1.NewPageServiceClient(conn))
}

func newTestServer(t *testing.T, fake *fakePageService) *httptest.Server {
	t.Helper()
	h := newTestHandler(t, fake)
	r := chi.NewRouter()
	h.Mount(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestCreatePageTranslatesRequestAndResponse(t *testing.T) {
	fake := &fakePageService{
		createFn: func(req *documentv1.CreatePageRequest) (*documentv1.Page, error) {
			assert.Equal(t, "Architecture", req.Title)
			return &documentv1.Page{
				Id:             "018f2b1c-0000-7000-8000-000000000000",
				CreatedBy:      "018f2b1c-0000-7000-8000-000000000001",
				Title:          req.Title,
				Path:           "p018f2b1c00007000",
				SortKey:        "a1",
				LifecycleState: documentv1.LifecycleState_LIFECYCLE_STATE_ACTIVE,
				CreatedAt:      timestamppb.New(time.Date(2026, 8, 7, 0, 38, 16, 0, time.UTC)),
				UpdatedAt:      timestamppb.New(time.Date(2026, 8, 7, 0, 38, 16, 0, time.UTC)),
			}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Post(srv.URL+"/pages", "application/json", jsonBody(t, map[string]any{"title": "Architecture"}))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "Architecture", body["title"])
	assert.Equal(t, "active", body["lifecycle_state"])
	assert.Nil(t, body["parent_id"])
	_, hasDeletedAt := body["deleted_at"]
	assert.False(t, hasDeletedAt, "deleted_at must be omitted while active, not sent as null")
}

func TestGetPageForwardsActorIDFromHeader(t *testing.T) {
	fake := &fakePageService{
		getFn: func(req *documentv1.GetPageRequest) (*documentv1.Page, error) {
			return &documentv1.Page{Id: req.Id, LifecycleState: documentv1.LifecycleState_LIFECYCLE_STATE_ACTIVE}, nil
		},
	}
	srv := newTestServer(t, fake)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/pages/some-id", nil)
	require.NoError(t, err)
	req.Header.Set("X-Actor-Id", "018f2b1c-0000-7000-8000-000000000009")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "018f2b1c-0000-7000-8000-000000000009", fake.gotActorID)
}

func TestGetPageNotFoundMapsTo404(t *testing.T) {
	fake := &fakePageService{
		getFn: func(*documentv1.GetPageRequest) (*documentv1.Page, error) {
			return nil, status.Error(codes.NotFound, "page not found")
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/pages/missing")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "not_found", body["error"])
	assert.Equal(t, "page not found", body["message"])
}

func TestListPagesBuildsQueryParamsAndResponse(t *testing.T) {
	fake := &fakePageService{
		listFn: func(req *documentv1.ListPagesRequest) (*documentv1.ListPagesResponse, error) {
			require.NotNil(t, req.ParentId)
			assert.Equal(t, "some-parent", *req.ParentId)
			require.NotNil(t, req.Limit)
			assert.Equal(t, int32(10), *req.Limit)
			return &documentv1.ListPagesResponse{
				Pages: []*documentv1.Page{{Id: "a", LifecycleState: documentv1.LifecycleState_LIFECYCLE_STATE_ACTIVE}},
			}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/pages?parent_id=some-parent&limit=10")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Pages []map[string]any `json:"pages"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Pages, 1)
	assert.Equal(t, "a", body.Pages[0]["id"])
}

func TestDeletePageReturnsNoContent(t *testing.T) {
	fake := &fakePageService{
		deleteFn: func(*documentv1.DeletePageRequest) (*emptypb.Empty, error) { return &emptypb.Empty{}, nil },
	}
	srv := newTestServer(t, fake)

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/pages/some-id", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestCreatePageRejectsInvalidJSON(t *testing.T) {
	srv := newTestServer(t, &fakePageService{})

	resp, err := http.Post(srv.URL+"/pages", "application/json", jsonBodyRaw(t, "{not json"))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestBacklinksTranslatesRequestAndResponse(t *testing.T) {
	fake := &fakePageService{
		backlinksFn: func(req *documentv1.ListBacklinksRequest) (*documentv1.ListBacklinksResponse, error) {
			assert.Equal(t, "target-id", req.PageId)
			return &documentv1.ListBacklinksResponse{
				Backlinks: []*documentv1.Backlink{
					{FromPage: "source-id", FromPageTitle: "Source Page", FromPageDeleted: false, TargetTitle: "Target Page"},
				},
			}, nil
		},
	}
	srv := newTestServer(t, fake)

	resp, err := http.Get(srv.URL + "/pages/target-id/backlinks")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Backlinks []map[string]any `json:"backlinks"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Backlinks, 1)
	assert.Equal(t, "source-id", body.Backlinks[0]["from_page"])
	assert.Equal(t, "Source Page", body.Backlinks[0]["from_page_title"])
	assert.Equal(t, "Target Page", body.Backlinks[0]["target_title"])
	assert.Equal(t, false, body.Backlinks[0]["from_page_deleted"])
}
