package paths

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

var paths *Paths

// Paths is a type that contains all of the paths that are relevant
// to this tool. This approach facilitates unit testing.
type Paths struct {
	Assets            string
	Charts            string
	Packages          string
	ConfigurationYaml string
	IndexYaml         string
}

func Get() Paths {
	if paths == nil {
		repoRoot := GetRepoRoot()
		paths = &Paths{
			Assets:            filepath.Join(repoRoot, "assets"),
			Charts:            filepath.Join(repoRoot, "charts"),
			Packages:          filepath.Join(repoRoot, "packages"),
			ConfigurationYaml: filepath.Join(repoRoot, "configuration.yaml"),
			IndexYaml:         filepath.Join(repoRoot, "index.yaml"),
		}
	}
	return *paths
}

// Fetches absolute repository root path
func GetRepoRoot() string {
	repoRoot, err := os.Getwd()
	if err != nil {
		logrus.Fatal(err)
	}

	return repoRoot
}
