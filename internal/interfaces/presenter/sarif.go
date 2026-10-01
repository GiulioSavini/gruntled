package presenter

import (
	"fmt"
	"strings"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
)

const (
	sarifSchemaURI   = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json"
	sarifVersion     = "2.1.0"
	sarifToolName    = "gruntled"
	sarifToolURI     = "https://github.com/GiulioSavini/gruntled"
	sarifDocsURI     = "https://github.com/GiulioSavini/gruntled/blob/master/docs/cli.md"
	sarifURIBaseID   = "%SRCROOT%"
	sarifColumnKind  = "unicodeCodePoints"
	sarifRulePrecise = "very-high"
)

// ToolInfo describes the gruntled build that produced a SARIF document.
type ToolInfo struct {
	// Version is written as tool.driver.version, for example "v0.2.0" or
	// "dev".
	Version string
}

// The SARIF document is built from structs only, never maps, so its key
// order is fixed by the field order here and the output is deterministic.
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	ColumnKind  string            `json:"columnKind"`
	Invocations []sarifInvocation `json:"invocations"`
	Results     []sarifResult     `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifRule struct {
	ID                   string              `json:"id"`
	Name                 string              `json:"name"`
	ShortDescription     sarifText           `json:"shortDescription"`
	FullDescription      sarifText           `json:"fullDescription"`
	HelpURI              string              `json:"helpUri"`
	Help                 sarifText           `json:"help"`
	DefaultConfiguration sarifRuleConfig     `json:"defaultConfiguration"`
	Properties           sarifRuleProperties `json:"properties"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifRuleProperties struct {
	Tags      []string `json:"tags"`
	Precision string   `json:"precision"`
}

type sarifInvocation struct {
	ExecutionSuccessful        bool                `json:"executionSuccessful"`
	ToolExecutionNotifications []sarifNotification `json:"toolExecutionNotifications"`
}

// sarifNotification reports a unit or module gruntled could not analyse.
// Locations is omitted for modules: a module is a directory, not a file.
type sarifNotification struct {
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

// sarifPhysicalLocation has no region for notifications, which point at a
// whole file.
type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

// sarifRuleDef is one diagnostic code as a SARIF rule. title and severity
// mirror the "### <code>: <title> (<severity>)" heading in docs/cli.md,
// which also gives the helpUri anchor.
type sarifRuleDef struct {
	code     diagnostic.Code
	name     string
	title    string
	severity string
	full     string
	help     string
}

// sarifRuleTable lists every rule in code order. ruleIndex in a result is
// a position in this table, so the order is part of the document format.
var sarifRuleTable = []sarifRuleDef{
	{
		code:     diagnostic.CodeUnknownOutput,
		name:     "DependencyOutputNotDeclared",
		title:    "dependency output not declared",
		severity: "error",
		full:     "A dependency.X.outputs.Y reference names an output Y that the module behind dependency X does not declare. gruntled follows config_path to the target unit and its terraform source to the module, and stays silent when either hop is uncertain.",
		help:     "Declare output Y in the module behind dependency X, or fix the reference to name an output the module declares. If mock_outputs supplies Y, apply would silently use the mock value.",
	},
	{
		code:     diagnostic.CodeMissingDependencyTarget,
		name:     "DependencyTargetHasNoUnit",
		title:    "dependency target has no unit",
		severity: "error",
		full:     "A dependency block's config_path, or an entry of a dependencies paths list, resolves to a directory that does not exist or has no terragrunt.hcl. Terragrunt refuses to run such a dependency, whatever skip_outputs or mock_outputs say.",
		help:     "Point config_path (or the dependencies paths entry) at a directory that holds a terragrunt.hcl, or remove the dependency.",
	},
	{
		code:     diagnostic.CodeDependencyCycle,
		name:     "DependencyCycle",
		title:    "dependency cycle",
		severity: "error",
		full:     "Units depend on each other in a cycle, which Terragrunt refuses to order. There is one diagnostic per cycle, attributed to its lexically smallest unit at that unit's first edge into the cycle.",
		help:     "Remove or disable one dependency in the cycle so the units can be ordered.",
	},
	{
		code:     diagnostic.CodeSyntaxError,
		name:     "HclSyntaxError",
		title:    "HCL syntax error",
		severity: "error",
		full:     "A Terragrunt or Terraform file does not parse. One diagnostic per file, reporting the first error only. The unit or module that depends on the file becomes unknown, so GRT001 stays silent for it.",
		help:     "Fix the HCL syntax at the reported position.",
	},
}

// sarifRules returns the driver.rules array built from sarifRuleTable.
func sarifRules() []sarifRule {
	rules := make([]sarifRule, 0, len(sarifRuleTable))
	for _, r := range sarifRuleTable {
		heading := string(r.code) + ": " + r.title + " (" + r.severity + ")"
		rules = append(rules, sarifRule{
			ID:                   string(r.code),
			Name:                 r.name,
			ShortDescription:     sarifText{Text: r.title},
			FullDescription:      sarifText{Text: r.full},
			HelpURI:              sarifDocsURI + "#" + githubAnchor(heading),
			Help:                 sarifText{Text: r.help},
			DefaultConfiguration: sarifRuleConfig{Level: r.severity},
			Properties: sarifRuleProperties{
				Tags:      []string{"terragrunt", "correctness"},
				Precision: sarifRulePrecise,
			},
		})
	}
	return rules
}

// ruleIndexOf returns the position of code in sarifRuleTable. A code with
// no rule is an internal bug, reported as an error rather than written as
// an undefined ruleId.
func ruleIndexOf(code diagnostic.Code) (int, error) {
	for i, r := range sarifRuleTable {
		if r.code == code {
			return i, nil
		}
	}
	return 0, fmt.Errorf("sarif: diagnostic code %q has no rule", string(code))
}

// githubAnchor derives the anchor GitHub gives a Markdown heading:
// lowercase, every character other than a letter, digit, '-', '_' or
// space dropped, spaces turned into '-'.
func githubAnchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// percentEncode turns a repo-relative path into a URI reference: every byte
// other than an RFC 3986 unreserved character (A-Z a-z 0-9 - . _ ~) or '/'
// becomes %XX with uppercase hex, so UTF-8 is encoded byte by byte.
func percentEncode(p string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '.', c == '_', c == '~', c == '/':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0F])
		}
	}
	return b.String()
}
