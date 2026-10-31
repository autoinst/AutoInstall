package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autoinst/AutoInstall/internal/model"
)

func TestResolveConfiguration(t *testing.T) {
	pack := model.Target{Version: "26.3", Loader: "fabric", LoaderVersion: "0.19.5"}
	cases := []struct {
		name, json         string
		args               []string
		pack               *model.Target
		want               model.Target
		follow, classifier bool
		wantErr            string
	}{
		{"cli without file", "", []string{"--mc-version", "1.21.1"}, nil, model.Target{Version: "1.21.1", Loader: "vanilla"}, true, true, ""},
		{"legacy file", `{"version":"1.21.1","loader":"neoforge","loaderVersion":"21.1.123","download":"none"}`, nil, nil, model.Target{Version: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1.123"}, true, true, ""},
		{"pack ignores old target types", `{"version":123,"loader":{},"loaderVersion":[],"download":false,"maxconnections":8}`, nil, &pack, pack, true, true, ""},
		{"file false", `{"followModpackFilenames":false,"enableModClassifier":false}`, nil, &pack, pack, false, false, ""},
		{"cli false", `{"followModpackFilenames":true,"enableModClassifier":true}`, []string{"--follow-modpack-filenames=false", "--mod-classifier=false"}, &pack, pack, false, false, ""},
		{"cli true wins", `{"followModpackFilenames":false}`, []string{"--follow-modpack-filenames=true"}, &pack, pack, true, true, ""},
		{"pack override", "", []string{"--mc-version", "1.21.1"}, &pack, model.Target{Version: "1.21.1", Loader: "fabric", LoaderVersion: "0.19.5"}, true, true, ""},
		{"loader cannot inherit other version", "", []string{"--loader", "forge"}, &pack, model.Target{}, false, false, "切换加载器"},
		{"switch to vanilla", "", []string{"--loader", "vanilla"}, &pack, model.Target{Version: "26.3", Loader: "vanilla"}, true, true, ""},
		{"no target", "", nil, nil, model.Target{}, false, false, "未找到安装目标"},
		{"invalid retries", `{"maxretries":0}`, nil, &pack, model.Target{}, false, false, "正整数"},
		{"broken json", `{`, nil, &pack, model.Target{}, false, false, "解析配置"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "inst.json")
			if tc.json != "" {
				if err := os.WriteFile(path, []byte(tc.json), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cli, err := Parse(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			req, _, err := Load(path, tc.pack, cli)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if req.Target != tc.want || req.FollowModpackFilenames != tc.follow || req.EnableModClassifier != tc.classifier {
				t.Fatalf("unexpected request: %+v", req)
			}
			if tc.json != "" {
				data, _ := os.ReadFile(path)
				if string(data) != tc.json {
					t.Fatal("configuration changed")
				}
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("configuration created")
			}
		})
	}
}

func TestExplicitEmptyArgsAndPrecedence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "inst.json")
	os.WriteFile(p, []byte(`{"version":"1.21.1","argsment":"-Xmx2G","maxconnections":8,"maxretries":2}`), 0600)
	cli, err := Parse([]string{"--argsment=", "--maxconnections=3", "--maxretries=4"})
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := Load(p, nil, cli)
	if err != nil {
		t.Fatal(err)
	}
	if r.Argsment != "" || r.MaxConnections != 3 || r.MaxRetries != 4 {
		t.Fatalf("explicit options lost: %+v", r)
	}
}

func TestTargetRejectsPathsAndControlCharacters(t *testing.T) {
	for _, value := range []string{".", "..", "../1.21.1", `..\1.21.1`, "C:version", "1.21\x00", "1.21\n.1", "1.21\t.1", "1.21?"} {
		for _, field := range []string{"mc-version", "loader-version"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				args := []string{"--mc-version=1.21.1", "--loader=fabric", "--loader-version=0.16.0", "--" + field + "=" + value}
				cli, err := Parse(args)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := Load(filepath.Join(t.TempDir(), "inst.json"), nil, cli); err == nil {
					t.Fatalf("accepted %s=%q", field, value)
				}
			})
		}
	}
}

func TestInputSelectionAndValidation(t *testing.T) {
	for _, args := range [][]string{{"--modpack="}, {"--mc-version="}, {"--loader-version="}, {"--modpack=x", "--no-modpack"}, {"extra"}, {"--unknown"}} {
		if _, err := Parse(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, tc := range []struct {
		args []string
		auto bool
	}{{nil, true}, {[]string{"--mc-version=1.21.1"}, false}, {[]string{"--no-modpack"}, false}, {[]string{"--maxconnections=4"}, true}} {
		cli, err := Parse(tc.args)
		if err != nil {
			t.Fatal(err)
		}
		if cli.DiscoverAutomatically() != tc.auto {
			t.Fatalf("wrong mode for %v", tc.args)
		}
	}
}
