package pkg

import (
	"os"
	"testing"
)

func TestCurseForgeRuntimeKeyOverridesLegacyBuild(t *testing.T) {
	old := cfapiKey
	cfapiKey = "legacy-test-only-key"
	t.Cleanup(func() { cfapiKey = old })
	t.Setenv("CF_API_KEY", "placeholder")
	if err := os.Unsetenv("CF_API_KEY"); err != nil {
		t.Fatal(err)
	}
	if got := curseForgeAPIKey(); got != cfapiKey {
		t.Fatal("legacy private build compatibility lost")
	}
	t.Setenv("CF_API_KEY", " runtime-test-only-key ")
	if got := curseForgeAPIKey(); got != "runtime-test-only-key" {
		t.Fatal("runtime key did not override legacy value")
	}
	t.Setenv("CF_API_KEY", "")
	if got := curseForgeAPIKey(); got != "" {
		t.Fatal("explicit empty runtime key did not disable legacy value")
	}
}
