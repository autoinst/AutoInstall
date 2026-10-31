package pkg

import (
	"fmt"
	"testing"

	"github.com/autoinst/AutoInstall/core"
)

func TestFabricLatestPreservesFixedFields(t *testing.T) {
	for _, tc := range []struct {
		mc, loader, wantMC, wantLoader string
		mcCalls, loaderCalls           int
	}{
		{"latest", "0.19.5", "26.3", "0.19.5", 1, 0},
		{"1.21.1", "latest", "1.21.1", "0.20.0", 0, 1},
		{"latest", "latest", "26.3", "0.20.0", 1, 1},
	} {
		mcCalls, loaderCalls := 0, 0
		got, err := resolveFabricTarget(core.InstConfig{Version: tc.mc, LoaderVersion: tc.loader}, func(string, int) (string, error) { mcCalls++; return "26.3", nil }, func(string, int) (string, error) { loaderCalls++; return "0.20.0", nil })
		if err != nil || got.Version != tc.wantMC || got.LoaderVersion != tc.wantLoader || mcCalls != tc.mcCalls || loaderCalls != tc.loaderCalls {
			t.Fatalf("resolved %+v with calls %d/%d: %v", got, mcCalls, loaderCalls, err)
		}
	}
}

func TestForgeLatestPreservesFixedFields(t *testing.T) {
	latest := func(int) (string, string, error) { return "26.2", "65.1.5", nil }
	builds := func(mc string, retries int) ([]string, error) {
		if mc != "26.2" && mc != "1.20.1" {
			return nil, fmt.Errorf("unexpected Minecraft %s", mc)
		}
		if mc == "1.20.1" {
			return []string{"47.2.9", "47.2.10", "47.1.1"}, nil
		}
		return []string{"65.1.3", "65.1.5"}, nil
	}
	tests := []struct {
		mc, loader, wantMC, wantLoader string
		fail                           bool
	}{
		{"latest", "latest", "26.2", "65.1.5", false},
		{"latest", "65.1.3", "26.2", "65.1.3", false},
		{"latest", "65.1.3-26.2", "26.2", "65.1.3-26.2", false},
		{"latest", "47.2.10", "26.2", "47.2.10", true},
		{"1.20.1", "latest", "1.20.1", "47.2.10", false},
		{"1.20.1", "47.1.1", "1.20.1", "47.1.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.mc+"/"+tt.loader, func(t *testing.T) {
			got, err := resolveForgeTarget(core.InstConfig{Version: tt.mc, LoaderVersion: tt.loader}, latest, builds)
			if (err != nil) != tt.fail {
				t.Fatalf("error = %v", err)
			}
			if got.Version != tt.wantMC || got.LoaderVersion != tt.wantLoader {
				t.Fatalf("fixed version overwritten: %+v", got)
			}
		})
	}
}

func TestNeoForgeLatestPreservesFixedFields(t *testing.T) {
	latest := func(int) (string, error) { return "21.1.200", nil }
	matching := func(mc string, retries int) (string, error) {
		if mc != "1.20.4" {
			return "", fmt.Errorf("unexpected Minecraft %s", mc)
		}
		return "20.4.237", nil
	}
	tests := []struct {
		mc, loader, wantMC, wantLoader string
		fail                           bool
	}{
		{"latest", "latest", "1.21.1", "21.1.200", false},
		{"latest", "21.1.123", "1.21.1", "21.1.123", false},
		{"latest", "20.4.237", "1.21.1", "20.4.237", true},
		{"1.20.4", "latest", "1.20.4", "20.4.237", false},
		{"1.20.4", "20.4.10", "1.20.4", "20.4.10", false},
		{"1.20.4", "21.1.123", "1.20.4", "21.1.123", true},
	}
	for _, tt := range tests {
		t.Run(tt.mc+"/"+tt.loader, func(t *testing.T) {
			got, err := resolveNeoForgeTarget(core.InstConfig{Version: tt.mc, LoaderVersion: tt.loader}, latest, matching)
			if (err != nil) != tt.fail {
				t.Fatalf("error = %v", err)
			}
			if got.Version != tt.wantMC || got.LoaderVersion != tt.wantLoader {
				t.Fatalf("fixed version overwritten: %+v", got)
			}
		})
	}
	for loader, want := range map[string]string{"21.0.10-beta": "1.21", "20.4.10": "1.20.4", "26.2.0-beta": "26.2"} {
		got, err := neoForgeMinecraftVersion(loader)
		if err != nil || got != want {
			t.Errorf("%s = %s, %v", loader, got, err)
		}
	}
}

func TestForgeInstallerLegacyNaming(t *testing.T) {
	urls := forgeInstallerURLs("1.7.10", "10.13.4.1614")
	if len(urls) != 2 || urls[1] != "https://maven.minecraftforge.net/net/minecraftforge/forge/1.7.10-10.13.4.1614-1.7.10/forge-1.7.10-10.13.4.1614-1.7.10-installer.jar" {
		t.Fatalf("legacy URLs %+v", urls)
	}
	if urls := forgeInstallerURLs("1.7.10", "10.13.4.1614-1.7.10"); len(urls) != 1 {
		t.Fatalf("duplicate legacy suffix: %+v", urls)
	}
}
