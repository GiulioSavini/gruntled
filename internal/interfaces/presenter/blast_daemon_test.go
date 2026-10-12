package presenter_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

// TestBlastJSONUnchanged pins BlastJSON's bytes for the presenter fixtures
// as they were before the daemon member existed.
func TestBlastJSONUnchanged(t *testing.T) {
	g, _ := blastGRT004(t)
	for name, c := range map[string]struct {
		res  impact.Result
		want string
	}{
		"nobaseline": {impact.Result{}, "a4e91394d4d84b970bb9f63218478569771e34dd75e28eb322ea37eda7624a11"},
		"mixed":      {blastMixed(t), "3a65b44abdcc9109531ef56e14483c21a2e9b9a0d87388531136edd4ba931698"},
		"chain":      {blastChain(t), "6bf60e33d00d82086e0075454a66dfd8ddbf3a9fd8332114553a478b4bd34926"},
		"typed":      {typedResult(t), "48fa3b8ad100a7ccdd6fd7d8213512601a9432a40a14f09556f509c16c64e4c7"},
		"crafted":    {blastCrafted(t), "9b2bb8d56b56c75d8afcdb4f98b08b54d0e992ebd6c0955906f842cf2e3ce2bc"},
		"grt004":     {g, "2f766d57d86cb68b7a17a8b3f3594c41fe00ebe0dde96a2a853648506681844d"},
	} {
		out := renderBlastJSON(t, c.res)
		sum := sha256.Sum256([]byte(out))
		if got := hex.EncodeToString(sum[:]); got != c.want || strings.Contains(out, "daemon_baseline") {
			t.Errorf("%s: BlastJSON sha256 %s, want %s", name, got, c.want)
		}
	}
}
