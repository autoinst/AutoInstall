package core

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func metadataInstaller(t *testing.T, files map[string]string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "installer.jar")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

// Reduced from the read-only MC26.2 / Forge65.1.5 references; neither is a remote client jar.
const forge26Version = `{"inheritsFrom":"26.2","libraries":[
 {"name":"net.minecraftforge:forge:26.2-65.1.5:client","downloads":{"artifact":{"path":"net/minecraftforge/forge/26.2-65.1.5/forge-26.2-65.1.5-client.jar","url":"","sha1":"be81e6d24785e467d30f30bcf31c18733233d3fe"}}},
 {"name":"net.minecraftforge:forge:26.2-65.1.5:universal","downloads":{"artifact":{"url":"https://maven.minecraftforge.net/universal.jar","sha1":"0f335ff3ac159e956c5a4b28aa058b466f6f9027"}}},
 {"name":"test:shared-client-name:1","downloads":{"artifact":{"url":"https://example.invalid/shared.jar"}}}
]}`
const forge26Profile = `{"minecraft":"26.2","serverJarPath":"{LIBRARY_DIR}/net/minecraft/server/{MINECRAFT_VERSION}/server-{MINECRAFT_VERSION}-bundled.jar",
 "data":{
 "PATCHED":{"client":"[net.minecraftforge:forge:26.2-65.1.5:client]","server":"[net.minecraftforge:forge:26.2-65.1.5:server]"},
 "PATCHED_SHA":{"client":"'be81e6d24785e467d30f30bcf31c18733233d3fe'","server":"'5bf74d1c9e606c86c9d38fcdeab05eb5a53ca32b'"}},
 "processors":[
 {"sides":["server"],"jar":"net.minecraftforge:binarypatcher:1.3.4","classpath":["test:shared-client-name:1"],"args":["--output","{PATCHED}"],"outputs":{"{PATCHED}":"{PATCHED_SHA}"}},
 {"sides":["client"],"jar":"net.minecraftforge:binarypatcher:1.3.4","classpath":["test:client-tool:1"],"outputs":{"{PATCHED}":"{PATCHED_SHA}"}}],
 "libraries":[
 {"name":"test:client-tool:1","downloads":{"artifact":{"url":"https://example.invalid/client-tool.jar"}}},
 {"name":"net.minecraftforge:binarypatcher:1.3.4","downloads":{"artifact":{"url":"https://maven.minecraftforge.net/binarypatcher.jar"}}},
 {"name":"test:embedded:1","downloads":{"artifact":{"url":""}}}
 ]}`

func TestFullForge26ReferenceProfiles(t *testing.T) {
	files := make(map[string]string)
	for _, name := range []string{"version.json", "install_profile.json"} {
		data, err := os.ReadFile(filepath.Join("testdata", "forge26", name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	info, err := ExtractVersionJson(metadataInstaller(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Processors) != 5 || len(info.RequiredServerArtifacts) != 2 {
		t.Fatalf("incomplete full profile: processors=%d, outputs=%+v", len(info.Processors), info.RequiredServerArtifacts)
	}
	if len(info.MetadataWarnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", info.MetadataWarnings)
	}
	foundClient, foundUniversal := false, false
	for _, lib := range info.Libraries {
		if lib.Name == "net.minecraftforge:forge:26.2-65.1.5:client" {
			foundClient = true
			if lib.Kind != LibraryClientOnly || !lib.ArtifactURLPresent || lib.Downloads.Artifact.URL != "" {
				t.Fatalf("client artifact became remote: %+v", lib)
			}
		} else if lib.Kind != LibraryRemote {
			t.Fatalf("full profile shared dependency excluded: %+v", lib)
		}
		if lib.Name == "net.minecraftforge:forge:26.2-65.1.5:universal" {
			foundUniversal = true
			if len(lib.Sources) != 2 {
				t.Fatalf("universal merge provenance: %+v", lib.Sources)
			}
		}
	}
	if !foundClient || !foundUniversal {
		t.Fatal("reference libraries lost")
	}
	for _, artifact := range info.RequiredServerArtifacts {
		if artifact.SHA1 == "" || strings.Contains(artifact.Path, "-client.jar") {
			t.Fatalf("invalid server artifact: %+v", artifact)
		}
	}
}

func TestExtractForge26ServerMetadata(t *testing.T) {
	info, err := ExtractVersionJson(metadataInstaller(t, map[string]string{
		"version.json": forge26Version, "install_profile.json": forge26Profile, "maven/test/embedded/1/embedded-1.jar": "embedded",
	}))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]LibraryKind{}
	for _, lib := range info.Libraries {
		kinds[lib.Name] = lib.Kind
	}
	for name, want := range map[string]LibraryKind{
		"net.minecraftforge:forge:26.2-65.1.5:client":    LibraryClientOnly,
		"net.minecraftforge:forge:26.2-65.1.5:universal": LibraryRemote,
		"test:shared-client-name:1":                      LibraryRemote,
		"test:client-tool:1":                             LibraryClientOnly,
		"net.minecraftforge:binarypatcher:1.3.4":         LibraryRemote,
		"test:embedded:1":                                LibraryEmbedded,
	} {
		if kinds[name] != want {
			t.Errorf("%s: got %s, want %s", name, kinds[name], want)
		}
	}
	if len(info.RequiredServerArtifacts) != 1 {
		t.Fatalf("server outputs: %+v", info.RequiredServerArtifacts)
	}
	want := RequiredServerArtifact{Path: "libraries/net/minecraftforge/forge/26.2-65.1.5/forge-26.2-65.1.5-server.jar", SHA1: "5bf74d1c9e606c86c9d38fcdeab05eb5a53ca32b"}
	if info.RequiredServerArtifacts[0] != want {
		t.Fatalf("server output = %+v", info.RequiredServerArtifacts[0])
	}
	if info.Data["PATCHED"].Client == info.Data["PATCHED"].Server || len(info.Processors) != 2 {
		t.Fatal("side/processor evidence lost")
	}
	if info.MinecraftVersion != "26.2" || !strings.Contains(info.ServerJarPath, "bundled") {
		t.Fatalf("layout lost: %+v", info)
	}
	for _, lib := range info.Libraries {
		if lib.Name == "net.minecraftforge:forge:26.2-65.1.5:client" {
			if !lib.DownloadsPresent || !lib.ArtifactPresent || !lib.ArtifactURLPresent || len(lib.Sources) == 0 {
				t.Fatalf("presence/source evidence lost: %+v", lib)
			}
		}
	}
}

func TestLibraryURLPresenceAndUnknownMetadata(t *testing.T) {
	tests := []struct {
		name, raw                string
		downloads, artifact, url bool
		kind                     LibraryKind
	}{
		{"legacy", `{"name":"g:a:1"}`, false, false, false, LibraryRemote},
		{"absent artifact", `{"name":"g:a:1","downloads":{}}`, true, false, false, LibraryUnknown},
		{"null artifact", `{"name":"g:a:1","downloads":{"artifact":null}}`, true, true, false, LibraryUnknown},
		{"absent URL", `{"name":"g:a:1","downloads":{"artifact":{"path":"g/a/1/a-1.jar"}}}`, true, true, false, LibraryUnknown},
		{"empty URL", `{"name":"g:a:1","downloads":{"artifact":{"url":""}}}`, true, true, true, LibraryUnknown},
		{"null URL", `{"name":"g:a:1","downloads":{"artifact":{"url":null}}}`, true, true, true, LibraryUnknown},
		{"remote client name", `{"name":"g:client:1","downloads":{"artifact":{"url":"https://example.invalid/lib.jar"}}}`, true, true, true, LibraryRemote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lib Library
			if err := json.Unmarshal([]byte(tt.raw), &lib); err != nil {
				t.Fatal(err)
			}
			if lib.DownloadsPresent != tt.downloads || lib.ArtifactPresent != tt.artifact || lib.ArtifactURLPresent != tt.url {
				t.Fatalf("presence: %+v", lib)
			}
			info := VersionInfo{Libraries: []Library{lib}}
			if err := AnalyzeServerMetadata(&info); err != nil {
				t.Fatal(err)
			}
			if info.Libraries[0].Kind != tt.kind {
				t.Fatalf("kind %s", info.Libraries[0].Kind)
			}
			if tt.kind == LibraryUnknown && len(info.MetadataWarnings) == 0 {
				t.Fatal("unknown modern library did not warn")
			}
		})
	}
}

func TestMetadataMergesListsAndRejectsSHA1Conflicts(t *testing.T) {
	t.Run("all lists survive", func(t *testing.T) {
		info, err := ExtractVersionJson(metadataInstaller(t, map[string]string{
			"version.json":         `{"libraries":[{"name":"g:common:1","downloads":{"artifact":{"path":"g/common/1/common-1.jar","url":"","sha1":"abc"}}}]}`,
			"install_profile.json": `{"install":{"path":"g:old:1","filePath":"old.jar"},"libraries":[{"name":"g:common:1","downloads":{"artifact":{"url":"","sha1":"ABC"}}},{"name":"g:top:1"}],"versionInfo":{"libraries":[{"name":"g:nested:1"}]}}`,
			"old.jar":              "legacy installer payload",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if len(info.Libraries) != 4 {
			t.Fatalf("libraries = %+v", info.Libraries)
		}
		for _, lib := range info.Libraries {
			if lib.Name == "g:common:1" && (!lib.ArtifactURLPresent || len(lib.Sources) != 2) {
				t.Fatalf("merge lost evidence: %+v", lib)
			}
			if lib.Name == "g:old:1" && lib.Kind != LibraryEmbedded {
				t.Fatalf("legacy embedded: %+v", lib)
			}
		}
		if info.InstallFilePath != "old.jar" {
			t.Fatal(info.InstallFilePath)
		}
	})
	for _, profile := range []string{
		`{"libraries":[{"name":"g:a:1","downloads":{"artifact":{"path":"g/a/1/a-1.jar","sha1":"def"}}}]}`,
		`{"libraries":[{"name":"g:a:1","checksums":["def"]}]}`,
	} {
		_, err := ExtractVersionJson(metadataInstaller(t, map[string]string{"version.json": `{"libraries":[{"name":"g:a:1","downloads":{"artifact":{"path":"g/a/1/a-1.jar","sha1":"abc"}}}]}`, "install_profile.json": profile}))
		if err == nil || !strings.Contains(err.Error(), "SHA1") {
			t.Fatalf("conflicting SHA1 accepted: %v", err)
		}
	}
}

func TestConservativeProcessorOutputs(t *testing.T) {
	tests := []struct {
		name, profile string
		outputs       int
		warning       bool
	}{
		{"cycle", `{"data":{"A":{"server":"{B}"},"B":{"server":"{A}"}},"processors":[{"sides":["server"],"outputs":{"{A}":"x"}}]}`, 0, true},
		{"unknown expression", `{"processors":[{"sides":["server"],"outputs":{"{UNKNOWN}/x.jar":"x"}}]}`, 0, true},
		{"unknown output hash", `{"processors":[{"sides":["server"],"outputs":{"{ROOT}/libraries/g/a/1/a-1.jar":{"unknown":true}}}]}`, 1, true},
		{"unknown sides", `{"processors":[{"sides":["future-side"],"outputs":{"[g:a:1]":"x"}}]}`, 0, true},
		{"traversal", `{"processors":[{"outputs":{"{ROOT}/../escape.jar":"x"}}]}`, 0, true},
		{"both sides", `{"processors":[{"outputs":{"[g:a:1]":"'da39a3ee5e6b4b0d3255bfef95601890afd80709'"}}]}`, 1, false},
		{"client only", `{"processors":[{"sides":["client"],"outputs":{"[g:a:1]":"'da39a3ee5e6b4b0d3255bfef95601890afd80709'"}}]}`, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ExtractVersionJson(metadataInstaller(t, map[string]string{"install_profile.json": tt.profile}))
			if err != nil {
				t.Fatal(err)
			}
			if len(info.RequiredServerArtifacts) != tt.outputs {
				t.Fatalf("outputs %+v", info.RequiredServerArtifacts)
			}
			if (len(info.MetadataWarnings) > 0) != tt.warning {
				t.Fatalf("warnings %+v", info.MetadataWarnings)
			}
		})
	}
}

func TestClientProcessorCannotExcludeRuntimeOrSharedRemoteLibrary(t *testing.T) {
	info, err := ExtractVersionJson(metadataInstaller(t, map[string]string{
		"version.json":         `{"libraries":[{"name":"g:runtime:1","downloads":{"artifact":{"url":"https://example.invalid/runtime.jar"}}}]}`,
		"install_profile.json": `{"libraries":[{"name":"g:runtime:1"},{"name":"g:shared:1"},{"name":"g:unknown-side:1"}],"versionInfo":{"libraries":[{"name":"g:nested-runtime:1"}]},"processors":[{"sides":["client"],"jar":"g:runtime:1","classpath":["g:shared:1","g:unknown-side:1","g:nested-runtime:1"]},{"sides":["server"],"jar":"g:shared:1"},{"sides":["future"],"jar":"g:unknown-side:1"}]}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, lib := range info.Libraries {
		if lib.Kind != LibraryRemote {
			t.Fatalf("shared library incorrectly excluded: %+v", lib)
		}
	}
}

func TestMavenArtifactPaths(t *testing.T) {
	for name, want := range map[string]string{
		"a.b:thing:1.2":              "a/b/thing/1.2/thing-1.2.jar",
		"a.b:thing:1.2:server":       "a/b/thing/1.2/thing-1.2-server.jar",
		"a.b:thing:1.2:mappings@txt": "a/b/thing/1.2/thing-1.2-mappings.txt",
	} {
		got, err := MavenArtifactPath(name)
		if err != nil || got != want {
			t.Errorf("%s = %s, %v", name, got, err)
		}
	}
	for _, name := range []string{"a:b", "a:b:../x", "a:b:1:../../x", "a:b:1@../jar", ":b:1", "a:b:1:client:extra"} {
		if _, err := MavenArtifactPath(name); err == nil {
			t.Errorf("invalid coordinate accepted: %s", name)
		}
	}
	for _, name := range []string{"../escape.jar", "/absolute.jar", "C:/escape.jar", `..\escape.jar`} {
		lib := Library{}
		lib.Downloads.Artifact.Path = name
		if _, err := LibraryArtifactPath(lib); err == nil {
			t.Errorf("invalid path accepted: %s", name)
		}
	}
	lib := Library{Name: "a.b:thing:1.2", FilePath: "thing-custom.jar"}
	got, err := LibraryArtifactPath(lib)
	if err != nil || got != "a/b/thing/1.2/thing-custom.jar" {
		t.Fatalf("legacy path %s, %v", got, err)
	}
}

func TestVerifyRequiredServerArtifacts(t *testing.T) {
	root := t.TempDir()
	info := VersionInfo{RequiredServerArtifacts: []RequiredServerArtifact{{Path: "libraries/generated.jar", SHA1: "a9993e364706816aba3e25717850c26c9cd0d89d"}}}
	if err := VerifyRequiredServerArtifacts(root, info); err == nil {
		t.Fatal("missing accepted")
	}
	target := filepath.Join(root, "libraries", "generated.jar")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRequiredServerArtifacts(root, info); err == nil {
		t.Fatal("empty accepted")
	}
	if err := os.WriteFile(target, []byte("bad"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRequiredServerArtifacts(root, info); err == nil {
		t.Fatal("wrong hash accepted")
	}
	if err := os.WriteFile(target, []byte("abc"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRequiredServerArtifacts(root, info); err != nil {
		t.Fatal(err)
	}
	info.RequiredServerArtifacts[0].Path = "../escape.jar"
	if err := VerifyRequiredServerArtifacts(root, info); err == nil {
		t.Fatal("unsafe path accepted")
	}
}

func TestMergedDeclarationsPreserveEmptyAndMissingURL(t *testing.T) {
	info, err := ExtractVersionJson(metadataInstaller(t, map[string]string{
		"version.json":         `{"libraries":[{"name":"g:a:1","downloads":{"artifact":{"url":""}}}]}`,
		"install_profile.json": `{"libraries":[{"name":"g:a:1","downloads":{"artifact":{}}}]}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Libraries) != 1 || len(info.Libraries[0].Declarations) != 2 {
		t.Fatalf("declarations lost: %+v", info.Libraries)
	}
	first, second := info.Libraries[0].Declarations[0], info.Libraries[0].Declarations[1]
	if first.Source != "version.json" || !first.ArtifactURLPresent || first.ArtifactURL != "" {
		t.Fatalf("explicit empty lost: %+v", first)
	}
	if second.Source != "install_profile.json" || second.ArtifactURLPresent {
		t.Fatalf("absent URL lost: %+v", second)
	}
}

func TestServerCoordinateArgumentsRetainSharedLibraries(t *testing.T) {
	info, err := ExtractVersionJson(metadataInstaller(t, map[string]string{
		"install_profile.json": `{"data":{"TOOL":{"server":"[g:shared:1]"}},"libraries":[{"name":"g:shared:1","downloads":{"artifact":{"url":"https://example.invalid/shared.jar"}}}],"processors":[{"sides":["client"],"jar":"g:shared:1"},{"sides":["server"],"args":["--input","{TOOL}"]}]}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if info.Libraries[0].Kind != LibraryRemote {
		t.Fatalf("server input was excluded: %+v", info.Libraries[0])
	}
}

func TestGeneratedOutputSHA1Conflicts(t *testing.T) {
	for _, raw := range []string{
		`{"libraries":[{"name":"g:a:1","downloads":{"artifact":{"sha1":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","url":""}}}],"processors":[{"sides":["server"],"outputs":{"[g:a:1]":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}]}`,
		`{"processors":[{"sides":["server"],"outputs":{"[g:a:1]":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},{"sides":["server"],"outputs":{"[g:a:1]":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}]}`,
	} {
		_, err := ExtractVersionJson(metadataInstaller(t, map[string]string{"install_profile.json": raw}))
		if err == nil || !strings.Contains(err.Error(), "SHA1") {
			t.Fatalf("conflict accepted: %v", err)
		}
	}
}
