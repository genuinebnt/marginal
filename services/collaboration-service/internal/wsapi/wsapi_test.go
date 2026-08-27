package wsapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
	"marginal/collaboration-service/internal/session"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeRepo is a minimal in-memory opstore.Repo — this package tests the
// WebSocket transport, not persistence (already covered by opstore's own
// integration tests and session's own unit tests against an equivalent
// fake), so a real Postgres isn't needed here.
type fakeRepo struct {
	mu     sync.Mutex
	byPage map[uuid.UUID][]oplog.LoggedOp
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byPage: make(map[uuid.UUID][]oplog.LoggedOp)} }

func (f *fakeRepo) Append(_ context.Context, l oplog.LoggedOp) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byPage[l.PageID] = append(f.byPage[l.PageID], l)
	return true, nil
}

func (f *fakeRepo) AppendBatch(_ context.Context, ls []oplog.LoggedOp) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range ls {
		f.byPage[l.PageID] = append(f.byPage[l.PageID], l)
	}
	return len(ls), nil
}

func (f *fakeRepo) ListForPage(_ context.Context, pageID uuid.UUID) ([]oplog.LoggedOp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]oplog.LoggedOp(nil), f.byPage[pageID]...), nil
}

func newTestServer(t *testing.T) (*httptest.Server, *session.Manager) {
	t.Helper()
	// nil accept options: httptest.Server dials itself, so the request's
	// Host and Origin genuinely match — no cross-origin case to configure
	// around here (that's cmd/main.go's job; see NewHandler's doc comment,
	// and TestCrossOriginConnection* below for the case this doesn't cover).
	return newTestServerWithAcceptOptions(t, nil)
}

func newTestServerWithAcceptOptions(t *testing.T, opts *websocket.AcceptOptions) (*httptest.Server, *session.Manager) {
	t.Helper()
	mgr := session.NewManager(newFakeRepo(), t.TempDir(), "server-actor")
	t.Cleanup(func() { _ = mgr.CloseAll() })

	mux := http.NewServeMux()
	mux.HandleFunc("/collab/pages/{id}", NewHandler(mgr, opts).ServeHTTP)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, mgr
}

// dialWithOrigin sets a specific Origin header — httptest.Server's own
// Dial calls never send one that disagrees with the request Host, so this
// is the only way to actually exercise coder/websocket's cross-origin
// rejection in a test (every other test in this file dials same-origin,
// which is exactly why the real bug this pins went unnoticed by tests —
// only trying it from an actual browser surfaced it; see
// docs/porting/PROGRESS.md).
func dialWithOrigin(t *testing.T, srv *httptest.Server, pageID uuid.UUID, actorID uuid.UUID, origin string) (*websocket.Conn, error) {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + "/collab/pages/" + pageID.String()
	header := http.Header{}
	header.Set("X-Actor-Id", actorID.String())
	header.Set("Origin", origin)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	return conn, err
}

func dial(t *testing.T, srv *httptest.Server, pageID uuid.UUID, actorID uuid.UUID) (*websocket.Conn, func()) {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + "/collab/pages/" + pageID.String()
	header := http.Header{}
	header.Set("X-Actor-Id", actorID.String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	require.NoError(t, err)
	return conn, func() { _ = conn.Close(websocket.StatusNormalClosure, "") }
}

// dialWithQueryParamActor mimics a real browser client, which cannot set
// a custom header on a WebSocket upgrade request at all — see
// actorFromRequest's own doc comment.
func dialWithQueryParamActor(t *testing.T, srv *httptest.Server, pageID uuid.UUID, actorID uuid.UUID) (*websocket.Conn, func()) {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + "/collab/pages/" + pageID.String() + "?actor_id=" + actorID.String()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)
	return conn, func() { _ = conn.Close(websocket.StatusNormalClosure, "") }
}

func insertTextOp(blockID documentcore.BlockID, text string) []byte {
	data, err := pageop.Marshal(pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: text}})
	if err != nil {
		panic(err)
	}
	return data
}

func deleteTextOp(blockID documentcore.BlockID, r anchor.AnchorRange) []byte {
	data, err := pageop.Marshal(pageop.Text{BlockID: blockID, Op: ops.DeleteText{Range: r}})
	if err != nil {
		panic(err)
	}
	return data
}

func insertBlockOp(blockID documentcore.BlockID, text string) []byte {
	data, err := pageop.Marshal(pageop.Block{
		Op: documentcore.InsertBlock{ID: blockID, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: text}},
	})
	if err != nil {
		panic(err)
	}
	return data
}

// insertBlock submits an InsertBlock op over the wire and waits for its
// ack — every test below needs a block to exist before it can send a Text
// op into it, since a page's live text is now scoped per block.
func insertBlock(t *testing.T, ctx context.Context, conn *websocket.Conn) documentcore.BlockID {
	t.Helper()
	id := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, wsjson.Write(ctx, conn, clientMessage{Type: "op", Op: insertBlockOp(id, "")}))
	var ack serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &ack))
	require.Equal(t, "ack", ack.Type)
	return id
}

func TestConnectReceivesInitialSnapshot(t *testing.T) {
	srv, _ := newTestServer(t)
	conn, closeConn := dial(t, srv, uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	defer closeConn()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var msg serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &msg))
	assert.Equal(t, "snapshot", msg.Type)
	require.NotNil(t, msg.Snapshot)
	assert.Empty(t, msg.Snapshot.Blocks)
}

func TestSubmittingAnOpReturnsAnAck(t *testing.T) {
	srv, _ := newTestServer(t)
	pageID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())
	conn, closeConn := dial(t, srv, pageID, actorID)
	defer closeConn()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var snapshot serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &snapshot))

	blockID := insertBlock(t, ctx, conn)

	require.NoError(t, wsjson.Write(ctx, conn, clientMessage{Type: "op", Op: insertTextOp(blockID, "hello")}))

	var ack serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &ack))
	require.Equal(t, "ack", ack.Type)
	require.NotNil(t, ack.Op)
	assert.Equal(t, actorID, ack.Op.ActorID)
	assert.Equal(t, oplog.ActorUser, ack.Op.ActorKind)
	assert.Equal(t, pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "hello"}}, ack.Op.Op)

	require.NotNil(t, ack.Boundaries, "a non-empty block must report boundary anchors — a client with no anchor of its own needs this to build its next op")
	assert.Equal(t, anchor.Before, ack.Boundaries.Start.Bias)
	assert.Equal(t, anchor.After, ack.Boundaries.End.Bias)
}

func TestSecondClientReceivesBroadcastNotAck(t *testing.T) {
	srv, _ := newTestServer(t)
	pageID := uuid.Must(uuid.NewV7())

	connA, closeA := dial(t, srv, pageID, uuid.Must(uuid.NewV7()))
	defer closeA()
	connB, closeB := dial(t, srv, pageID, uuid.Must(uuid.NewV7()))
	defer closeB()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var snap serverMessage
	require.NoError(t, wsjson.Read(ctx, connA, &snap))
	require.NoError(t, wsjson.Read(ctx, connB, &snap))

	// connA was already here when connB joined — it gets a "presence"
	// frame for that join; connB doesn't (A was already in its own
	// snapshot's Present list, per session.Session.Subscribe's contract).
	var presence serverMessage
	require.NoError(t, wsjson.Read(ctx, connA, &presence))
	require.Equal(t, "presence", presence.Type)

	blockID := insertBlock(t, ctx, connA)
	// connB also observes the InsertBlock's broadcast — drain it before the
	// text op this test actually cares about.
	var blockBroadcast serverMessage
	require.NoError(t, wsjson.Read(ctx, connB, &blockBroadcast))

	require.NoError(t, wsjson.Write(ctx, connA, clientMessage{Type: "op", Op: insertTextOp(blockID, "hi")}))

	var ack serverMessage
	require.NoError(t, wsjson.Read(ctx, connA, &ack))
	assert.Equal(t, "ack", ack.Type)

	var broadcast serverMessage
	require.NoError(t, wsjson.Read(ctx, connB, &broadcast))
	assert.Equal(t, "broadcast", broadcast.Type)
	require.NotNil(t, broadcast.Op)
	assert.Equal(t, ack.Op.ID, broadcast.Op.ID, "B must receive the exact same committed op A got acked for")
	require.NotNil(t, broadcast.Boundaries, "B needs the same boundary anchors A got, to build its own next op")
	assert.Equal(t, ack.Boundaries, broadcast.Boundaries)
}

func TestReconnectingToANonEmptyPageGetsBoundariesInTheSnapshot(t *testing.T) {
	srv, _ := newTestServer(t)
	pageID := uuid.Must(uuid.NewV7())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	first, closeFirst := dial(t, srv, pageID, uuid.Must(uuid.NewV7()))
	var firstSnap serverMessage
	require.NoError(t, wsjson.Read(ctx, first, &firstSnap))

	blockID := insertBlock(t, ctx, first)

	require.NoError(t, wsjson.Write(ctx, first, clientMessage{Type: "op", Op: insertTextOp(blockID, "hello")}))
	var ack serverMessage
	require.NoError(t, wsjson.Read(ctx, first, &ack))
	closeFirst()

	// A second, later connection to the SAME page — the same in-memory
	// Session (Manager keeps it warm), but a fresh WebSocket that never
	// submitted anything itself and so has no boundaries from any ack of
	// its own.
	second, closeSecond := dial(t, srv, pageID, uuid.Must(uuid.NewV7()))
	defer closeSecond()
	var secondSnap serverMessage
	require.NoError(t, wsjson.Read(ctx, second, &secondSnap))

	require.NotNil(t, secondSnap.Snapshot)
	require.Len(t, secondSnap.Snapshot.Blocks, 1)
	assert.Equal(t, "hello", secondSnap.Snapshot.Blocks[0].Text)
	require.NotNil(t, secondSnap.Snapshot.Blocks[0].Boundaries, "reconnecting to a non-empty page must get usable boundaries immediately, not only after submitting an op")
	assert.Equal(t, ack.Boundaries, secondSnap.Snapshot.Blocks[0].Boundaries)
}

func TestBoundariesBecomeNilAfterDeletingWholeDocument(t *testing.T) {
	srv, _ := newTestServer(t)
	conn, closeConn := dial(t, srv, uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	defer closeConn()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var snap serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &snap))

	blockID := insertBlock(t, ctx, conn)

	require.NoError(t, wsjson.Write(ctx, conn, clientMessage{Type: "op", Op: insertTextOp(blockID, "hi")}))
	var insertAck serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &insertAck))
	require.NotNil(t, insertAck.Boundaries)

	require.NoError(t, wsjson.Write(ctx, conn, clientMessage{Type: "op", Op: deleteTextOp(blockID, *insertAck.Boundaries)}))
	var deleteAck serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &deleteAck))
	assert.Equal(t, "ack", deleteAck.Type)
	assert.Nil(t, deleteAck.Boundaries, "an emptied block must report nil boundaries")
}

// TestCrossOriginConnectionRejectedByDefault is the real bug this whole
// mechanism exists for: a browser at one origin (a different port from
// this service, in local dev — genuinely a different origin by the
// WebSocket spec) connecting with coder/websocket's own default
// (nil AcceptOptions) gets rejected before the upgrade completes. This
// only reproduces with a real cross-origin Origin header — every other
// test in this file dials same-origin and would never have caught it.
func TestCrossOriginConnectionRejectedByDefault(t *testing.T) {
	srv, _ := newTestServerWithAcceptOptions(t, nil)
	_, err := dialWithOrigin(t, srv, uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "http://localhost:5173")
	assert.Error(t, err, "coder/websocket's default must reject a genuinely cross-origin upgrade")
}

// TestCrossOriginConnectionAllowedWithInsecureSkipVerify is
// cmd/main.go's actual fix, exercised the same way the bug above was
// found: InsecureSkipVerify: true (the default wsAcceptOptions picks when
// COLLAB_ALLOWED_ORIGINS is unset) lets the identical cross-origin dial
// succeed.
func TestCrossOriginConnectionAllowedWithInsecureSkipVerify(t *testing.T) {
	srv, _ := newTestServerWithAcceptOptions(t, &websocket.AcceptOptions{InsecureSkipVerify: true})
	conn, err := dialWithOrigin(t, srv, uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "http://localhost:5173")
	require.NoError(t, err)
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var snap serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &snap))
	assert.Equal(t, "snapshot", snap.Type)
}

func TestActorIdViaQueryParamWorksLikeTheHeader(t *testing.T) {
	srv, _ := newTestServer(t)
	pageID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())
	conn, closeConn := dialWithQueryParamActor(t, srv, pageID, actorID)
	defer closeConn()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var snap serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &snap))

	blockID := insertBlock(t, ctx, conn)

	require.NoError(t, wsjson.Write(ctx, conn, clientMessage{Type: "op", Op: insertTextOp(blockID, "hi")}))
	var ack serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &ack))
	require.Equal(t, "ack", ack.Type)
	assert.Equal(t, actorID, ack.Op.ActorID)
}

func TestInvalidConnectionAttemptsAreRejected(t *testing.T) {
	tests := []struct {
		name       string
		pageID     string
		withHeader bool
		wantStatus int
	}{
		{name: "missing actor header", pageID: uuid.Must(uuid.NewV7()).String(), withHeader: false, wantStatus: http.StatusUnauthorized},
		{name: "invalid page id", pageID: "not-a-uuid", withHeader: true, wantStatus: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			url := "ws" + srv.URL[len("http"):] + "/collab/pages/" + tc.pageID

			var opts *websocket.DialOptions
			if tc.withHeader {
				header := http.Header{}
				header.Set("X-Actor-Id", uuid.Must(uuid.NewV7()).String())
				opts = &websocket.DialOptions{HTTPHeader: header}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, resp, err := websocket.Dial(ctx, url, opts)
			require.Error(t, err)
			if resp != nil {
				assert.Equal(t, tc.wantStatus, resp.StatusCode)
			}
		})
	}
}

// TestSlowConsumerIsDisconnectedNotAllowedToStallOthers is the property
// connSubscriber.enqueue exists for: a broadcast recipient that never
// reads must not be able to block the sender, which may be holding the
// whole Session's mutex on behalf of every other client. A live client
// that keeps submitting ops after the slow one is dropped is proof the
// session itself was never stalled.
func TestSlowConsumerIsDisconnectedNotAllowedToStallOthers(t *testing.T) {
	srv, _ := newTestServer(t)
	pageID := uuid.Must(uuid.NewV7())

	slowConn, closeSlowConn := dial(t, srv, pageID, uuid.Must(uuid.NewV7()))
	defer closeSlowConn()
	liveConn, closeLiveConn := dial(t, srv, pageID, uuid.Must(uuid.NewV7()))
	defer closeLiveConn()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var snap serverMessage
	require.NoError(t, wsjson.Read(ctx, slowConn, &snap))
	require.NoError(t, wsjson.Read(ctx, liveConn, &snap))

	// slowConn was already here when liveConn joined — drain its "presence" frame.
	var presence serverMessage
	require.NoError(t, wsjson.Read(ctx, slowConn, &presence))
	require.Equal(t, "presence", presence.Type)

	blockID := insertBlock(t, ctx, liveConn)
	// slowConn observes the InsertBlock broadcast too — drain it before it
	// stops reading altogether below.
	var blockBroadcast serverMessage
	require.NoError(t, wsjson.Read(ctx, slowConn, &blockBroadcast))

	// slowConn never reads again from here on — its outbox buffer
	// (outboxBufferSize) fills from liveConn's broadcasts and must
	// eventually force a disconnect rather than block liveConn's own
	// submissions.
	for i := 0; i < outboxBufferSize+10; i++ {
		writeCtx, writeCancel := context.WithTimeout(context.Background(), time.Second)
		err := wsjson.Write(writeCtx, liveConn, clientMessage{Type: "op", Op: insertTextOp(blockID, "x")})
		writeCancel()
		require.NoError(t, err, "liveConn's own write must never be the thing that blocks")

		readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
		var ack serverMessage
		err = wsjson.Read(readCtx, liveConn, &ack)
		readCancel()
		require.NoError(t, err, "liveConn must keep getting acked even while slowConn is stalling")
		require.Equal(t, "ack", ack.Type)
	}
}

// TestPresenceJoinAndLeaveNotifyOtherConnections is live presence
// end-to-end over the wire: a joining actor's presence is announced to
// everyone already on the page (but not to the actor's own connection),
// and their leave is announced once their last connection closes.
func TestPresenceJoinAndLeaveNotifyOtherConnections(t *testing.T) {
	srv, _ := newTestServer(t)
	pageID := uuid.Must(uuid.NewV7())
	actorA := uuid.Must(uuid.NewV7())
	actorB := uuid.Must(uuid.NewV7())

	connA, closeA := dial(t, srv, pageID, actorA)
	defer closeA()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var snapA serverMessage
	require.NoError(t, wsjson.Read(ctx, connA, &snapA))
	assert.Empty(t, snapA.Present, "the first connection has nobody already present")

	connB, closeB := dial(t, srv, pageID, actorB)
	var snapB serverMessage
	require.NoError(t, wsjson.Read(ctx, connB, &snapB))
	require.Len(t, snapB.Present, 1, "B's own snapshot must already list A, who was here first")
	assert.Equal(t, actorA.String(), snapB.Present[0])

	var joinMsg serverMessage
	require.NoError(t, wsjson.Read(ctx, connA, &joinMsg))
	require.Equal(t, "presence", joinMsg.Type)
	assert.Equal(t, actorB.String(), joinMsg.ActorID)
	require.NotNil(t, joinMsg.Joined)
	assert.True(t, *joinMsg.Joined)

	closeB()
	var leaveMsg serverMessage
	require.NoError(t, wsjson.Read(ctx, connA, &leaveMsg))
	require.Equal(t, "presence", leaveMsg.Type)
	assert.Equal(t, actorB.String(), leaveMsg.ActorID)
	require.NotNil(t, leaveMsg.Joined)
	assert.False(t, *leaveMsg.Joined)
}

// TestPresenceIgnoresASecondConnectionFromTheSameActor is the
// multi-tab-is-not-two-people property: a second connection from an
// actor already present must not fire another "joined" event, and
// closing just one of the two connections must not fire "left" while the
// other is still open.
func TestPresenceIgnoresASecondConnectionFromTheSameActor(t *testing.T) {
	srv, _ := newTestServer(t)
	pageID := uuid.Must(uuid.NewV7())
	actorA := uuid.Must(uuid.NewV7())
	observer := uuid.Must(uuid.NewV7())

	observerConn, closeObserver := dial(t, srv, pageID, observer)
	defer closeObserver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var observerSnap serverMessage
	require.NoError(t, wsjson.Read(ctx, observerConn, &observerSnap))

	// One persistent reader for observerConn from here on — coder/websocket
	// doesn't support concurrent reads on one connection, so every
	// "expect a frame"/"expect nothing" check below shares this single
	// goroutine's channel rather than each issuing its own Read.
	observerFrames := startReader(observerConn)

	tab1, closeTab1 := dial(t, srv, pageID, actorA)
	var snap1 serverMessage
	require.NoError(t, wsjson.Read(ctx, tab1, &snap1))
	join1 := requireFrame(t, observerFrames, 5*time.Second)
	require.Equal(t, "presence", join1.Type)
	assert.True(t, *join1.Joined)

	tab2, closeTab2 := dial(t, srv, pageID, actorA)
	var snap2 serverMessage
	require.NoError(t, wsjson.Read(ctx, tab2, &snap2))
	// No second "joined" for tab2 — actorA was already present via tab1.
	requireNoFrame(t, observerFrames, 200*time.Millisecond)

	closeTab1()
	// No "left" either — actorA is still present via tab2.
	requireNoFrame(t, observerFrames, 200*time.Millisecond)

	closeTab2()
	leave := requireFrame(t, observerFrames, 5*time.Second)
	require.Equal(t, "presence", leave.Type)
	assert.Equal(t, actorA.String(), leave.ActorID)
	assert.False(t, *leave.Joined, "only the LAST connection closing should announce a leave")
}

// startReader reads conn in a loop, publishing each decoded frame to the
// returned channel until conn closes (then the channel closes too). One
// reader per connection — coder/websocket doesn't support concurrent
// reads on the same connection.
func startReader(conn *websocket.Conn) <-chan serverMessage {
	ch := make(chan serverMessage, 16)
	go func() {
		defer close(ch)
		for {
			var msg serverMessage
			if err := wsjson.Read(context.Background(), conn, &msg); err != nil {
				return
			}
			ch <- msg
		}
	}()
	return ch
}

func requireFrame(t *testing.T, ch <-chan serverMessage, d time.Duration) serverMessage {
	t.Helper()
	select {
	case msg, ok := <-ch:
		require.True(t, ok, "connection closed before a frame arrived")
		return msg
	case <-time.After(d):
		t.Fatalf("no frame arrived within %s", d)
		return serverMessage{}
	}
}

func requireNoFrame(t *testing.T, ch <-chan serverMessage, d time.Duration) {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if ok {
			t.Fatalf("expected no frame within %s, got %+v", d, msg)
		}
	case <-time.After(d):
	}
}

func TestUnknownMessageTypeGetsAnErrorFrameNotADisconnect(t *testing.T) {
	srv, _ := newTestServer(t)
	conn, closeConn := dial(t, srv, uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	defer closeConn()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var snap serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &snap))

	require.NoError(t, wsjson.Write(ctx, conn, clientMessage{Type: "bogus"}))

	var errMsg serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &errMsg))
	assert.Equal(t, "error", errMsg.Type)
	assert.NotEmpty(t, errMsg.Message)

	// connection must still be alive — a follow-up real op still works
	blockID := insertBlock(t, ctx, conn)
	require.NoError(t, wsjson.Write(ctx, conn, clientMessage{Type: "op", Op: insertTextOp(blockID, "x")}))
	var ack serverMessage
	require.NoError(t, wsjson.Read(ctx, conn, &ack))
	assert.Equal(t, "ack", ack.Type)
}
