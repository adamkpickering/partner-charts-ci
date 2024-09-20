package upstreamyaml

import (
	"os"
	"path/filepath"

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

func Parse(packagePath string) (UpstreamYaml, error) {
	upstreamYamlPath := filepath.Join(packagePath, UpstreamOptionsFile)
	logrus.Debugf("Attempting to parse %s", upstreamYamlPath)
	upstreamYamlFile, err := os.ReadFile(upstreamYamlPath)
	upstreamYaml := UpstreamYaml{}
	if err != nil {
		logrus.Debug(err)
	} else {
		err = yaml.Unmarshal(upstreamYamlFile, &upstreamYaml)
	}

	return upstreamYaml, err
}
