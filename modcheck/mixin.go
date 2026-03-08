package modcheck

import (
	"encoding/json"
	"strings"
)

func filterByMixin(files []ModFile) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})

	for _, file := range files {
		if strings.Contains(strings.ToLower(file.Name), "lib") {
			continue
		}

		for _, mixin := range file.Mixins {
			var config struct {
				Mixins []any `json:"mixins"`
				Client []any `json:"client"`
			}
			if err := json.Unmarshal([]byte(mixin.Data), &config); err != nil {
				continue
			}
			if len(config.Mixins) == 0 && len(config.Client) > 0 {
				if _, ok := seen[file.Path]; ok {
					break
				}
				seen[file.Path] = struct{}{}
				result = append(result, file.Path)
				break
			}
		}
	}

	return result
}
