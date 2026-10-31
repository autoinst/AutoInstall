package pkg

import (
	"reflect"
	"testing"

	"github.com/autoinst/AutoInstall/internal/loader"
)

func TestMavenCompatibilityWrapper(t *testing.T) {
	name := "org.example:shared-library:1.2.3"
	if got, want := mavenRelPathFromName(name), loader.MavenRelPathFromName(name); got != want {
		t.Fatalf("wrapper=%s, want %s", got, want)
	}
	if defaultMavenBase != loader.DefaultMavenBase {
		t.Fatal("repository base changed")
	}
}

func TestFabricLibraryParsingAfterMavenExtraction(t *testing.T) {
	meta := &metaResult{Libraries: []rawLibrary{
		{"name": "net.fabricmc:fabric-loader:0.14.21", "url": "https://maven.fabricmc.net/"},
		{"name": "org.example:common:1.0"},
		{"name": "org.example:native:1.0", "natives": map[string]interface{}{"linux": "natives-linux"}},
	}}
	got, err := parseLibraries(meta)
	if err != nil {
		t.Fatal(err)
	}
	want := []parsedLib{
		{Name: "net.fabricmc:fabric-loader:0.14.21", RelPath: "net/fabricmc/fabric-loader/0.14.21/fabric-loader-0.14.21.jar", URL: "https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.14.21/fabric-loader-0.14.21.jar"},
		{Name: "org.example:common:1.0", RelPath: "org/example/common/1.0/common-1.0.jar", URL: "https://repo1.maven.org/maven2/org/example/common/1.0/common-1.0.jar"},
		{Name: "org.example:native:1.0", Skip: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed libraries=%+v, want %+v", got, want)
	}
}
