package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
)

// dumpFile is the on-disk report used where sockets are unavailable.
type dumpFile struct {
	V        int       `json:"v"`
	Info     Info      `json:"info"`
	State    string    `json:"state"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
}

// WriteDump replaces the dump atomically through w (0600 file, checked
// 0700 directory). s nil means the daemon is still indexing. A dump over
// responseCap whose blast view has bytes is written with a copy of the
// view without them (TooLarge set), so plain report reads it as in v1.
// s is never modified.
func WriteDump(w *statusfile.Writer, info Info, s *Snapshot) error {
	d := dumpFile{V: ProtocolVersion, Info: info, State: StateIndexing}
	if s != nil {
		d.State = s.State
		if d.State == "" {
			d.State = StateReady
		}
		d.Snapshot = s
	}
	b, err := encodeLine(d)
	if err != nil {
		return err
	}
	if len(b) > responseCap && s != nil && s.Blast != nil && (s.Blast.Text != nil || s.Blast.JSON != nil) {
		c := *s
		v := *s.Blast
		v.Text, v.JSON, v.TooLarge = nil, nil, int64(len(b))
		c.Blast = &v
		d.Snapshot = &c
		if b, err = encodeLine(d); err != nil {
			return err
		}
	}
	return w.Write(string(b))
}

// ReadDump reads a dump written by WriteDump. The snapshot is nil when the
// daemon was still indexing.
func ReadDump(path string) (Info, *Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, int64(responseCap)+1))
	if err != nil {
		return Info{}, nil, err
	}
	if len(b) > responseCap {
		return Info{}, nil, fmt.Errorf("ipc: %s: dump over the %s cap: %w", path, capString(responseCap), errLineTooLong)
	}
	var d dumpFile
	if err := json.Unmarshal(b, &d); err != nil {
		return Info{}, nil, fmt.Errorf("ipc: malformed dump %s: %w", path, err)
	}
	if d.V != ProtocolVersion {
		return Info{}, nil, &VersionError{Daemon: d.V, Client: ProtocolVersion}
	}
	if d.State == StateIndexing {
		return d.Info, nil, nil
	}
	if d.Snapshot == nil {
		return Info{}, nil, errors.New("ipc: malformed dump: no snapshot")
	}
	return d.Info, d.Snapshot, nil
}
