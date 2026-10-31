package core

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeInstallerTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func fakeInstallerRunner(t *testing.T, f func(*exec.Cmd) error) {
	t.Helper()
	old := installerCommandRunner
	installerCommandRunner = f
	t.Cleanup(func() { installerCommandRunner = old })
}

func createTestInstaller(t *testing.T, generated bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "installer.jar")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("install_profile.json")
	if err != nil {
		t.Fatal(err)
	}
	profile := `{ "libraries": [] }`
	if generated {
		sum := sha1.Sum([]byte("generated"))
		profile = fmt.Sprintf(`{"processors":[{"sides":["server"],"outputs":{"[test:generated:1:server]":"'%s'"}}]}`, hex.EncodeToString(sum[:]))
	}
	if _, err := entry.Write([]byte(profile)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstallerUsesTargetJavaInManagedEnvironments(t *testing.T) {
	for _, tc := range []struct{ version, major, path string }{
		{"1.12.2", "8", "/usr/bin/jdk/jdk1.8.0_361/bin/java"},
		{"1.20.1", "17", "/usr/bin/jdk/jdk-17.0.6/bin/java"},
		{"1.21.1", "21", "/usr/bin/jdk/jdk-21.0.2/bin/java"},
		{"26.2", "25", "/usr/bin/jdk/jdk-25.0.2/bin/java"},
	} {
		for _, mise := range []bool{false, true} {
			t.Run(tc.version+"/mise="+strconv.FormatBool(mise), func(t *testing.T) {
				fakeInstallerRunner(t, func(cmd *exec.Cmd) error {
					if mise {
						want := "exec java@" + tc.major + " -- java"
						if len(cmd.Args) < 5 || strings.Join(cmd.Args[1:5], " ") != want {
							t.Fatalf("wrong mise target: %v", cmd.Args)
						}
					} else if cmd.Args[0] != tc.path {
						t.Fatalf("wrong JDK: %v", cmd.Args)
					}
					return nil
				})
				if err := RunInstaller("installer.jar", "neoforge", tc.version, "test", "official", true, mise); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestInstallerContextCancellationStopsRetriesAndFallback(t *testing.T) {
	for _, useMise := range []bool{false, true} {
		t.Run(strconv.FormatBool(useMise), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			fakeInstallerRunner(t, func(cmd *exec.Cmd) error {
				calls++
				if cmd.Cancel == nil {
					t.Fatal("installer command is not cancellable")
				}
				cancel()
				return errors.New("fake process killed")
			})
			err := RunInstallerWithFallback("installer.jar", "forge", "1.20.1", "47.1", "official", useMise, useMise, 3, "", ctx)
			if !errors.Is(err, context.Canceled) || calls != 1 {
				t.Fatalf("error = %v, calls = %d", err, calls)
			}
			// An already cancelled request must not even call the runner.
			err = RunInstaller("installer.jar", "forge", "1.20.1", "47.1", "official", useMise, useMise, ctx)
			if !errors.Is(err, context.Canceled) || calls != 1 {
				t.Fatalf("pre-cancelled error = %v, calls = %d", err, calls)
			}
		})
	}
}

func TestInstallerRetriesAfterSuccessfulExitWithMissingArtifacts(t *testing.T) {
	t.Chdir(t.TempDir())
	installer := createTestInstaller(t, false)
	calls := 0
	fakeInstallerRunner(t, func(cmd *exec.Cmd) error {
		calls++
		if calls == 2 {
			writeInstallerTestFile(t, "libraries/net/minecraftforge/forge/1.20.1-47.1/unix_args.txt", "arguments")
		}
		return nil
	})
	if err := runInstallerRetry(installer, "forge", "1.20.1", "47.1", "official", false, false, 2, ""); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("missing artifact did not trigger retry: calls %d", calls)
	}
}

func TestInstallerMissingMetadataOutputTriggersFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	installer := createTestInstaller(t, true)
	writeInstallerTestFile(t, "libraries/net/minecraftforge/forge/1.20.1-47.1/unix_args.txt", "arguments")
	info, err := ExtractVersionJson(installer)
	if err != nil || len(info.RequiredServerArtifacts) == 0 {
		t.Fatalf("fixture must declare a required output: %#v, %v", info, err)
	}
	var commands [][]string
	fakeInstallerRunner(t, func(cmd *exec.Cmd) error {
		commands = append(commands, append([]string(nil), cmd.Args...))
		if len(commands) == 2 {
			writeInstallerTestFile(t, "libraries/test/generated/1/generated-1-server.jar", "generated")
		}
		return nil
	})
	if err := RunInstallerWithFallback(installer, "forge", "1.20.1", "47.1", "official", false, false, 1, ""); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 || strings.Contains(strings.Join(commands[0], " "), "--mirror") || !strings.Contains(strings.Join(commands[1], " "), "--mirror") {
		t.Fatalf("missing generated artifact did not trigger source fallback: %v", commands)
	}
}

func TestInstallerFailureAndEmptyArtifactNeverSucceed(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail bool
	}{{"command failure", true}, {"empty artifact", false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			installer := createTestInstaller(t, false)
			writeInstallerTestFile(t, "libraries/net/neoforged/neoforge/21.1/unix_args.txt", "")
			calls := 0
			fakeInstallerRunner(t, func(cmd *exec.Cmd) error {
				calls++
				if tc.fail {
					return errors.New("fake exit failure")
				}
				return nil
			})
			err := RunInstallerWithFallback(installer, "neoforge", "1.21.1", "21.1", "bmclapi", false, false, 2, "")
			if err == nil || calls != 4 {
				t.Fatalf("error = %v, calls = %d", err, calls)
			}
			if _, err := os.Stat("run.sh"); !os.IsNotExist(err) {
				t.Fatalf("our installer wrapper unexpectedly wrote run.sh: %v", err)
			}
		})
	}
}

func TestInstallerUsesJavaHomeAndScopedMise(t *testing.T) {
	home := filepath.Join(t.TempDir(), "JDK with spaces")
	java := filepath.Join(home, "bin", "java")
	writeInstallerTestFile(t, java, "fake")
	t.Setenv("JAVA_HOME", home)
	var commands [][]string
	fakeInstallerRunner(t, func(cmd *exec.Cmd) error {
		commands = append(commands, append([]string(nil), cmd.Args...))
		return nil
	})
	if err := RunInstaller("installer.jar", "forge", "1.20.1", "47.1", "official", false, false); err != nil {
		t.Fatal(err)
	}
	if commands[0][0] != java {
		t.Fatalf("JAVA_HOME was not used: %v", commands)
	}
	if err := RunInstaller("installer.jar", "forge", "1.20.1", "47.1", "bmclapi", true, true); err != nil {
		t.Fatal(err)
	}
	want := "mise exec java@17 -- java -jar installer.jar --installServer --mirror https://bmclapi2.bangbang93.com/maven/"
	if strings.Join(commands[1], " ") != want {
		t.Fatalf("mise command = %v", commands[1])
	}
}

func TestRunScriptLayoutsJavaVersionsAndMemory(t *testing.T) {
	for _, tc := range []struct {
		name, mc, loader, loaderVersion, installPath, artifact, fragment string
		major                                                            int
	}{
		{"legacy forge", "1.12.2", "forge", "14.23", "custom forge.jar", "custom forge.jar", "-jar 'custom forge.jar'", 8},
		{"forge java16", "1.17.1", "forge", "37.1", "", "libraries/net/minecraftforge/forge/1.17.1-37.1/unix_args.txt", "'@libraries/net/minecraftforge/forge/1.17.1-37.1/unix_args.txt'", 16},
		{"forge java17", "1.20.1", "forge", "47.1", "", "libraries/net/minecraftforge/forge/1.20.1-47.1/unix_args.txt", "'@libraries/net/minecraftforge/forge/1.20.1-47.1/unix_args.txt'", 17},
		{"neoforge java21", "1.21.1", "neoforge", "21.1", "", "libraries/net/neoforged/neoforge/21.1/unix_args.txt", "'@libraries/net/neoforged/neoforge/21.1/unix_args.txt'", 21},
		{"neoforge java25", "26.2", "neoforge", "26.2.1", "", "libraries/net/neoforged/neoforge/26.2.1/unix_args.txt", "'@libraries/net/neoforged/neoforge/26.2.1/unix_args.txt'", 25},
		{"fabric launch jar", "1.21.1", "fabric", "0.16", "", "fabric-server-launch.jar", "-jar fabric-server-launch.jar", 21},
		{"vanilla", "1.16.5", "vanilla", "", "", "server.jar", "-jar server.jar", 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("JAVA_HOME", "")
			t.Setenv("SERVER_MEMORY", "8192")
			writeInstallerTestFile(t, tc.artifact, "artifact")
			if tc.loader == "fabric" {
				writeInstallerTestFile(t, "server.jar", "server")
			}
			// A fixed old temporary name must not be touched.
			writeInstallerTestFile(t, "run.sh.tmp", "user data")
			if err := RunScript(tc.mc, tc.loader, tc.loaderVersion, true, true, "-Xmx{maxmen}M", tc.installPath); err != nil {
				t.Fatal(err)
			}
			script, err := os.ReadFile("run.sh")
			if err != nil {
				t.Fatal(err)
			}
			want := "#!/bin/bash\nmise exec java@" + strconv.Itoa(tc.major) + " -- java -Xmx6692M " + tc.fragment + " \"$@\"\n"
			if string(script) != want {
				t.Fatalf("script = %q, want %q", script, want)
			}
			assertDownloadContent(t, "run.sh.tmp", "user data")
			matches, err := filepath.Glob(".autoinst-run-*.tmp")
			if err != nil || len(matches) != 0 {
				t.Fatalf("owned temporary scripts remain: %v, %v", matches, err)
			}
		})
	}
}

func TestRunScriptPreservesExistingScriptWhenArtifactsInvalid(t *testing.T) {
	t.Chdir(t.TempDir())
	writeInstallerTestFile(t, "run.sh", "official installer or user script")
	if err := RunScript("1.21.1", "forge", "51.1", false, false, "", ""); err == nil {
		t.Fatal("missing args accepted")
	}
	assertDownloadContent(t, "run.sh", "official installer or user script")
	writeInstallerTestFile(t, "libraries/net/minecraftforge/forge/1.21.1-51.1/unix_args.txt", "args")
	metadata := VersionInfo{RequiredServerArtifacts: []RequiredServerArtifact{{Path: "libraries/required.jar"}}}
	if err := RunScript("1.21.1", "forge", "51.1", false, false, "", "", metadata); err == nil {
		t.Fatal("missing metadata output accepted")
	}
	assertDownloadContent(t, "run.sh", "official installer or user script")
	writeInstallerTestFile(t, "libraries/required.jar", "required")
	metadata.RequiredServerArtifacts[0].SHA1 = strings.Repeat("0", 40)
	if err := RunScript("1.21.1", "forge", "51.1", false, false, "", "", metadata); err == nil {
		t.Fatal("invalid metadata output hash accepted")
	}
	assertDownloadContent(t, "run.sh", "official installer or user script")
}

func TestRunScriptUsesJavaHomeAndSimpfunPaths(t *testing.T) {
	t.Chdir(t.TempDir())
	writeInstallerTestFile(t, "server.jar", "server")
	home := filepath.Join(t.TempDir(), "JDK's home")
	java := filepath.Join(home, "bin", "java")
	writeInstallerTestFile(t, java, "fake")
	t.Setenv("JAVA_HOME", home)
	if err := RunScript("1.21.1", "vanilla", "", false, false, "", ""); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("run.sh")
	if err != nil || !strings.Contains(string(script), shellQuote(filepath.ToSlash(java))) {
		t.Fatalf("script does not use quoted JAVA_HOME: %q, %v", script, err)
	}
	for _, tc := range []struct{ mc, path string }{
		{"1.16.5", "/usr/bin/jdk/jdk1.8.0_361/bin/java"},
		{"1.17.1", "/usr/bin/jdk/jdk-16/bin/java"},
		{"1.20.1", "/usr/bin/jdk/jdk-17.0.6/bin/java"},
		{"1.21.1", "/usr/bin/jdk/jdk-21.0.2/bin/java"},
		{"26.2", "/usr/bin/jdk/jdk-25.0.2/bin/java"},
	} {
		if err := RunScript(tc.mc, "vanilla", "", true, false, "", ""); err != nil {
			t.Fatal(err)
		}
		script, err := os.ReadFile("run.sh")
		if err != nil || !strings.Contains(string(script), shellQuote(tc.path)) {
			t.Fatalf("wrong simpfun Java for %s: %q, %v", tc.mc, script, err)
		}
	}
}

func TestRuntimeJavaCompatibility(t *testing.T) {
	for version, want := range map[string]int{"1.12.2": 8, "1.17.1": 16, "1.18.2": 17, "1.20.4": 17, "1.20.5": 21, "1.21.11": 21, "1.21.12": 25, "26.2": 25, "26w01a": 25, "25w01a": 21} {
		if got := runtimeJava(version); got != want {
			t.Errorf("runtimeJava(%q) = %d, want %d", version, got, want)
		}
	}
}
