package main

import (
	"os"
	"path/filepath"
)

type generatedFile struct {
	path string
	body string
}

func onboardFiles(cfg onboardServiceConfig) []generatedFile {
	files := []generatedFile{}
	if cfg.DockerMode == "generate" {
		files = append(files, generatedFile{path: cfg.DockerPath, body: dockerfileTemplate(cfg)})
	}
	if cfg.CIMode == "" || cfg.CIMode == "include" || cfg.CIMode == "generate" {
		files = append(files, generatedFile{path: ".gitlab-ci.yml", body: gitlabCIIncludeTemplate(cfg)})
	}
	files = append(files,
		generatedFile{path: filepath.Join("deploy", "k8s", "namespace.yaml"), body: namespaceTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "limitrange.yaml"), body: limitRangeTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "resourcequota.yaml"), body: resourceQuotaTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "serviceaccount.yaml"), body: serviceAccountTemplate(cfg)},
	)
	if len(cfg.ConfigSources) > 0 {
		files = append(files, generatedFile{path: filepath.Join("deploy", "k8s", "configmap.yaml"), body: configSourcesConfigMapTemplate(cfg)})
	}
	files = append(files,
		generatedFile{path: filepath.Join("deploy", "k8s", "deployment.yaml"), body: deploymentTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "service.yaml"), body: serviceTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "kustomization.yaml"), body: kustomizationTemplate(cfg)},
		generatedFile{path: filepath.Join(".opspilot", "quality.yaml"), body: qualityTemplate(cfg)},
		generatedFile{path: "opspilot.release-service.txt", body: releaseMapping(cfg) + "\n"},
	)
	for _, item := range cfg.Middleware {
		if middlewareAutoProvisioned(item) {
			files = append(files, generatedFile{
				path: filepath.Join("deploy", "k8s", "middleware-"+item.Name+".yaml"),
				body: middlewareTemplate(cfg, item),
			})
		}
	}
	return files
}

func writeGeneratedFile(path, body string, force bool) (string, error) {
	if _, err := os.Stat(path); err == nil && !force {
		return "skipped_existing", nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return "written", nil
}
