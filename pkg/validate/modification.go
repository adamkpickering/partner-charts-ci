package validate

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/rancher/partner-charts-ci/pkg/conform"
	"github.com/rancher/partner-charts-ci/pkg/paths"
	"github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/chart/loader"
)

type DirectoryComparison struct {
	Unchanged []string
	Modified  []string
	Added     []string
	Removed   []string
}

func (directoryComparison *DirectoryComparison) Match() bool {
	return len(directoryComparison.Modified)+len(directoryComparison.Added)+len(directoryComparison.Removed) == 0
}

func (directoryComparison *DirectoryComparison) Merge(newComparison DirectoryComparison) {
	directoryComparison.Unchanged = append(directoryComparison.Unchanged, newComparison.Unchanged...)
	directoryComparison.Modified = append(directoryComparison.Modified, newComparison.Modified...)
	directoryComparison.Added = append(directoryComparison.Added, newComparison.Added...)
	directoryComparison.Removed = append(directoryComparison.Removed, newComparison.Removed...)
}

// PreventReleasedChartModifications validates that no released chart
// versions have been modified outside of a few that must be allowed,
// such as the deprecated field of Chart.yaml and Rancher-specific
// annotations.
func PreventReleasedChartModifications(configYaml ConfigurationYaml) []error {
	cloneDir, err := os.MkdirTemp("", "gitRepo")
	if err != nil {
		logrus.Fatal(err)
	}
	defer os.RemoveAll(cloneDir)

	err = CloneRepo(configYaml.ValidateUpstreams[0].Url, configYaml.ValidateUpstreams[0].Branch, cloneDir)
	if err != nil {
		logrus.Fatal(err)
	}

	directoryComparison := DirectoryComparison{}
	for _, dirPath := range []string{"assets"} {
		upstreamPath := path.Join(cloneDir, dirPath)
		updatePath := path.Join(paths.GetRepoRoot(), dirPath)
		if _, err := os.Stat(updatePath); os.IsNotExist(err) {
			logrus.Infof("Directory '%s' not in source. Skipping...", dirPath)
			continue
		}
		if _, err := os.Stat(upstreamPath); os.IsNotExist(err) {
			logrus.Infof("Directory '%s' not in upstream. Skipping...", dirPath)
			continue
		}
		newComparison, err := CompareDirectories(upstreamPath, updatePath)
		if err != nil {
			logrus.Error(err)
		}
		directoryComparison.Merge(newComparison)
	}

	errors := make([]error, 0, len(directoryComparison.Modified))
	for _, modifiedFile := range directoryComparison.Modified {
		errors = append(errors, fmt.Errorf("%s was modified", modifiedFile))
	}
	return errors
}

func CloneRepo(url string, branch string, targetDir string) error {
	branchReference := fmt.Sprintf("refs/heads/%s", branch)
	cloneOptions := git.CloneOptions{
		URL:           url,
		ReferenceName: plumbing.ReferenceName(branchReference),
		SingleBranch:  true,
		Depth:         1,
	}

	_, err := git.PlainClone(targetDir, false, &cloneOptions)
	if err != nil {
		return err
	}

	return nil
}

func ChecksumFile(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	hash := fmt.Sprintf("%x", h.Sum(nil))

	return hash, nil
}

func CompareDirectories(upstreamPath, updatePath string) (DirectoryComparison, error) {
	logrus.Debugf("Comparing directories %s and %s", upstreamPath, updatePath)
	directoryComparison := DirectoryComparison{}
	checkedSet := make(map[string]struct{})
	var checked = struct{}{}

	if _, err := os.Stat(upstreamPath); os.IsNotExist(err) {
		return directoryComparison, err
	}
	if _, err := os.Stat(updatePath); os.IsNotExist(err) {
		return directoryComparison, err
	}

	findRemovalAndModification := func(upstreamFilePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relativePath := strings.TrimPrefix(upstreamFilePath, upstreamPath)
		checkedSet[relativePath] = checked

		if info.IsDir() {
			return nil
		}

		updateFilePath := path.Join(updatePath, relativePath)
		if _, err := os.Stat(updateFilePath); os.IsNotExist(err) {
			directoryComparison.Removed = append(directoryComparison.Removed, updateFilePath)
			return nil
		}
		leftCheckSum, err := ChecksumFile(upstreamFilePath)
		if err != nil {
			logrus.Error(err)
		}
		rightCheckSum, err := ChecksumFile(updateFilePath)
		if err != nil {
			logrus.Error(err)
		}

		if leftCheckSum != rightCheckSum && strings.HasSuffix(upstreamFilePath, ".tgz") {
			chartMatch, err := matchHelmCharts(upstreamFilePath, updateFilePath)
			if chartMatch {
				directoryComparison.Unchanged = append(directoryComparison.Unchanged, updateFilePath)
			} else {
				directoryComparison.Modified = append(directoryComparison.Modified, updateFilePath)
			}
			if err != nil {
				logrus.Debug(err)
			}
		} else if leftCheckSum != rightCheckSum {
			directoryComparison.Modified = append(directoryComparison.Modified, updateFilePath)
		} else {
			directoryComparison.Unchanged = append(directoryComparison.Unchanged, updateFilePath)
		}

		return nil
	}

	findAddition := func(updateFilePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relativePath := strings.TrimPrefix(updateFilePath, updatePath)

		if _, ok := checkedSet[relativePath]; !ok && !info.IsDir() {
			directoryComparison.Added = append(directoryComparison.Added, updateFilePath)
		}

		return nil
	}

	if err := filepath.Walk(upstreamPath, findRemovalAndModification); err != nil {
		return DirectoryComparison{}, fmt.Errorf("failed to search %q for removed or modified files: %w", upstreamPath, err)
	}
	if err := filepath.Walk(updatePath, findAddition); err != nil {
		return DirectoryComparison{}, fmt.Errorf("failed to search %q for added files: %w", updatePath, err)
	}

	return directoryComparison, nil
}

// prepareTgzForComparison takes a path to a .tgz helm chart. It unpacks this
// helm chart, applies modifications to it that cause the validation process
// to ignore certain changes, and exports it to a temporary chart directory.
// The caller is responsible for removing the chart directory after they are
// finished using it.
func prepareTgzForComparison(tgzPath string) (string, error) {
	upstreamFile, err := os.Open(tgzPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer upstreamFile.Close()

	upstreamChart, err := loader.LoadArchive(upstreamFile)
	if err != nil {
		return "", fmt.Errorf("failed to load archive: %w", err)
	}

	// Do not consider changes to partner-charts-specific chart annotations
	for annotation := range upstreamChart.Metadata.Annotations {
		if strings.HasPrefix(annotation, "catalog.cattle.io") {
			delete(upstreamChart.Metadata.Annotations, annotation)
		}
	}

	// Do not consider changes to the Chart.yaml deprecated field
	upstreamChart.Metadata.Deprecated = false

	chartDirectory, err := os.MkdirTemp("", "partner-charts-ci-validate-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	if err = conform.ExportChartDirectory(upstreamChart, chartDirectory); err != nil {
		return "", fmt.Errorf("failed to export chart directory: %w", err)
	}

	return chartDirectory, nil
}

func matchHelmCharts(upstreamPath, updatePath string) (bool, error) {
	upstreamChartDirectory, err := prepareTgzForComparison(upstreamPath)
	if err != nil {
		return false, fmt.Errorf("failed to prepare %s for comparison: %w", upstreamPath, err)
	}
	defer os.RemoveAll(upstreamChartDirectory)

	updateChartDirectory, err := prepareTgzForComparison(updatePath)
	if err != nil {
		return false, fmt.Errorf("failed to prepare %s for comparison: %w", updatePath, err)
	}
	defer os.RemoveAll(updateChartDirectory)

	directoryComparison, err := CompareDirectories(upstreamChartDirectory, updateChartDirectory)
	if err != nil {
		return false, fmt.Errorf("failed to compare directories: %w", err)
	}

	return directoryComparison.Match(), err
}
