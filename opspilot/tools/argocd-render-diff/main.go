package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type manifest struct {
	Kind     string `yaml:"kind" json:"kind"`
	Metadata struct {
		Name      string `yaml:"name" json:"name"`
		Namespace string `yaml:"namespace" json:"namespace,omitempty"`
	} `yaml:"metadata" json:"metadata"`
}

type result struct {
	OldPath       string   `json:"old_path"`
	NewPath       string   `json:"new_path"`
	OldCount      int      `json:"old_count"`
	NewCount      int      `json:"new_count"`
	CommonCount   int      `json:"common_count"`
	OnlyOld       []string `json:"only_old,omitempty"`
	OnlyNew       []string `json:"only_new,omitempty"`
	RenderCommand string   `json:"render_command"`
}

func main() {
	oldPath := flag.String("old", "", "old Argo CD core Kustomize path")
	newPath := flag.String("new", "", "new Argo CD core Kustomize path")
	flag.Parse()
	if strings.TrimSpace(*oldPath) == "" || strings.TrimSpace(*newPath) == "" {
		fmt.Fprintln(os.Stderr, "--old and --new are required")
		os.Exit(2)
	}
	oldRendered, cmdName, err := render(*oldPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render old failed: %v\n", err)
		os.Exit(1)
	}
	newRendered, _, err := render(*newPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render new failed: %v\n", err)
		os.Exit(1)
	}
	oldIDs, err := identities(oldRendered)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse old failed: %v\n", err)
		os.Exit(1)
	}
	newIDs, err := identities(newRendered)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse new failed: %v\n", err)
		os.Exit(1)
	}
	out := compare(*oldPath, *newPath, cmdName, oldIDs, newIDs)
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json output failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(body))
}

func render(path string) ([]byte, string, error) {
	commands := [][]string{
		{"kubectl", "kustomize", path},
		{"kustomize", "build", path},
	}
	var lastErr error
	for _, parts := range commands {
		cmd := exec.Command(parts[0], parts[1:]...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err == nil {
			return out, strings.Join(parts[:len(parts)-1], " "), nil
		}
		lastErr = fmt.Errorf("%s: %w: %s", strings.Join(parts, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil, "", lastErr
}

func identities(body []byte) ([]string, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	ids := []string{}
	for {
		var item manifest
		err := decoder.Decode(&item)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if item.Kind == "" || item.Metadata.Name == "" {
			continue
		}
		namespace := item.Metadata.Namespace
		if namespace == "" {
			namespace = "_cluster"
		}
		ids = append(ids, item.Kind+"/"+namespace+"/"+item.Metadata.Name)
	}
	sort.Strings(ids)
	return ids, nil
}

func compare(oldPath, newPath, cmdName string, oldIDs, newIDs []string) result {
	oldSet := toSet(oldIDs)
	newSet := toSet(newIDs)
	onlyOld := []string{}
	onlyNew := []string{}
	common := 0
	for _, id := range oldIDs {
		if newSet[id] {
			common++
			continue
		}
		onlyOld = append(onlyOld, id)
	}
	for _, id := range newIDs {
		if !oldSet[id] {
			onlyNew = append(onlyNew, id)
		}
	}
	return result{
		OldPath:       oldPath,
		NewPath:       newPath,
		OldCount:      len(oldIDs),
		NewCount:      len(newIDs),
		CommonCount:   common,
		OnlyOld:       onlyOld,
		OnlyNew:       onlyNew,
		RenderCommand: cmdName,
	}
}

func toSet(items []string) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		out[item] = true
	}
	return out
}
