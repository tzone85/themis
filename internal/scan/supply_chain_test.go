package scan

import (
	"testing"

	"github.com/tzone85/themis/internal/aichange"
)

func runSupplyChain(t *testing.T, files map[string]string) []Finding {
	t.Helper()
	c := aichange.AIChange{}
	bodies := map[string][]byte{}
	for path, body := range files {
		c.TouchedFiles = append(c.TouchedFiles, aichange.FileTouch{
			Path: path, ChangeKind: aichange.FileAdded, AfterHash: "h",
		})
		bodies[path] = []byte(body)
	}
	out, err := NewSupplyChainScanner().Scan(c, bodies)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSupplyChain_DetectsSlopsquat_NPM(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"package.json": `{"dependencies":{"reactt":"^18.0.0"}}`,
	})
	if len(findings) == 0 {
		t.Fatal("expected slopsquat finding for reactt")
	}
	saw := false
	for _, f := range findings {
		if f.Kind == "slopsquat" && f.Severity == SeverityHigh {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("no slopsquat finding: %+v", findings)
	}
}

func TestSupplyChain_DetectsHallucination_NPM(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"package.json": `{"dependencies":{"completely-made-up-zzz-pkg":"^1.0.0"}}`,
	})
	saw := false
	for _, f := range findings {
		if f.Kind == "hallucinated_import" && f.Severity == SeverityCritical {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("no hallucinated_import finding: %+v", findings)
	}
}

func TestSupplyChain_PopularPackagePasses(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"package.json": `{"dependencies":{"react":"^18.0.0","lodash":"^4.0.0"}}`,
	})
	if len(findings) != 0 {
		t.Fatalf("popular packages should not flag: %+v", findings)
	}
}

func TestSupplyChain_PyPISlopsquat(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"requirements.txt": "reqests==2.30.0\n",
	})
	saw := false
	for _, f := range findings {
		if f.Kind == "slopsquat" && f.Line == 1 {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("expected slopsquat at line 1: %+v", findings)
	}
}

func TestSupplyChain_PyPIHallucination(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"requirements.txt": "phantom-pkg-xyz==1.0\n",
	})
	saw := false
	for _, f := range findings {
		if f.Kind == "hallucinated_import" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("expected hallucination finding: %+v", findings)
	}
}

func TestSupplyChain_PyPIPopularPasses(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"requirements.txt": "requests==2.30.0\nnumpy==1.24.0\n",
	})
	if len(findings) != 0 {
		t.Fatalf("popular pypi pkgs should not flag: %+v", findings)
	}
}

func TestSupplyChain_GoModule(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"go.mod": "module example.com/x\n\ngo 1.22\n\nrequire (\n\tgithub.com/spf13/cobr v1.8.0\n)\n",
	})
	// "cobr" vs popular "github.com/spf13/cobra" — but the name comparison is
	// against the full import path. Distance is 1.
	saw := false
	for _, f := range findings {
		if f.Kind == "slopsquat" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("expected go-module slopsquat: %+v", findings)
	}
}

func TestSupplyChain_NonManifestSkipped(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"src/x.go": "package x\n",
		"README.md": "hello\n",
	})
	if len(findings) != 0 {
		t.Fatalf("non-manifest files should be skipped: %+v", findings)
	}
}

func TestSupplyChain_BrokenJSONSkipped(t *testing.T) {
	findings := runSupplyChain(t, map[string]string{
		"package.json": "{not json",
	})
	if len(findings) != 0 {
		t.Fatalf("broken manifest should skip silently: %+v", findings)
	}
}

func TestSupplyChain_DeletedFileSkipped(t *testing.T) {
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{{Path: "package.json", ChangeKind: aichange.FileDeleted}},
	}
	out, err := NewSupplyChainScanner().Scan(c, map[string][]byte{
		"package.json": []byte(`{"dependencies":{"reactt":"^18.0.0"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("deleted manifests must be skipped: %+v", out)
	}
}
