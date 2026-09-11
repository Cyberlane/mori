package source

import (
	"strings"
	"testing"
)

func TestGperfGeneratedHeader(t *testing.T) {
	for _, test := range []struct {
		name, content string
		generated     bool
	}{
		{"observed", "/* ANSI-C code produced by gperf version 3.3 */\nint lookup() { return 1; }", true},
		{"crlf", "/* ANSI-C code produced by gperf version 3.1.0 */\r\n", true},
		{"prose", "// This code was produced by gperf version 3.3\n", false},
		{"mention", "// ANSI-C code produced by gperf version compatibility matters\n", false},
		{"string", "const char *header = \"ANSI-C code produced by gperf version 3.3\";", false},
		{"late", strings.Repeat("\n", maxGeneratedHeaderBytes) + "/* ANSI-C code produced by gperf version 3.3 */", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, marker := detectGeneratedContent([]byte(test.content))
			if got != test.generated || got && marker != "gperf-generated" {
				t.Fatalf("got %v %q", got, marker)
			}
		})
	}
}

func TestClassificationPathExcludesExternalAncestors(t *testing.T) {
	for _, c := range []struct{ cwd, root, path, want string }{
		{"/work/project", "/work/project/src", "/work/project/tests/main.go", "tests/main.go"},
		{"/work/project", "/tmp/tests/project", "/tmp/tests/project/src/main.go", "src/main.go"},
		{"/work/project", "/tmp/tests/project", "/tmp/tests/project/tests/main.go", "tests/main.go"},
		{"/work/project", "/tmp/tests/project/src", "/tmp/tests/project/src/main.go", "main.go"},
	} {
		if got := classificationPath(c.cwd, c.root, c.path); got != c.want {
			t.Errorf("got %s want %s", got, c.want)
		}
	}
}
