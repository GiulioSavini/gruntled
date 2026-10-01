package presenter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

// The test-side mirror of the SARIF document. Decoding with
// DisallowUnknownFields pins the exact key set: any extra key fails.
type tSarifLog struct {
	Schema  string      `json:"$schema"`
	Version string      `json:"version"`
	Runs    []tSarifRun `json:"runs"`
}

type tSarifRun struct {
	Tool struct {
		Driver struct {
			Name           string            `json:"name"`
			Version        string            `json:"version"`
			InformationURI string            `json:"informationUri"`
			Rules          []json.RawMessage `json:"rules"`
		} `json:"driver"`
	} `json:"tool"`
	ColumnKind  string             `json:"columnKind"`
	Invocations []tSarifInvocation `json:"invocations"`
	Results     []tSarifResult     `json:"results"`
}

type tSarifText struct {
	Text string `json:"text"`
}

type tSarifInvocation struct {
	ExecutionSuccessful        bool                 `json:"executionSuccessful"`
	ToolExecutionNotifications []tSarifNotification `json:"toolExecutionNotifications"`
}

type tSarifNotification struct {
	Level     string           `json:"level"`
	Message   tSarifText       `json:"message"`
	Locations []tSarifLocation `json:"locations"`
}

type tSarifResult struct {
	RuleID    string           `json:"ruleId"`
	RuleIndex int              `json:"ruleIndex"`
	Level     string           `json:"level"`
	Message   tSarifText       `json:"message"`
	Locations []tSarifLocation `json:"locations"`
}

type tSarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI       string `json:"uri"`
			URIBaseID string `json:"uriBaseId"`
		} `json:"artifactLocation"`
		Region *struct {
			StartLine   int `json:"startLine"`
			StartColumn int `json:"startColumn"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

func loc(uri string, line, col int) tSarifLocation {
	var l tSarifLocation
	l.PhysicalLocation.ArtifactLocation.URI = uri
	l.PhysicalLocation.ArtifactLocation.URIBaseID = "%SRCROOT%"
	if line > 0 {
		l.PhysicalLocation.Region = &struct {
			StartLine   int `json:"startLine"`
			StartColumn int `json:"startColumn"`
		}{line, col}
	}
	return l
}

func runSARIF(t *testing.T, g *repograph.RepositoryGraph, set diagnostic.Set) (string, tSarifRun) {
	t.Helper()
	var b bytes.Buffer
	if err := presenter.SARIF(&b, g, set, presenter.ToolInfo{Version: "v9.9.9"}); err != nil {
		t.Fatalf("SARIF: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b.Bytes()))
	dec.DisallowUnknownFields()
	var doc tSarifLog
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decode SARIF: %v\n%s", err, b.String())
	}
	if doc.Schema != "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json" || doc.Version != "2.1.0" {
		t.Fatalf("schema/version = %q %q", doc.Schema, doc.Version)
	}
	if len(doc.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(doc.Runs))
	}
	run := doc.Runs[0]
	if run.Tool.Driver.Name != "gruntled" || run.Tool.Driver.Version != "v9.9.9" ||
		run.Tool.Driver.InformationURI != "https://github.com/GiulioSavini/gruntled" || len(run.Tool.Driver.Rules) != 4 {
		t.Fatalf("driver = %+v", run.Tool.Driver)
	}
	if run.ColumnKind != "unicodeCodePoints" {
		t.Fatalf("columnKind = %q", run.ColumnKind)
	}
	if len(run.Invocations) != 1 || !run.Invocations[0].ExecutionSuccessful {
		t.Fatalf("invocations = %+v", run.Invocations)
	}
	for _, banned := range []string{"partialFingerprints", "relatedLocations", "originalUriBaseIds", "automationDetails", "startTimeUtc", "guid", "exitCode"} {
		if strings.Contains(b.String(), banned) {
			t.Fatalf("SARIF contains %q:\n%s", banned, b.String())
		}
	}
	return b.String(), run
}

func TestSARIFEmpty(t *testing.T) {
	out, run := runSARIF(t, emptyGraph(t), diagnostic.NewSet())
	if !strings.Contains(out, `"results": []`) || !strings.Contains(out, `"toolExecutionNotifications": []`) {
		t.Fatalf("empty lists not written as []:\n%s", out)
	}
	if len(run.Results) != 0 || len(run.Invocations[0].ToolExecutionNotifications) != 0 {
		t.Fatalf("run = %+v", run)
	}
	wantPrefix := `{
  "$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
  "version": "2.1.0",
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "gruntled",
          "version": "v9.9.9",
          "informationUri": "https://github.com/GiulioSavini/gruntled",
          "rules": [
            {
              "id": "GRT001",
              "name": "DependencyOutputNotDeclared",
              "shortDescription": {
                "text": "dependency output not declared"
              },
`
	if !strings.HasPrefix(out, wantPrefix) {
		t.Fatalf("SARIF prefix =\n%s\nwant\n%s", out[:min(len(out), len(wantPrefix))], wantPrefix)
	}
	wantSuffix := `      "columnKind": "unicodeCodePoints",
      "invocations": [
        {
          "executionSuccessful": true,
          "toolExecutionNotifications": []
        }
      ],
      "results": []
    }
  ]
}
`
	if !strings.HasSuffix(out, wantSuffix) {
		t.Fatalf("SARIF =\n%s\nwant suffix\n%s", out, wantSuffix)
	}
}

func TestSARIFMixed(t *testing.T) {
	out, run := runSARIF(t, mixedGraph(t), mixedSet(t))
	if !strings.Contains(out, `output \"a<b>&c\" is not declared`) {
		t.Fatalf("HTML characters escaped:\n%s", out)
	}
	wantResults := []tSarifResult{
		{
			RuleID: "GRT001", RuleIndex: 0, Level: "error",
			Message:   tSarifText{`output "a<b>&c" is not declared (unit live/app)`},
			Locations: []tSarifLocation{loc("live/app/terragrunt.hcl", 7, 17)},
		},
		{
			RuleID: "GRT100", RuleIndex: 3, Level: "error",
			Message:   tSarifText{"Unclosed configuration block"},
			Locations: []tSarifLocation{loc("live/broken/terragrunt.hcl", 3, 1)},
		},
	}
	if !reflect.DeepEqual(run.Results, wantResults) {
		t.Fatalf("results = %+v\nwant %+v", run.Results, wantResults)
	}
	wantNotes := []tSarifNotification{
		{Level: "note", Message: tSarifText{`unit "live/broken" is config-unknown: syntax-error`},
			Locations: []tSarifLocation{loc("live/broken/terragrunt.hcl", 0, 0)}},
		{Level: "note", Message: tSarifText{`unit "live/remote" is module-unknown: remote-source`},
			Locations: []tSarifLocation{loc("live/remote/terragrunt.hcl", 0, 0)}},
		{Level: "note", Message: tSarifText{`module "modules/empty" surface is unknown: no-terraform-files`}},
	}
	if got := run.Invocations[0].ToolExecutionNotifications; !reflect.DeepEqual(got, wantNotes) {
		t.Fatalf("notifications = %+v\nwant %+v", got, wantNotes)
	}
	if strings.Count(out, `"locations"`) != 4 {
		t.Fatalf("module notification must have no locations key:\n%s", out)
	}
}

func TestSARIFRootUnitAndEncoding(t *testing.T) {
	root, err := repograph.NewConfigUnknownUnit(rp(t, "."), reasonSyntaxError)
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: %v", err)
	}
	g, err := repograph.NewRepositoryGraph([]repograph.Unit{root}, nil)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	set := diagnostic.NewSet(fileDiag(t, "live/my app/é.hcl", 2, 5, "bad"))
	_, run := runSARIF(t, g, set)
	if got := run.Results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI; got != "live/my%20app/%C3%A9.hcl" {
		t.Fatalf("uri = %q", got)
	}
	n := run.Invocations[0].ToolExecutionNotifications
	if len(n) != 1 || n[0].Message.Text != `unit "." is config-unknown: syntax-error` ||
		!reflect.DeepEqual(n[0].Locations, []tSarifLocation{loc("terragrunt.hcl", 0, 0)}) {
		t.Fatalf("root notification = %+v", n)
	}
}

func TestSARIFWarningAndSharedInclude(t *testing.T) {
	msg := `dependency "vpc" output "nope" is not declared by module "modules/vpc"`
	w, err := diagnostic.NewForUnit(diagnostic.CodeUnknownOutput, diagnostic.SeverityWarning, rp(t, "live/c"), pos(t, "common/deps.hcl", 9, 1), "w")
	if err != nil {
		t.Fatalf("NewForUnit: %v", err)
	}
	set := diagnostic.NewSet(
		unitDiag(t, "live/b", "common/deps.hcl", 4, 9, msg),
		unitDiag(t, "live/a", "common/deps.hcl", 4, 9, msg),
		w,
	)
	_, run := runSARIF(t, emptyGraph(t), set)
	var got []string
	for _, r := range run.Results {
		got = append(got, r.Level+" "+r.Message.Text)
	}
	want := []string{
		"error " + msg + " (unit live/a)",
		"error " + msg + " (unit live/b)",
		"warning w (unit live/c)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("results = %q\nwant %q", got, want)
	}
}

func TestSARIFUnknownCodeErrors(t *testing.T) {
	d, err := diagnostic.New(diagnostic.Code("GRT999"), diagnostic.SeverityError, pos(t, "a.hcl", 1, 1), "x")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var b bytes.Buffer
	if err := presenter.SARIF(&b, emptyGraph(t), diagnostic.NewSet(d), presenter.ToolInfo{Version: "dev"}); err == nil {
		t.Fatal("SARIF accepted a code with no rule")
	}
	if b.Len() != 0 {
		t.Fatalf("SARIF wrote %d bytes on error", b.Len())
	}
}

func TestSARIFDeterministic(t *testing.T) {
	g, set := mixedGraph(t), mixedSet(t)
	var a, b bytes.Buffer
	for _, buf := range []*bytes.Buffer{&a, &b} {
		if err := presenter.SARIF(buf, g, set, presenter.ToolInfo{Version: "dev"}); err != nil {
			t.Fatalf("SARIF: %v", err)
		}
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatalf("SARIF not deterministic:\n%s\n---\n%s", a.String(), b.String())
	}
}

func TestSARIFWriteError(t *testing.T) {
	boom := errors.New("boom")
	err := presenter.SARIF(failingWriter{err: boom}, emptyGraph(t), diagnostic.NewSet(), presenter.ToolInfo{Version: "dev"})
	if !errors.Is(err, boom) {
		t.Fatalf("SARIF error = %v, want %v", err, boom)
	}
}
