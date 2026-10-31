package loader

import "testing"

func TestMavenRelPathFromNameCompatibility(t *testing.T) {
	for name, want := range map[string]string{
		"net.fabricmc:fabric-loader:0.14.21": "net/fabricmc/fabric-loader/0.14.21/fabric-loader-0.14.21.jar",
		"org.example:dependency:1.0-beta":    "org/example/dependency/1.0-beta/dependency-1.0-beta.jar",
		"already/relative.jar":               "already/relative.jar",
		"short:name":                         "short:name",
		"":                                   "",
	} {
		if got := MavenRelPathFromName(name); got != want {
			t.Errorf("%q=%q, want %q", name, got, want)
		}
	}
	if DefaultMavenBase != "https://repo1.maven.org/maven2/" {
		t.Fatal(DefaultMavenBase)
	}
}
