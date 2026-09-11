package cli

import "testing"

func TestDiagnosticPathClassPortableRoots(t *testing.T) {
	for path, want := range map[string]string{`C:\foo.stories.dir\main.ts`: "", `C:\foo.test.dir\main.ts`: "", `C:\x\main.test.ts`: "test", `C:\x\main.stories.tsx`: "story", `\\server\tests\main.ts`: "", "/tmp/tests/project/main.ts": ""} {
		if got := diagnosticPathClass(path); got != want {
			t.Errorf("%s=%q want %q", path, got, want)
		}
	}
}
