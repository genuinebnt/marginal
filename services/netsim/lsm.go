package netsim

// LSMLevel is one row of § 14's LOG · LSM SHAPE.
type LSMLevel struct {
	Name  string `json:"name"`
	Files int    `json:"files"`
	Ops   int    `json:"ops"`
	// Compacting is the level currently being merged into. Drawn
	// empty-with-a-caption rather than as a bar, because a level
	// mid-compaction genuinely has no settled size.
	Compacting bool `json:"compacting"`
}

// LSMView is the log's shape as a storage engine would hold it.
//
// This is a MODEL of the shape, not `collab.ops`' actual storage —
// that is one Postgres table (`DATA_MODEL.md`), and this repo does not
// run an LSM. It is drawn because the op log has an LSM's exact
// access pattern (append-only writes, ordered reads, periodic
// compaction of old segments) and seeing that shape is what makes
// the WAL-and-flush design in RFC-002 §7 legible. The screen says so.
type LSMView struct {
	Levels []LSMLevel `json:"levels"`
	// WriteAmplification is bytes written to disk over bytes accepted —
	// the price of the shape, which a drawing that only showed neat
	// bars would hide.
	WriteAmplification float64 `json:"write_amplification"`
	MemtableCap        int     `json:"memtable_cap"`
	Fanout             int     `json:"fanout"`
}

const (
	memtableCap = 64 // ops
	fanout      = 4
)

// BuildLSM derives the level shape from one number: how many ops the
// log holds.
func BuildLSM(ops int) LSMView {
	v := LSMView{MemtableCap: memtableCap, Fanout: fanout}

	inMem := ops % memtableCap
	flushed := ops / memtableCap // sealed segments, each one memtable

	// Each level holds `fanout` segments before merging into the next.
	levels := []LSMLevel{{Name: "memtable", Ops: inMem, Files: 1}}
	remaining := flushed
	written := ops // every op is written once on the way in
	for l := 0; remaining > 0 || l == 0; l++ {
		here := remaining % fanout
		promoted := remaining / fanout
		levels = append(levels, LSMLevel{
			Name:  levelName(l),
			Files: here,
			Ops:   here * memtableCap,
			// The level a compaction is currently draining into is
			// the first one that has been promoted into but is not
			// full — the honest "we are mid-merge" state.
			Compacting: promoted > 0 && here == 0,
		})
		written += promoted * fanout * memtableCap
		remaining = promoted
		if l > 6 {
			break
		}
	}
	v.Levels = levels
	if ops > 0 {
		v.WriteAmplification = float64(written) / float64(ops)
	}
	return v
}

func levelName(l int) string {
	switch l {
	case 0:
		return "L0"
	case 1:
		return "L1"
	case 2:
		return "L2"
	case 3:
		return "L3"
	default:
		return "L" + string(rune('0'+l))
	}
}
