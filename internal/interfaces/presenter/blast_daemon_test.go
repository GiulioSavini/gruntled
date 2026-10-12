package presenter_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

func TestDaemonBaselineLabel(t *testing.T) {
	for _, c := range []struct {
		d    presenter.DaemonBaseline
		want string
	}{
		{presenter.DaemonBaseline{Source: "watch start", Generation: 1, CurrentGeneration: 3}, "daemon generation 1 (watch start), current generation 3"},
		{presenter.DaemonBaseline{Source: "rebase", Generation: 4, CurrentGeneration: 4}, "daemon generation 4 (rebase), current generation 4"},
	} {
		if got := presenter.DaemonBaselineLabel(c.d); got != c.want {
			t.Fatalf("DaemonBaselineLabel(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
	label := presenter.DaemonBaselineLabel(presenter.DaemonBaseline{Source: "x\x1b[2Jy", Generation: 1, CurrentGeneration: 1})
	out := renderBlastText(t, blastMixed(t), label)
	first, _, _ := strings.Cut(out, "\n")
	if strings.ContainsRune(out, 0x1b) || first != `baseline: daemon generation 1 (x\x1b[2Jy), current generation 1` {
		t.Fatalf("label line %q", first)
	}
}

// daemonMember is the exact block BlastJSONDaemon inserts after the
// "baseline": true line for {watch start, 1, 3}.
const daemonMember = `  "daemon_baseline": {
    "source": "watch start",
    "generation": 1,
    "current_generation": 3
  },
`

func TestBlastJSONDaemonMember(t *testing.T) {
	d := presenter.DaemonBaseline{Source: "watch start", Generation: 1, CurrentGeneration: 3}
	g, _ := blastGRT004(t)
	for name, res := range map[string]impact.Result{"mixed": blastMixed(t), "chain": blastChain(t), "typed": typedResult(t), "grt004": g} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			if err := presenter.BlastJSONDaemon(&b, res, d); err != nil {
				t.Fatal(err)
			}
			got := b.String()
			anchor := "  \"baseline\": true,\n"
			if strings.Count(got, daemonMember) != 1 || !strings.Contains(got, anchor+daemonMember) {
				t.Fatalf("daemon member missing or misplaced:\n%s", got)
			}
			if stripped := strings.Replace(got, daemonMember, "", 1); stripped != renderBlastJSON(t, res) {
				t.Fatalf("BlastJSONDaemon differs from BlastJSON beyond the member:\n%s", got)
			}
			var doc struct {
				Version int `json:"version"`
				Daemon  struct {
					Source            string `json:"source"`
					Generation        uint64 `json:"generation"`
					CurrentGeneration uint64 `json:"current_generation"`
				} `json:"daemon_baseline"`
			}
			if err := json.Unmarshal(b.Bytes(), &doc); err != nil || doc.Version != 2 ||
				doc.Daemon.Source != "watch start" || doc.Daemon.Generation != 1 || doc.Daemon.CurrentGeneration != 3 {
				t.Fatalf("decoded %+v (%v)", doc, err)
			}
		})
	}
}

// fnv64a is FNV-1a 64; the presenter allowlist has no hash package.
func fnv64a(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// TestBlastJSONUnchanged pins BlastJSON's bytes (FNV-1a 64) for the
// presenter fixtures as they were before the daemon member existed; the
// values were recorded at 7a0911c, together with their sha256.
func TestBlastJSONUnchanged(t *testing.T) {
	g, _ := blastGRT004(t)
	for name, c := range map[string]struct {
		res  impact.Result
		want uint64
	}{
		"nobaseline": {impact.Result{}, 0x8d0186cab214dc64},
		"mixed":      {blastMixed(t), 0xd7b0de2ae72aa6e1},
		"chain":      {blastChain(t), 0x9f952f495ba5dcb6},
		"typed":      {typedResult(t), 0xabc9bfa6c5616ddc},
		"crafted":    {blastCrafted(t), 0xe6656df9430f0589},
		"grt004":     {g, 0x6cc24018c5bc8def},
	} {
		out := renderBlastJSON(t, c.res)
		if got := fnv64a(out); got != c.want || strings.Contains(out, "daemon_baseline") {
			t.Errorf("%s: BlastJSON fnv64a %#x, want %#x", name, got, c.want)
		}
	}
}
