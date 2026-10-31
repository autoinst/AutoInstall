package pkg

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/autoinst/AutoInstall/core"
)

func TestForgeFamilyInstallOrderAndMetadata(t *testing.T) {
	for _, loader := range []string{"forge", "neoforge"} {
		for _, legacy := range []bool{false, true} {
			name := loader + "/modern"
			if legacy {
				name = loader + "/legacy"
			}
			t.Run(name, func(t *testing.T) {
				info := core.VersionInfo{
					MinecraftVersion:        "1.21.1",
					InstallFilePath:         "legacy-server.jar",
					RequiredServerArtifacts: []core.RequiredServerArtifact{{Path: "libraries/generated-server.jar", SHA1: "a9993e364706816aba3e25717850c26c9cd0d89d"}},
				}
				if legacy {
					info.MinecraftVersion = ""
				}
				var order []string
				stage := func(name string) func(core.VersionInfo) error {
					return func(got core.VersionInfo) error {
						order = append(order, name)
						if !reflect.DeepEqual(got, info) {
							t.Fatalf("%s lost metadata: %+v", name, got)
						}
						return nil
					}
				}
				steps := forgeServerInstallSteps{
					metadata: func() (core.VersionInfo, error) { order = append(order, "metadata"); return info, nil },
					prefetch: stage("prefetch"), installer: stage("installer"), script: stage("script"),
				}
				if err := runForgeServerInstall(core.InstConfig{Version: "1.21.1", Loader: loader}, steps); err != nil {
					t.Fatal(err)
				}
				if want := []string{"metadata", "prefetch", "installer", "script"}; !reflect.DeepEqual(order, want) {
					t.Fatalf("order=%v, want %v", order, want)
				}
			})
		}
	}
}

func TestForgeFamilyInstallStopsOnError(t *testing.T) {
	for _, failing := range []string{"metadata", "mismatch", "prefetch", "installer", "script"} {
		t.Run(failing, func(t *testing.T) {
			sentinel := errors.New("stage failure")
			var order []string
			stage := func(name string) func(core.VersionInfo) error {
				return func(core.VersionInfo) error {
					order = append(order, name)
					if name == failing {
						return sentinel
					}
					return nil
				}
			}
			steps := forgeServerInstallSteps{
				metadata: func() (core.VersionInfo, error) {
					order = append(order, "metadata")
					if failing == "metadata" {
						return core.VersionInfo{}, sentinel
					}
					version := "1.21.1"
					if failing == "mismatch" {
						version = "1.20.1"
					}
					return core.VersionInfo{MinecraftVersion: version}, nil
				},
				prefetch: stage("prefetch"), installer: stage("installer"), script: stage("script"),
			}
			err := runForgeServerInstall(core.InstConfig{Version: "1.21.1", Loader: "forge"}, steps)
			if err == nil {
				t.Fatal("failure ignored")
			}
			if failing != "mismatch" && !errors.Is(err, sentinel) {
				t.Fatalf("error cause lost: %v", err)
			}
			want := []string{"metadata"}
			if failing != "metadata" && failing != "mismatch" {
				want = append(want, "prefetch")
				if failing != "prefetch" {
					want = append(want, "installer")
				}
				if failing == "script" {
					want = append(want, "script")
				}
			}
			if !reflect.DeepEqual(order, want) {
				t.Fatalf("order=%v, want %v", order, want)
			}
			if failing == "mismatch" && !strings.Contains(err.Error(), "不匹配") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
