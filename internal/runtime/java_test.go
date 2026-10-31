package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJavaSelection(t *testing.T) {
	for version, want := range map[string]int{"1.12.2": 8, "1.16.5": 8, "1.17.1": 16, "1.18.2": 17, "1.20.4": 17, "1.20.5": 21, "1.21.11": 21, "26.2": 25, "25w10a": 21, "26w10a": 25, "snapshot": 25} {
		if got := JavaMajor(version); got != want {
			t.Errorf("%s: got %d want %d", version, got, want)
		}
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.Mkdir(bin, 0755)
	path := filepath.Join(bin, "java")
	if err := os.WriteFile(path, []byte("not executed"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAVA_HOME", home)
	if got := InstallerJava(false, false); got != path {
		t.Fatalf("JAVA_HOME ignored: %s", got)
	}
	if got := InstallerJava(true, true); got != "java" {
		t.Fatalf("mise Java: %s", got)
	}
	if got := InstallerJava(true, false); got != "/usr/bin/jdk/jdk1.8.0_361/bin/java" {
		t.Fatalf("simpfun Java: %s", got)
	}
}
