package netsim

import (
	"fmt"
	"sort"
)

// Wire is the network, as § 14's inspector exposes it. Every field is a
// control on the screen; nothing here is measured, all of it is set.
type Wire struct {
	RTTMs    int `json:"rtt_ms"`
	LossPct  int `json:"loss_pct"`
	JitterMs int `json:"jitter_ms"`
	// Seed makes a bad network REPRODUCIBLE. A dropped packet you cannot
	// re-run is an anecdote; the same seed twice is a test.
	Seed int64 `json:"seed"`
}

// Scenario is one run: the wire, whether transform is on, and the edits.
type Scenario struct {
	Wire      Wire   `json:"wire"`
	Transform bool   `json:"transform"`
	Initial   string `json:"initial"`
	Edits     []Edit `json:"edits"`
}

// Edit is one authored intent, before it becomes an Op. `At` is the tick
// it is typed at — the author's own clock, not the server's.
type Edit struct {
	At    int    `json:"at"`
	Actor string `json:"actor"`
	Kind  Kind   `json:"kind"`
	Pos   int    `json:"pos"`
	Text  string `json:"text,omitempty"`
	Len   int    `json:"len,omitempty"`
}

// Delivery is one packet crossing the wire, kept so § 14 can draw it.
type Delivery struct {
	OpID      string `json:"op_id"`
	Actor     string `json:"actor"`
	SentAt    int    `json:"sent_at"`
	ArrivesAt int    `json:"arrives_at"`
	// Attempt 1 is the first send; >1 means the earlier ones were lost.
	Attempt int  `json:"attempt"`
	Lost    bool `json:"lost"`
}

// Replica is one client's whole visible state.
type Replica struct {
	Actor      string `json:"actor"`
	Text       string `json:"text"`
	Predicted  int    `json:"predicted"`
	Confirmed  int    `json:"confirmed"`
	RolledBack int    `json:"rolled_back"`
	// Pending is what is still in flight — the ops this replica has
	// applied locally that the server has not confirmed back yet.
	Pending int `json:"pending"`
}

// Report is every panel on § 14, computed from one run.
type Report struct {
	Replicas []Replica `json:"replicas"`
	// ServerText is the confirmed document — the one the log produces.
	ServerText string `json:"server_text"`
	// Converged is whether every replica equals the server. With
	// transform ON this must hold; with it OFF it usually still does,
	// which is the trap the intent ledger exists to catch.
	Converged bool `json:"converged"`

	Log         []Op       `json:"log"`
	Deliveries  []Delivery `json:"deliveries"`
	Lost        int        `json:"lost"`
	Retransmits int        `json:"retransmits"`
	Ticks       int        `json:"ticks"`

	Merkle    MerkleView `json:"merkle"`
	Causality DAGView    `json:"causality"`
	LSM       LSMView    `json:"lsm"`

	// ReplayMatches is RFC-002's own law, re-checked rather than
	// asserted: replaying the confirmed log from empty must equal the
	// incrementally-maintained server document.
	ReplayMatches bool   `json:"replay_matches"`
	ReplayText    string `json:"replay_text,omitempty"`

	// IntentViolations is the SECOND instrument. Structural agreement
	// (the Merkle digest) says the replicas hold the same bytes; this
	// says whether those bytes are what the authors meant. The check is
	// the same scenario re-run with transform ON: an op whose confirmed
	// effect differs from that run landed somewhere nobody asked for.
	IntentViolations []Violation `json:"intent_violations"`
	IntentText       string      `json:"intent_text,omitempty"`
}

// Violation is one op that landed differently from what its author meant.
type Violation struct {
	OpID  string `json:"op_id"`
	Actor string `json:"actor"`
	Meant string `json:"meant"`
	Got   string `json:"got"`
}

// client is one replica's state. `pending` holds ops applied locally
// but not yet confirmed, in the order they were applied — the stack
// rollback unwinds and then re-lays.
type client struct {
	text      string
	pending   []Op
	seen      int // server ops this client has applied
	predicted int
	confirmed int
	rolled    int
}

// rng is a splitmix64 — enough for jitter and loss, and identical on
// every platform, which math/rand does not promise across versions.
type rng struct{ state uint64 }

func (r *rng) next() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

func (r *rng) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n))
}

// Run plays the scenario out and returns everything § 14 draws.
//
// The model is a discrete tick loop, not wall-clock: one tick is one
// millisecond of simulated time, every delay is computed from the wire,
// and the seeded rng makes the whole run reproducible. A netcode
// debugger whose output changes when you look at it twice is not a
// debugger.
func Run(s Scenario) Report {
	r := &rng{state: uint64(s.Wire.Seed)*2862933555777941757 + 3037000493}

	type inFlight struct {
		op      Op
		arrives int
		attempt int
	}

	edits := append([]Edit(nil), s.Edits...)
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].At < edits[j].At })

	clients := map[string]*client{}
	actorOf := func(name string) *client {
		if clients[name] == nil {
			clients[name] = &client{text: s.Initial}
		}
		return clients[name]
	}
	// Both actors named in the scenario exist even if one never types —
	// § 14 draws two panes, and an absent second pane reads as a bug.
	for _, e := range edits {
		actorOf(e.Actor)
	}
	if len(clients) == 0 {
		actorOf("you")
	}

	serverText := s.Initial
	var log []Op
	var deliveries []Delivery
	var toServer, toClients []inFlight
	lost, retransmits := 0, 0

	half := max(s.Wire.RTTMs/2, 1)
	lastEdit := 0
	if len(edits) > 0 {
		lastEdit = edits[len(edits)-1].At
	}
	// Long enough for the last edit to make a full round trip even after
	// the worst run of retransmits the loss rate can plausibly produce.
	horizon := lastEdit + s.Wire.RTTMs*8 + s.Wire.JitterMs*8 + 200

	seq := 0
	tick := 0
	editIdx := 0

	send := func(op Op, at int) {
		delay := half + r.intn(s.Wire.JitterMs+1)
		attempt := 1
		// Loss is per attempt, and a lost op is RESENT rather than
		// dropped: an editor that loses a keystroke because a packet
		// died is not a design anyone chose. The retransmit is what
		// costs you, and § 14 counts it.
		for r.intn(100) < s.Wire.LossPct {
			lost++
			retransmits++
			attempt++
			deliveries = append(deliveries, Delivery{
				OpID: op.ID, Actor: op.Actor, SentAt: at,
				ArrivesAt: at + delay, Attempt: attempt - 1, Lost: true,
			})
			at += s.Wire.RTTMs // one timeout before trying again
			if attempt > 12 {
				break
			}
		}
		deliveries = append(deliveries, Delivery{
			OpID: op.ID, Actor: op.Actor, SentAt: at,
			ArrivesAt: at + delay, Attempt: attempt,
		})
		toServer = append(toServer, inFlight{op: op, arrives: at + delay, attempt: attempt})
	}

	for tick = 0; tick <= horizon; tick++ {
		// 1. Author types. The client applies it IMMEDIATELY — that is
		//    the prediction, and it is why the editor feels like a text
		//    box rather than a form.
		for editIdx < len(edits) && edits[editIdx].At <= tick {
			e := edits[editIdx]
			editIdx++
			c := actorOf(e.Actor)
			seq++
			op := Op{
				ID: fmt.Sprintf("o%d", seq), Actor: e.Actor, Kind: e.Kind,
				Pos: e.Pos, Text: e.Text, Len: e.Len, Base: c.seen,
				Deps: depsOf(log, c.seen),
			}
			c.text = op.Apply(c.text)
			c.pending = append(c.pending, op)
			c.predicted++
			send(op, tick)
		}

		// 2. The server accepts what has arrived, in arrival order, and
		//    transforms each against everything it accepted that the
		//    author had not seen.
		var stillFlying []inFlight
		for _, f := range toServer {
			if f.arrives > tick {
				stillFlying = append(stillFlying, f)
				continue
			}
			op := f.op
			if s.Transform {
				for _, prior := range log[op.Base:] {
					op = Transform(op, prior)
				}
			}
			op.Base = len(log)
			serverText = op.Apply(serverText)
			log = append(log, op)
			// Broadcast to everyone, author included: the author needs
			// the confirmed op to learn where it actually landed.
			for name := range clients {
				delay := half + r.intn(s.Wire.JitterMs+1)
				toClients = append(toClients, inFlight{
					op: op, arrives: tick + delay, attempt: len(name),
				})
			}
		}
		toServer = stillFlying

		// 3. Clients receive confirmations. This is the rollback: undo
		//    every pending op, apply what the server said, then re-apply
		//    the pending ops transformed against it.
		var stillArriving []inFlight
		for _, f := range toClients {
			if f.arrives > tick {
				stillArriving = append(stillArriving, f)
				continue
			}
			for _, c := range clients {
				if c.seen != f.op.Base {
					// Out of order for this client — hold it until the
					// gap fills. Ordering is the server's, not the wire's.
					continue
				}
				applyConfirmed(c, f.op, s.Transform)
			}
		}
		toClients = stillArriving

		if editIdx >= len(edits) && len(toServer) == 0 && len(toClients) == 0 {
			break
		}
	}

	// Drain what is still in the air after the horizon, so the report
	// describes a settled system rather than a snapshot mid-flight.
	// Several passes because a confirmation can only be applied once
	// the client's `seen` has caught up to its base, and the ones
	// still queued may be out of order.
	for pass := 0; pass < 4; pass++ {
		for _, f := range toClients {
			for _, c := range clients {
				if c.seen == f.op.Base {
					applyConfirmed(c, f.op, s.Transform)
				}
			}
		}
	}

	// Every slice here is initialised, never left nil.
	//
	// encoding/json renders a nil slice as `null`, and the screen
	// reads `report.log.length` — so an empty script (which the
	// screen passes through on every keystroke that clears the
	// textarea) crashed the whole page rather than drawing an empty
	// panel. Same bug class as a []byte marshalling to base64: it
	// exists only on the far side of the boundary, so it is fixed
	// at the boundary and tested there.
	rep := Report{
		ServerText: serverText, Lost: lost, Retransmits: retransmits, Ticks: tick,
		Log: log, Deliveries: deliveries,
		Replicas: []Replica{}, IntentViolations: []Violation{},
	}
	if rep.Log == nil {
		rep.Log = []Op{}
	}
	if rep.Deliveries == nil {
		rep.Deliveries = []Delivery{}
	}

	names := make([]string, 0, len(clients))
	for n := range clients {
		names = append(names, n)
	}
	sort.Strings(names)
	rep.Converged = true
	for _, n := range names {
		c := clients[n]
		rep.Replicas = append(rep.Replicas, Replica{
			Actor: n, Text: c.text, Predicted: c.predicted,
			Confirmed: c.confirmed, RolledBack: c.rolled, Pending: len(c.pending),
		})
		if c.text != serverText {
			rep.Converged = false
		}
	}

	// The law, re-checked.
	replay := s.Initial
	for _, op := range log {
		replay = op.Apply(replay)
	}
	rep.ReplayMatches = replay == serverText
	rep.ReplayText = replay

	rep.Merkle = CompareMerkle(rep.replicaText(0), rep.replicaText(1))
	rep.Causality = BuildDAG(log)
	rep.LSM = BuildLSM(len(log))
	violations, intentText := checkIntent(s, rep)
	if violations == nil {
		violations = []Violation{}
	}
	rep.IntentViolations, rep.IntentText = violations, intentText
	return rep
}

func (r Report) replicaText(i int) string {
	if i < len(r.Replicas) {
		return r.Replicas[i].Text
	}
	return r.ServerText
}

// applyConfirmed is the rollback-and-replay path, run on every
// confirmation a client receives.
func applyConfirmed(c *client, op Op, transform bool) {
	// Its own op coming home: pop it, and if the server moved it, the
	// local text has to move with it.
	if len(c.pending) > 0 && c.pending[0].ID == op.ID {
		mine := c.pending[0]
		c.pending = c.pending[1:]
		c.seen++
		c.confirmed++
		if mine.Pos != op.Pos || mine.width() != op.width() {
			c.rolled++
			rebuild(c, mine, op, transform)
		}
		return
	}
	// Somebody else's. Undo what is pending, take theirs, redo ours on
	// top — transformed, so our intent survives their edit.
	for i := len(c.pending) - 1; i >= 0; i-- {
		c.text = c.pending[i].Invert(c.text).Apply(c.text)
	}
	if len(c.pending) > 0 {
		c.rolled++
	}
	c.text = op.Apply(c.text)
	for i := range c.pending {
		if transform {
			c.pending[i] = Transform(c.pending[i], op)
		}
		c.text = c.pending[i].Apply(c.text)
	}
	c.seen++
}

// rebuild re-lays the pending stack after the server moved one of our
// own ops — the same undo-all, redo-all shape, one op deeper.
func rebuild(c *client, mine, confirmed Op, transform bool) {
	for i := len(c.pending) - 1; i >= 0; i-- {
		c.text = c.pending[i].Invert(c.text).Apply(c.text)
	}
	c.text = mine.Invert(c.text).Apply(c.text)
	c.text = confirmed.Apply(c.text)
	for i := range c.pending {
		if transform {
			c.pending[i] = Transform(c.pending[i], confirmed)
		}
		c.text = c.pending[i].Apply(c.text)
	}
}

// depsOf names the ops an author had seen — the DAG's edges. Only the
// frontier is recorded, not the whole prefix: everything earlier is
// reachable through it, and storing the transitive closure would make
// the drawn graph a solid block of edges.
func depsOf(log []Op, seen int) []string {
	if seen == 0 || seen > len(log) {
		return nil
	}
	return []string{log[seen-1].ID}
}

// checkIntent re-runs the same scenario with transform ON and reports
// which ops landed somewhere else.
//
// Using the transform-on run as the reference is the honest framing and
// the screen says it: this does not know what a person MEANT, it knows
// what the protocol would have preserved. When transform is already on,
// the two runs are the same run and the ledger is empty by construction.
func checkIntent(s Scenario, got Report) ([]Violation, string) {
	if s.Transform {
		return nil, got.ServerText
	}
	ref := s
	ref.Transform = true
	want := Run(ref)
	if want.ServerText == got.ServerText {
		return nil, want.ServerText
	}
	var out []Violation
	for i, op := range got.Log {
		if i >= len(want.Log) {
			break
		}
		w := want.Log[i]
		if op.Pos != w.Pos || op.width() != w.width() {
			out = append(out, Violation{
				OpID: op.ID, Actor: op.Actor,
				Meant: w.String(), Got: op.String(),
			})
		}
	}
	if len(out) == 0 {
		// The positions all match and the text still differs — the
		// divergence is in ORDER, not placement. Worth saying rather
		// than reporting a clean ledger over a wrong document.
		out = append(out, Violation{
			OpID: "-", Actor: "-",
			Meant: want.ServerText, Got: got.ServerText,
		})
	}
	return out, want.ServerText
}
