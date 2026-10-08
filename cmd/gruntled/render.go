package main

import (
	"bytes"
	"fmt"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

// renderReport renders rep in format exactly as check prints it: out goes
// to stdout; summary, set only for "text", goes to stderr. check and the
// watch daemon both call it, so a report served by the daemon is
// byte-identical to check's output by construction.
func renderReport(format string, rep checking.Report) (out, summary []byte, err error) {
	var buf bytes.Buffer
	switch format {
	case "json":
		err = presenter.JSON(&buf, rep.Graph, rep.Diagnostics)
	case "sarif":
		err = presenter.SARIF(&buf, rep.Graph, rep.Diagnostics, presenter.ToolInfo{Version: version})
	case "text":
		err = presenter.Text(&buf, rep.Diagnostics)
		if err == nil {
			var sum bytes.Buffer
			if err = presenter.Summary(&sum, rep.Graph, rep.Diagnostics); err == nil {
				summary = sum.Bytes()
			}
		}
	default:
		return nil, nil, fmt.Errorf("unknown format %q", format)
	}
	if err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), summary, nil
}

// buildSnapshot pre-renders rep in every format. The snapshot holds bytes
// only, never rep, so connection handlers share nothing mutable with the
// indexer.
func buildSnapshot(rep checking.Report, gen uint64) (*ipc.Snapshot, error) {
	s := &ipc.Snapshot{State: ipc.StateReady, HasErrors: rep.Diagnostics.HasErrors(), Generation: gen}
	var err error
	if s.Text, s.Summary, err = renderReport("text", rep); err != nil {
		return nil, err
	}
	if s.JSON, _, err = renderReport("json", rep); err != nil {
		return nil, err
	}
	if s.SARIF, _, err = renderReport("sarif", rep); err != nil {
		return nil, err
	}
	return s, nil
}
