package modcheck

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

const modrinthAPI = "https://api.modrinth.com/v2"

type versionFileResponse map[string]struct {
	ProjectID string `json:"project_id"`
}

type projectInfo struct {
	ID         string   `json:"id"`
	ClientSide string   `json:"client_side"`
	ServerSide string   `json:"server_side"`
	Categories []string `json:"categories"`
}

func filterByModrinthProject(client *http.Client, files []ModFile) []string {
	projectToFiles := make(map[string][]string)
	projectIDs := make([]string, 0, len(files))
	seen := make(map[string]struct{})

	for _, file := range files {
		for _, projectID := range file.ProjectIDs {
			if projectID == "" {
				continue
			}
			projectToFiles[projectID] = append(projectToFiles[projectID], file.Path)
			if _, ok := seen[projectID]; ok {
				continue
			}
			seen[projectID] = struct{}{}
			projectIDs = append(projectIDs, projectID)
		}
	}

	projects := fetchModrinthProjects(client, projectIDs)
	if len(projects) == 0 {
		return nil
	}

	result := make([]string, 0)
	seenPath := make(map[string]struct{})
	for projectID, info := range projects {
		if !isClientProject(info, true) {
			continue
		}
		for _, filePath := range projectToFiles[projectID] {
			if _, ok := seenPath[filePath]; ok {
				continue
			}
			seenPath[filePath] = struct{}{}
			result = append(result, filePath)
		}
	}

	return result
}

func filterByModrinthHash(client *http.Client, files []ModFile) []string {
	if len(files) == 0 {
		return nil
	}

	hashes := make([]string, 0, len(files))
	hashToFile := make(map[string]string, len(files))
	for _, file := range files {
		if file.Hash == "" {
			continue
		}
		hashes = append(hashes, file.Hash)
		hashToFile[file.Hash] = file.Path
	}
	if len(hashes) == 0 {
		return nil
	}

	payload, _ := json.Marshal(map[string]any{
		"hashes":    hashes,
		"algorithm": "sha1",
	})

	req, err := http.NewRequest(http.MethodPost, modrinthAPI+"/version_files", bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "autoinst/1.3.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var versionFiles versionFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&versionFiles); err != nil {
		return nil
	}

	projectToFiles := make(map[string][]string)
	projectIDs := make([]string, 0, len(versionFiles))
	seen := make(map[string]struct{})
	for hash, info := range versionFiles {
		filePath := hashToFile[hash]
		if filePath == "" || info.ProjectID == "" {
			continue
		}
		projectToFiles[info.ProjectID] = append(projectToFiles[info.ProjectID], filePath)
		if _, ok := seen[info.ProjectID]; ok {
			continue
		}
		seen[info.ProjectID] = struct{}{}
		projectIDs = append(projectIDs, info.ProjectID)
	}

	projects := fetchModrinthProjects(client, projectIDs)
	if len(projects) == 0 {
		return nil
	}

	result := make([]string, 0)
	seenPath := make(map[string]struct{})
	for projectID, info := range projects {
		if !isClientProject(info, false) {
			continue
		}
		for _, filePath := range projectToFiles[projectID] {
			if _, ok := seenPath[filePath]; ok {
				continue
			}
			seenPath[filePath] = struct{}{}
			result = append(result, filePath)
		}
	}

	return result
}

func fetchModrinthProjects(client *http.Client, projectIDs []string) map[string]projectInfo {
	if len(projectIDs) == 0 {
		return nil
	}

	results := make(map[string]projectInfo, len(projectIDs))
	const batchSize = 50

	for start := 0; start < len(projectIDs); start += batchSize {
		end := start + batchSize
		if end > len(projectIDs) {
			end = len(projectIDs)
		}

		batch := projectIDs[start:end]
		idsJSON, _ := json.Marshal(batch)
		requestURL := modrinthAPI + "/projects?ids=" + url.QueryEscape(string(idsJSON))

		req, err := http.NewRequest(http.MethodGet, requestURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "autoinst/1.3.0")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var batchProjects []projectInfo
		if err := json.NewDecoder(resp.Body).Decode(&batchProjects); err == nil {
			for _, project := range batchProjects {
				if project.ID == "" {
					continue
				}
				results[project.ID] = project
			}
		}
		resp.Body.Close()
	}

	return results
}

func isClientProject(project projectInfo, allowOptional bool) bool {
	clientSide := strings.TrimSpace(project.ClientSide)
	serverSide := strings.TrimSpace(project.ServerSide)
	if clientSide == "required" && serverSide == "unsupported" {
		return true
	}
	return allowOptional && clientSide == "optional" && serverSide == "unsupported"
}
