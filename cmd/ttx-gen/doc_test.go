package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	teletext "github.com/Eyevinn/go-teletext"
)

// docs/test-stream.md lists the page and the text of every language preset in
// the stream that scripts/all-langs-stream.sh makes. The table there must be
// what ttx-gen sends for all presets in the order of -list.
func TestAllLangsTable(t *testing.T) {
	var codes []string
	for _, l := range teletext.Languages {
		codes = append(codes, l.Code)
	}
	o, err := parseOptions(flag.NewFlagSet(appName, flag.ContinueOnError),
		[]string{appName, "-langs", strings.Join(codes, ",")})
	if err != nil {
		t.Fatal(err)
	}
	ps, _, err := pageStates(o)
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	want.WriteString("| Page | Code | First line | Second line starts with | What it needs from the decoder |\n")
	want.WriteString("|---|---|---|---|---|\n")
	for _, p := range ps {
		fmt.Fprintf(&want, "| %d%02X | %s | %s | %s | %s |\n",
			p.addr.Magazine, p.addr.Page, p.lang.Code, p.lang.Sample, p.lang.Name, p.lang.Note)
	}
	doc, err := os.ReadFile("../../docs/test-stream.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), want.String()) {
		t.Errorf("docs/test-stream.md does not have this table of the language presets:\n%s", want.String())
	}
}
