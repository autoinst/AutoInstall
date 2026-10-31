package pkg

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/autoinst/AutoInstall/core"
)

const testForgePromotions = `{"homepage":"https://files.minecraftforge.net/net/minecraftforge/forge/","promos":{"1.9-latest":"11.14.4.1577","1.10.2-latest":"12.18.3.2511","26.2-latest":"65.1.5","26.3-latest":"66.1.2","26.3-recommended":"66.1.0","27.1-recommended":"70.0.0"}}`

func TestForgeVersionSourcePreferenceAndFallback(t *testing.T) {
	for _, source := range []string{"official", "none", "bmclapi"} {
		for _, mc := range []string{"", "26.3"} {
			for _, failure := range []string{"", "unavailable", "missing", "malformed"} {
				t.Run(fmt.Sprintf("%s/mc=%s/failure=%s", source, mc, failure), func(t *testing.T) {
					old := core.HTTPClient
					t.Cleanup(func() { core.HTTPClient = old })
					var requests []string
					core.HTTPClient = &http.Client{Transport: libraryTransport(func(r *http.Request) (*http.Response, error) {
						requests = append(requests, r.URL.String())
						isOfficial := r.URL.String() == forgePromotionsURL
						if !isOfficial && r.URL.Host != "bmclapi2.bangbang93.com" {
							t.Fatalf("unexpected source: %s", r.URL)
						}
						body := testForgePromotions
						if !isOfficial {
							if mc == "" {
								if r.URL.Path != "/forge/latest" {
									t.Fatalf("unexpected global query: %s", r.URL)
								}
								body = `{"build":{"mcversion":"26.3","version":"66.1.9"}}`
							} else {
								if r.URL.Path != "/forge/minecraft/26.3" {
									t.Fatalf("unexpected matching query: %s", r.URL)
								}
								body = `[{"version":"66.1.8"},{"version":"66.1.9"}]`
							}
						}
						status := 200
						isPrimary := isOfficial == (source != "bmclapi")
						if isPrimary {
							switch failure {
							case "unavailable":
								status, body = 503, "unavailable"
							case "missing":
								if isOfficial {
									body = `{"promos":{"26.3-recommended":"66.1.0"}}`
								} else if mc == "" {
									body = `{"build":{}}`
								} else {
									body = `[]`
								}
							case "malformed":
								body = `{`
							}
						}
						return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
					})}
					var gotMC, loader string
					var err error
					if mc == "" {
						gotMC, loader, err = FetchLatestForgeVersion(1, source)
					} else {
						gotMC = mc
						loader, err = FetchLatestForgeLoaderForVersion(mc, 1, source)
					}
					wantOfficial := source != "bmclapi"
					wantRequests := 1
					if failure != "" {
						wantOfficial = !wantOfficial
						wantRequests = 2
					}
					wantLoader := "66.1.9"
					if wantOfficial {
						wantLoader = "66.1.2"
					}
					if err != nil || gotMC != "26.3" || loader != wantLoader || len(requests) != wantRequests {
						t.Fatalf("target=%s/%s requests=%v error=%v", gotMC, loader, requests, err)
					}
					if (requests[0] == forgePromotionsURL) != (source != "bmclapi") {
						t.Fatalf("source preference ignored: %v", requests)
					}
				})
			}
		}
	}
}

func TestForgePromotionTarget(t *testing.T) {
	for _, tc := range []struct {
		name, mc, wantMC, wantLoader string
		promos                       map[string]string
		fail                         bool
	}{
		{"numeric Minecraft ordering", "", "1.10", "12.0.1", map[string]string{"1.9-latest": "11.0.1", "1.10-latest": "12.0.1"}, false},
		{"exact version", "1.7.10", "1.7.10", "10.13.4.1614", map[string]string{"1.7.10-latest": "10.13.4.1614"}, false},
		{"recommended is not latest", "26.3", "", "", map[string]string{"26.3-recommended": "66.1.0"}, true},
		{"empty promos", "", "", "", nil, true},
		{"invalid loader", "26.3", "", "", map[string]string{"26.3-latest": "../outside"}, true},
		{"invalid Minecraft", "", "", "", map[string]string{"../outside-latest": "66.1.2"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mc, loader, err := forgePromotionTarget(tc.promos, tc.mc)
			if (err != nil) != tc.fail || mc != tc.wantMC || loader != tc.wantLoader {
				t.Fatalf("target=%s/%s error=%v", mc, loader, err)
			}
		})
	}
}

func TestForgeVersionLookupFailsAfterBothSources(t *testing.T) {
	old := core.HTTPClient
	t.Cleanup(func() { core.HTTPClient = old })
	calls := 0
	core.HTTPClient = &http.Client{Transport: libraryTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable")), ContentLength: 11}, nil
	})}
	if _, err := FetchLatestForgeLoaderForVersion("26.3", 2, "bmclapi"); err == nil || calls != 4 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestForgeTargetUsesMatchingLatestWithoutListingBuilds(t *testing.T) {
	for _, loader := range []string{"latest", "66.1.0"} {
		t.Run(loader, func(t *testing.T) {
			calls := 0
			got, err := resolveForgeTarget(core.InstConfig{Version: "26.3", LoaderVersion: loader},
				func(int) (string, string, error) {
					t.Fatal("queried global latest for fixed Minecraft")
					return "", "", nil
				},
				func(string, int) ([]string, error) {
					t.Fatal("listed builds instead of matching latest")
					return nil, nil
				},
				func(mc string, retries int) (string, error) {
					calls++
					if mc != "26.3" {
						t.Fatalf("Minecraft overwritten: %s", mc)
					}
					return "66.1.2", nil
				},
			)
			want, wantCalls := "66.1.0", 0
			if loader == "latest" {
				want, wantCalls = "66.1.2", 1
			}
			if err != nil || got.Version != "26.3" || got.LoaderVersion != want || calls != wantCalls {
				t.Fatalf("target=%+v calls=%d error=%v", got, calls, err)
			}
		})
	}
}
