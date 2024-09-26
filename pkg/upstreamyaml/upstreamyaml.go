package upstreamyaml

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"helm.sh/helm/v3/pkg/chart"

	"sigs.k8s.io/yaml"
)

const (
	UpstreamOptionsFile = "upstream.yaml"
)

type UpstreamYaml struct {
	AHPackageName      string         `json:"ArtifactHubPackage"`
	AHRepoName         string         `json:"ArtifactHubRepo"`
	AutoInstall        string         `json:"AutoInstall"`
	ChartYaml          chart.Metadata `json:"ChartMetadata"`
	DisplayName        string         `json:"DisplayName"`
	Experimental       bool           `json:"Experimental"`
	Fetch              string         `json:"Fetch"`
	GitBranch          string         `json:"GitBranch"`
	GitHubRelease      bool           `json:"GitHubRelease"`
	GitRepoUrl         string         `json:"GitRepo"`
	GitSubDirectory    string         `json:"GitSubdirectory"`
	HelmChart          string         `json:"HelmChart"`
	HelmRepoUrl        string         `json:"HelmRepo"`
	Hidden             bool           `json:"Hidden"`
	Namespace          string         `json:"Namespace"`
	PackageVersion     int            `json:"PackageVersion"`
	RemoteDependencies bool           `json:"RemoteDependencies"`
	TrackVersions      []string       `json:"TrackVersions"`
	ReleaseName        string         `json:"ReleaseName"`
	Vendor             string         `json:"Vendor"`
}

func (upstreamYaml *UpstreamYaml) SetDefaults() {
	if upstreamYaml.Fetch == "" {
		upstreamYaml.Fetch = "latest"
	}

	if upstreamYaml.ReleaseName == "" {
		upstreamYaml.ReleaseName = upstreamYaml.HelmChart
	}
}

func (upstreamYaml *UpstreamYaml) Validate() error {
	// if Fetch != "latest", HelmChart and HelmRepo must be present
	// if TrackVersions is present, HelmChart and HelmRepo must be present

	// if ArtifactHubPackage is present, ArtifactHubRepo must be present
	// if ArtifactHubRepo is present, ArtifactHubPackage must be present

	// if GitBranch is present, GitRepo must be present
	// if GitHubRelease is present, GitRepo must be present
	// if GitSubDirectory is present, GitRepo must be present

	// if HelmChart is present, HelmRepo must be present
	// if HelmRepo is present, HelmChart must be present

	// one of ArtifactHubPackage and ArtifactHubRepo, GitRepo (et al) or
	// HelmRepo and HelmChart must be present

	return nil
}

func Parse(upstreamYamlPath string) (*UpstreamYaml, error) {
	logrus.Debugf("Attempting to parse %s", upstreamYamlPath)
	contents, err := os.ReadFile(upstreamYamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %w", err)
	}

	upstreamYaml := &UpstreamYaml{}
	if err := yaml.Unmarshal(contents, &upstreamYaml); err != nil {
		return nil, fmt.Errorf("failed to parse as YAML: %w", err)
	}

	upstreamYaml.SetDefaults()

	if err := upstreamYaml.Validate(); err != nil {
		return nil, fmt.Errorf("invalid upstream.yaml: %w", err)
	}

	return upstreamYaml, nil
}
