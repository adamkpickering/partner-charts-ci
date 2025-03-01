package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/go-git/go-git/v5"
	"github.com/rancher/partner-charts-ci/pkg/conform"
	"github.com/rancher/partner-charts-ci/pkg/fetcher"
	"github.com/rancher/partner-charts-ci/pkg/icons"
	p "github.com/rancher/partner-charts-ci/pkg/paths"
	"github.com/rancher/partner-charts-ci/pkg/pkg"
	"github.com/rancher/partner-charts-ci/pkg/upstreamyaml"
	"github.com/rancher/partner-charts-ci/pkg/utils"
	"github.com/rancher/partner-charts-ci/pkg/validate"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	annotationAutoInstall  = "catalog.cattle.io/auto-install"
	annotationCertified    = "catalog.cattle.io/certified"
	annotationDisplayName  = "catalog.cattle.io/display-name"
	annotationExperimental = "catalog.cattle.io/experimental"
	annotationFeatured     = "catalog.cattle.io/featured"
	annotationHidden       = "catalog.cattle.io/hidden"
	annotationKubeVersion  = "catalog.cattle.io/kube-version"
	annotationNamespace    = "catalog.cattle.io/namespace"
	annotationReleaseName  = "catalog.cattle.io/release-name"
	//packageEnvVariable sets the environment variable to check for a package name
	packageEnvVariable = "PACKAGE"
	featuredMax        = 5
	upstreamYamlFile   = "upstream.yaml"
)

var (
	version         = "v0.0.0"
	commit          = "HEAD"
	force           = false
	makeCommit      = false
	modifyGenerated = false
)

// ChartWrapper is like a chart.Chart, but it tracks whether the chart
// has been modified so that we can avoid making changes to chart
// artifacts when the chart has not been modified.
type ChartWrapper struct {
	*chart.Chart
	Modified bool
}

func NewChartWrapper(helmChart *chart.Chart) *ChartWrapper {
	return &ChartWrapper{
		Chart:    helmChart,
		Modified: false,
	}
}

func annotate(paths p.Paths, vendor, chartName, annotation, value string, remove, onlyLatest bool) error {
	existingCharts, err := loadExistingCharts(paths, vendor, chartName)
	if err != nil {
		return fmt.Errorf("failed to load existing charts: %w", err)
	}

	chartsToUpdate := make([]*ChartWrapper, 0, len(existingCharts))
	if onlyLatest {
		chartsToUpdate = append(chartsToUpdate, existingCharts[0])
	} else {
		chartsToUpdate = existingCharts
	}

	for _, chartToUpdate := range chartsToUpdate {
		if remove {
			chartToUpdate.Modified = conform.DeannotateChart(chartToUpdate.Chart, annotation, value)
		} else {
			chartToUpdate.Modified = conform.AnnotateChart(chartToUpdate.Chart, annotation, value, true)
		}
	}

	if err := writeCharts(paths, vendor, chartName, existingCharts); err != nil {
		return fmt.Errorf("failed to write charts: %w", err)
	}

	return nil
}

// sortPackageWrappers sorts a slice of PackageWrappers first by vendor
// and then by package name.
func sortPackageWrappers(packageWrappers []pkg.PackageWrapper) {
	slices.SortFunc(packageWrappers, func(a, b pkg.PackageWrapper) int {
		return strings.Compare(a.FullName(), b.FullName())
	})
}

// Commits changes to index file, assets, charts, and packages
func commitChanges(paths p.Paths, updatedPackageWrappers []pkg.PackageWrapper) error {
	commitOptions := git.CommitOptions{}

	r, err := git.PlainOpen(paths.RepoRoot)
	if err != nil {
		return err
	}

	wt, err := r.Worktree()
	if err != nil {
		return err
	}

	logrus.Info("Committing changes")

	if _, err := wt.Add(paths.Icons); err != nil {
		return fmt.Errorf("failed to add %q to working tree: %w", paths.Icons, err)
	}

	for _, packageWrapper := range updatedPackageWrappers {
		assetsPath := filepath.Join(paths.Assets, packageWrapper.Vendor)
		chartsPath := filepath.Join(paths.Charts, packageWrapper.Vendor, packageWrapper.Name)
		packagesPath := filepath.Join(paths.Packages, packageWrapper.Vendor, packageWrapper.Name)

		for _, path := range []string{assetsPath, chartsPath, packagesPath} {
			if _, err := wt.Add(path); err != nil {
				return fmt.Errorf("failed to add %q to working tree: %w", path, err)
			}
		}

		gitStatus, err := wt.Status()
		if err != nil {
			return err
		}

		for f, s := range gitStatus {
			if s.Worktree == git.Deleted {
				_, err = wt.Remove(f)
				if err != nil {
					return err
				}
			}
		}

	}

	if _, err := wt.Add(paths.IndexYaml); err != nil {
		return fmt.Errorf("failed to add %q to working tree: %w", paths.IndexYaml, err)
	}
	commitMessage := "Added chart versions:\n"
	sortPackageWrappers(updatedPackageWrappers)
	for _, packageWrapper := range updatedPackageWrappers {
		commitMessage += fmt.Sprintf("  %s:\n", packageWrapper.FullName())
		for _, version := range packageWrapper.FetchVersions {
			commitMessage += fmt.Sprintf("    - %s\n", version.Version)
		}
	}

	_, err = wt.Commit(commitMessage, &commitOptions)
	if err != nil {
		return err
	}

	gitStatus, err := wt.Status()
	if err != nil {
		return err
	}

	if !gitStatus.IsClean() {
		logrus.Fatal("Git status is not clean")
	}

	return nil
}

func ApplyUpdates(paths p.Paths, packageWrapper pkg.PackageWrapper) error {
	logrus.Debugf("Applying updates for package %s/%s\n", packageWrapper.Vendor, packageWrapper.Name)

	existingCharts, err := loadExistingCharts(paths, packageWrapper.Vendor, packageWrapper.Name)
	if err != nil {
		return fmt.Errorf("failed to load existing charts: %w", err)
	}

	// for new charts, convert repo.ChartVersions to *chart.Chart
	newCharts := make([]*ChartWrapper, 0, len(packageWrapper.FetchVersions))
	for _, chartVersion := range packageWrapper.FetchVersions {
		var newChart *chart.Chart
		var err error
		if packageWrapper.SourceMetadata.Source == "Git" {
			newChart, err = fetcher.LoadChartFromGit(chartVersion.URLs[0], packageWrapper.SourceMetadata.SubDirectory, packageWrapper.SourceMetadata.Commit)
		} else {
			newChart, err = fetcher.LoadChartFromUrl(chartVersion.URLs[0])
		}
		if err != nil {
			return fmt.Errorf("failed to fetch chart: %w", err)
		}
		newChart.Metadata.Version = chartVersion.Version
		newCharts = append(newCharts, NewChartWrapper(newChart))
	}

	if err := integrateCharts(paths, packageWrapper, existingCharts, newCharts); err != nil {
		return fmt.Errorf("failed to reconcile charts for package %q: %w", packageWrapper.Name, err)
	}

	allCharts := make([]*ChartWrapper, 0, len(existingCharts)+len(newCharts))
	allCharts = append(allCharts, existingCharts...)
	allCharts = append(allCharts, newCharts...)
	if err := writeCharts(paths, packageWrapper.Vendor, packageWrapper.Name, allCharts); err != nil {
		return fmt.Errorf("failed to write charts: %w", err)
	}

	return nil
}

// Copied from helm's chartutil.Save, which unfortunately does
// not split it out into a separate function.
func getTgzFilename(helmChart *chart.Chart) string {
	return fmt.Sprintf("%s-%s.tgz", helmChart.Name(), helmChart.Metadata.Version)
}

// writeCharts ensures that the relevant assets/ and charts/
// directories for package <vendor>/<chartName> reflect the set of
// packages passed in chartWrappers. In other words, charts that are
// not in chartWrappers are deleted, and charts from chartWrappers
// that are modified or do not exist on disk are written.
func writeCharts(paths p.Paths, vendor, chartName string, chartWrappers []*ChartWrapper) error {
	chartsDir := filepath.Join(paths.Charts, vendor, chartName)
	assetsDir := filepath.Join(paths.Assets, vendor)

	if err := os.RemoveAll(chartsDir); err != nil {
		return fmt.Errorf("failed to wipe existing charts directory: %w", err)
	}

	// delete any charts on disk that are not in chartWrappers
	existingCharts, err := loadExistingCharts(paths, vendor, chartName)
	if err != nil {
		return fmt.Errorf("failed to load existing charts: %w", err)
	}
	versionToChartWrapper := map[string]*ChartWrapper{}
	for _, chartWrapper := range chartWrappers {
		versionToChartWrapper[chartWrapper.Metadata.Version] = chartWrapper
	}
	for _, existingChart := range existingCharts {
		if _, ok := versionToChartWrapper[existingChart.Metadata.Version]; !ok {
			assetFilename := getTgzFilename(existingChart.Chart)
			assetPath := filepath.Join(assetsDir, assetFilename)
			if err := os.RemoveAll(assetPath); err != nil {
				return fmt.Errorf("failed to remove %q: %w", assetFilename, err)
			}
		}
	}

	// create or update existing charts
	for _, chartWrapper := range chartWrappers {
		assetsFilename := getTgzFilename(chartWrapper.Chart)
		assetsPath := filepath.Join(assetsDir, assetsFilename)
		tgzFileExists, err := utils.Exists(assetsPath)
		if err != nil {
			return fmt.Errorf("failed to check %s for existence: %w", assetsPath, err)
		}
		if chartWrapper.Modified || !tgzFileExists {
			_, err := chartutil.Save(chartWrapper.Chart, assetsDir)
			if err != nil {
				return fmt.Errorf("failed to write tgz for %q version %q: %w", chartWrapper.Name(), chartWrapper.Metadata.Version, err)
			}
		}

		chartsPath := filepath.Join(chartsDir, chartWrapper.Metadata.Version)
		chartsPathExists, err := utils.Exists(chartsPath)
		if err != nil {
			return fmt.Errorf("failed to check %s for existence: %w", chartsPath, err)
		}
		if chartWrapper.Modified || !chartsPathExists {
			if err := conform.Gunzip(assetsPath, chartsPath); err != nil {
				return fmt.Errorf("failed to unpack %q version %q to %q: %w", chartWrapper.Name(), chartWrapper.Metadata.Version, chartsPath, err)
			}
		}
	}

	return nil
}

// loadExistingCharts loads the existing charts for package
// <vendor>/<packageName> from the assets directory. It returns
// them in a slice that is sorted by chart version, newest first.
func loadExistingCharts(paths p.Paths, vendor string, packageName string) ([]*ChartWrapper, error) {
	existingChartPaths, err := getExistingChartTgzFiles(paths, vendor, packageName)
	if err != nil {
		return nil, fmt.Errorf("failed to get paths to existing chart tgz files: %w", err)
	}
	existingChartWrappers := make([]*ChartWrapper, 0, len(existingChartPaths))
	for _, existingChartPath := range existingChartPaths {
		existingChart, err := loader.LoadFile(existingChartPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load chart version %q: %w", existingChartPath, err)
		}
		existingChartWrapper := NewChartWrapper(existingChart)
		existingChartWrappers = append(existingChartWrappers, existingChartWrapper)
	}
	slices.SortFunc(existingChartWrappers, func(a, b *ChartWrapper) int {
		parsedA := semver.MustParse(a.Chart.Metadata.Version)
		parsedB := semver.MustParse(b.Chart.Metadata.Version)
		return parsedB.Compare(parsedA)
	})
	return existingChartWrappers, nil
}

// getExistingChartTgzFiles lists the .tgz files for package <vendor>/
// <packageName> from that package vendor's assets directory.
func getExistingChartTgzFiles(paths p.Paths, vendor string, packageName string) ([]string, error) {
	assetsPath := filepath.Join(paths.Assets, vendor)
	tgzFiles, err := os.ReadDir(assetsPath)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read dir %q: %w", assetsPath, err)
	}
	filePaths := make([]string, 0, len(tgzFiles))
	for _, tgzFile := range tgzFiles {
		if tgzFile.IsDir() {
			continue
		}
		matchName := filepath.Base(tgzFile.Name())
		if matched, err := filepath.Match(fmt.Sprintf("%s-*.tgz", packageName), matchName); err != nil {
			return nil, fmt.Errorf("failed to check match for %q: %w", matchName, err)
		} else if !matched {
			continue
		}
		existingChartPath := filepath.Join(assetsPath, tgzFile.Name())
		filePaths = append(filePaths, existingChartPath)
	}
	return filePaths, nil
}

// integrateCharts integrates new charts from upstream with any
// existing charts. It applies modifications to the new charts, and
// ensures that the state of all charts, both current and new, is
// correct. Should never modify an existing chart, except for in
// the special case of the "featured" annotation.
func integrateCharts(paths p.Paths, packageWrapper pkg.PackageWrapper, existingCharts, newCharts []*ChartWrapper) error {
	overlayFiles, err := packageWrapper.GetOverlayFiles()
	if err != nil {
		return fmt.Errorf("failed to get overlay files: %w", err)
	}

	for _, newChart := range newCharts {
		if err := applyOverlayFiles(overlayFiles, newChart.Chart); err != nil {
			return fmt.Errorf("failed to apply overlay files to chart %q version %q: %w", newChart.Name(), newChart.Metadata.Version, err)
		}
		conform.OverlayChartMetadata(newChart.Chart, packageWrapper.UpstreamYaml.ChartMetadata)
		if err := addAnnotations(packageWrapper, newChart.Chart); err != nil {
			return fmt.Errorf("failed to add annotations to chart %q version %q: %w", newChart.Name(), newChart.Metadata.Version, err)
		}
		if err := ensureIcon(paths, packageWrapper, newChart); err != nil {
			return fmt.Errorf("failed to ensure icon for chart %q version %q: %w", newChart.Name(), newChart.Metadata.Version, err)
		}
		newChart.Modified = true
	}

	if err := ensureFeaturedAnnotation(existingCharts, newCharts); err != nil {
		return fmt.Errorf("failed to ensure featured annotation: %w", err)
	}

	return nil
}

// applyOverlayFiles applies the files referenced in overlayFiles to the files
// in helmChart.Files. If a file already exists, it is overwritten.
func applyOverlayFiles(overlayFiles map[string][]byte, helmChart *chart.Chart) error {
	for relativePath, contents := range overlayFiles {
		newFile := &chart.File{
			Name: relativePath,
			Data: contents,
		}
		for _, file := range helmChart.Files {
			if file.Name == relativePath {
				file.Data = contents
				goto skip
			}
		}
		helmChart.Files = append(helmChart.Files, newFile)
	skip:
	}
	return nil
}

// Ensures that an icon for the chart has been downloaded to the local icons
// directory, and that the icon URL field for helmChart refers to this local
// icon file. We do this so that airgap installations of Rancher have access
// to icons without needing to download them from a remote source.
func ensureIcon(paths p.Paths, packageWrapper pkg.PackageWrapper, chartWrapper *ChartWrapper) error {
	localIconPath, err := icons.EnsureIconDownloaded(paths, chartWrapper.Metadata.Icon, packageWrapper.Name)
	if err != nil {
		return fmt.Errorf("failed to ensure icon downloaded: %w", err)
	}

	localIconUrl := "file://" + localIconPath
	if chartWrapper.Metadata.Icon != localIconUrl {
		chartWrapper.Metadata.Icon = localIconUrl
		chartWrapper.Modified = true
	}

	return nil
}

// Sets annotations on helmChart according to values from packageWrapper,
// and especially from packageWrapper.UpstreamYaml.
func addAnnotations(packageWrapper pkg.PackageWrapper, helmChart *chart.Chart) error {
	annotations := make(map[string]string)

	if autoInstall := packageWrapper.UpstreamYaml.AutoInstall; autoInstall != "" {
		annotations[annotationAutoInstall] = autoInstall
	}

	if packageWrapper.UpstreamYaml.Experimental {
		annotations[annotationExperimental] = "true"
	}

	if packageWrapper.UpstreamYaml.Hidden {
		annotations[annotationHidden] = "true"
	}

	annotations[annotationCertified] = "partner"

	annotations[annotationDisplayName] = packageWrapper.DisplayName

	if packageWrapper.UpstreamYaml.ReleaseName != "" {
		annotations[annotationReleaseName] = packageWrapper.UpstreamYaml.ReleaseName
	} else {
		annotations[annotationReleaseName] = packageWrapper.Name
	}

	if packageWrapper.UpstreamYaml.Namespace != "" {
		annotations[annotationNamespace] = packageWrapper.UpstreamYaml.Namespace
	}

	if packageWrapper.UpstreamYaml.ChartMetadata.KubeVersion != "" {
		annotations[annotationKubeVersion] = packageWrapper.UpstreamYaml.ChartMetadata.KubeVersion
	} else if helmChart.Metadata.KubeVersion != "" {
		annotations[annotationKubeVersion] = helmChart.Metadata.KubeVersion
	}

	if packageVersion := packageWrapper.UpstreamYaml.PackageVersion; packageVersion != 0 {
		generatedVersion, err := conform.GeneratePackageVersion(helmChart.Metadata.Version, &packageVersion)
		helmChart.Metadata.Version = generatedVersion
		if err != nil {
			return fmt.Errorf("failed to generate version: %w", err)
		}
	}

	conform.ApplyChartAnnotations(helmChart, annotations, false)

	return nil
}

// Ensures that "featured" annotation is set properly for the set of all passed
// charts. Is separate from setting other annotations because only the latest
// chart version for a given package must have the "featured" annotation, so
// this function must consider and possibly modify all of the package's chart
// versions.
func ensureFeaturedAnnotation(existingCharts, newCharts []*ChartWrapper) error {
	// get current value of featured annotation
	featuredAnnotationValue := ""
	for _, existingChart := range existingCharts {
		val, ok := existingChart.Metadata.Annotations[annotationFeatured]
		if !ok {
			continue
		}
		if featuredAnnotationValue != "" && featuredAnnotationValue != val {
			return fmt.Errorf("found two different values for featured annotation %q and %q", featuredAnnotationValue, val)
		}
		featuredAnnotationValue = val
	}
	if featuredAnnotationValue == "" {
		// the chart is not featured
		return nil
	}

	// set featured annotation on last of new charts
	// TODO: This replicates a bug in the existing code. Whichever ChartVersion
	// comes last in the ChartVersions that conformPackage is working on has
	// the featured annotation applies. This could easily give the wrong result, which
	// presumably is for only the latest chart version to have the "featured"
	// annotation.
	// But in practice this is not a problem: as of the time of writing, only
	// one chart (kasten/k10) uses a value for UpstreamYaml.Fetch other than the
	// default value of "latest", and that chart is not featured.
	lastNewChart := newCharts[len(newCharts)-1]
	if conform.AnnotateChart(lastNewChart.Chart, annotationFeatured, featuredAnnotationValue, true) {
		lastNewChart.Modified = true
	}

	// Ensure featured annotation is not present on existing charts. We don't
	// need to worry about other new charts because they will not have the
	// featured annotation.
	for _, existingChart := range existingCharts {
		if conform.DeannotateChart(existingChart.Chart, annotationFeatured, "") {
			existingChart.Modified = true
		}
	}

	return nil
}

// getByAnnotation gets all repo.ChartVersions from index.yaml that have
// the specified annotation with the specified value. If value is "",
// all repo.ChartVersions that have the specified annotation will be
// returned, regardless of that annotation's value.
func getByAnnotation(paths p.Paths, annotation, value string) map[string]repo.ChartVersions {
	indexYaml, err := repo.LoadIndexFile(paths.IndexYaml)
	if err != nil {
		logrus.Fatalf("failed to read index.yaml: %s", err)
	}
	matchedVersions := make(map[string]repo.ChartVersions)

	for chartName, entries := range indexYaml.Entries {
		for _, version := range entries {
			appendVersion := false
			if _, ok := version.Annotations[annotation]; ok {
				if value != "" {
					if version.Annotations[annotation] == value {
						appendVersion = true
					}
				} else {
					appendVersion = true
				}
			}
			if appendVersion {
				if _, ok := matchedVersions[chartName]; !ok {
					matchedVersions[chartName] = repo.ChartVersions{version}
				} else {
					matchedVersions[chartName] = append(matchedVersions[chartName], version)
				}
			}
		}
	}

	return matchedVersions
}

// writeIndex is the only way that index.yaml should ever be written.
// It looks at the set of charts in the assets directory and generates
// a new index.yaml file from their metadata. Some information from
// the old index.yaml file is used to avoid making unnecessary changes,
// but for the most part this function enforces the idea that the
// index.yaml file should treat the charts' Chart.yaml files as the
// authoritative source of chart metadata.
func writeIndex(paths p.Paths) error {
	newHelmIndexYaml, err := repo.IndexDirectory(paths.Assets, filepath.Base(paths.Assets))
	if err != nil {
		return fmt.Errorf("failed to index assets directory: %w", err)
	}

	oldHelmIndexYaml, err := repo.LoadIndexFile(paths.IndexYaml)
	if errors.Is(err, os.ErrNotExist) {
		if err := newHelmIndexYaml.WriteFile(paths.IndexYaml, 0o644); err != nil {
			return fmt.Errorf("failed to write index.yaml: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to load index.yaml: %w", err)
	}

	for chartName, newChartVersions := range newHelmIndexYaml.Entries {
		for _, newChartVersion := range newChartVersions {
			// Use the values of created field from old index.yaml to avoid making
			// unnecessary changes, since it is set to time.Now() in repo.LoadIndexFile.
			oldChartVersion, err := oldHelmIndexYaml.Get(chartName, newChartVersion.Version)
			if err == nil {
				newChartVersion.Created = oldChartVersion.Created
			}

			// Older charts cannot be changed, and may have remote (i.e. not
			// beginning with file://) icon URLs. So instead of changing the
			// icon URL in the Chart.yaml and allowing it to propagate automatically
			// to the index.yaml for these chart versions, we change it only in
			// the index.yaml. This works because Rancher uses the icon URL
			// value from index.yaml, not the chart itself, when loading a chart's
			// icon.
			iconPath, err := icons.GetDownloadedIconPath(paths, newChartVersion.Name)
			if err != nil {
				return fmt.Errorf("failed to get downloaded icon path for chart %q version %s: %s", newChartVersion.Name, newChartVersion.Version, err)
			} else {
				newChartVersion.Icon = "file://" + iconPath
			}
		}
	}

	// Both human-created PRs and the nightly update may modify the
	// `generated: ...` line of index.yaml. Because PRs often take more
	// than 24 hours to review, which means the nightly update runs in
	// the meantime, merge conflicts are very common and are a pain.
	// We solve this by only modifying the line when the nightly update
	// runs.
	if !modifyGenerated {
		newHelmIndexYaml.Generated = oldHelmIndexYaml.Generated
	}

	newHelmIndexYaml.SortEntries()

	if err := newHelmIndexYaml.WriteFile(paths.IndexYaml, 0o644); err != nil {
		return fmt.Errorf("failed to write index.yaml: %w", err)
	}

	return nil
}

// listPackages prints out the packages in the current repository.
func listPackages(c *cli.Context) error {
	currentPackage := os.Getenv(packageEnvVariable)
	paths, err := p.GetPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}
	packageWrappers, err := pkg.ListPackageWrappers(paths, currentPackage)
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}
	sortPackageWrappers(packageWrappers)
	for _, packageWrapper := range packageWrappers {
		fmt.Println(packageWrapper.FullName())
	}

	return nil
}

// addFeaturedChart adds the "featured" annotation to a chart.
func addFeaturedChart(c *cli.Context) error {
	if c.Args().Len() != 2 {
		logrus.Fatalf("Please provide the chart name and featured number (1 - %d) as arguments\n", featuredMax)
	}
	featuredChart := c.Args().Get(0)
	inputIndex := c.Args().Get(1)
	featuredNumber, err := strconv.Atoi(inputIndex)
	if err != nil {
		return fmt.Errorf("failed to parse given index %q: %w", inputIndex, err)
	}
	if featuredNumber < 1 || featuredNumber > featuredMax {
		return fmt.Errorf("featured number must be between %d and %d\n", 1, featuredMax)
	}

	paths, err := p.GetPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}
	packageList, err := pkg.ListPackageWrappers(paths, featuredChart)
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}
	packageWrapper := packageList[0]

	featuredVersions := getByAnnotation(paths, annotationFeatured, inputIndex)
	if len(featuredVersions) > 0 {
		for chartName := range featuredVersions {
			logrus.Errorf("%s already featured at index %d\n", chartName, featuredNumber)
		}
	} else {
		vendor := packageWrapper.Vendor
		chartName := packageWrapper.Name
		paths, err := p.GetPaths()
		if err != nil {
			return fmt.Errorf("failed to get paths: %w", err)
		}
		if err := annotate(paths, vendor, chartName, annotationFeatured, inputIndex, false, true); err != nil {
			return fmt.Errorf("failed to annotate %q: %w", packageWrapper.FullName(), err)
		}
		if err := writeIndex(paths); err != nil {
			return fmt.Errorf("failed to write index: %w", err)
		}
	}

	return nil
}

// removeFeaturedChart removes the "featured" annotation from a chart.
func removeFeaturedChart(c *cli.Context) error {
	if c.Args().Len() != 1 {
		logrus.Fatal("Please provide the chart name as argument")
	}
	featuredChart := c.Args().Get(0)
	paths, err := p.GetPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	packageList, err := pkg.ListPackageWrappers(paths, featuredChart)
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}
	packageWrapper := packageList[0]

	vendor := packageWrapper.Vendor
	chartName := packageWrapper.Name
	if err := annotate(paths, vendor, chartName, annotationFeatured, "", true, false); err != nil {
		return fmt.Errorf("failed to deannotate %q: %w", packageWrapper.FullName(), err)
	}

	if err := writeIndex(paths); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

func listFeaturedCharts(c *cli.Context) error {
	indexConflict := false
	featuredSorted := make([]string, featuredMax)
	paths, err := p.GetPaths()
	if err != nil {
		logrus.Fatalf("failed to get paths: %s", err)
	}
	featuredVersions := getByAnnotation(paths, annotationFeatured, "")

	for chartName, chartVersion := range featuredVersions {
		featuredIndex, err := strconv.Atoi(chartVersion[0].Annotations[annotationFeatured])
		if err != nil {
			logrus.Fatal(err)
		}
		featuredIndex--
		if featuredSorted[featuredIndex] != "" {
			indexConflict = true
			featuredSorted[featuredIndex] += fmt.Sprintf(", %s", chartName)
		} else {
			featuredSorted[featuredIndex] = chartName
		}
	}
	if indexConflict {
		logrus.Errorf("Multiple charts given same featured index")
	}

	for i, chartName := range featuredSorted {
		if featuredSorted[i] != "" {
			fmt.Printf("%d: %s\n", i+1, chartName)
		}
	}

	return nil
}

// hideChart ensures each released version of a package has the "hidden"
// annotation set to "true". This hides the package in the Rancher UI.
func hideChart(c *cli.Context) error {
	if c.Args().Len() != 1 {
		logrus.Fatal("Must provide exactly one package name as argument")
	}
	currentPackage := c.Args().Get(0)
	paths, err := p.GetPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	packageWrappers, err := pkg.ListPackageWrappers(paths, currentPackage)
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}
	packageWrapper := packageWrappers[0]

	// set Hidden: true in upstream.yaml
	packageWrapper.UpstreamYaml.Hidden = true
	upstreamYamlPath := filepath.Join(packageWrapper.Path, upstreamYamlFile)
	if err := upstreamyaml.Write(upstreamYamlPath, packageWrapper.UpstreamYaml); err != nil {
		return fmt.Errorf("failed to write upstream.yaml: %w", err)
	}

	vendor := packageWrapper.Vendor
	chartName := packageWrapper.Name
	if err := annotate(paths, vendor, chartName, annotationHidden, "true", false, false); err != nil {
		return fmt.Errorf("failed to annotate package: %w", err)
	}
	if err := writeIndex(paths); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

// CLI function call - Generates automated commit
func autoUpdate(c *cli.Context) error {
	currentPackage := os.Getenv(packageEnvVariable)
	paths, err := p.GetPaths()
	if err != nil {
		logrus.Fatalf("failed to get paths: %s", err)
	}
	packageWrappers, err := pkg.ListPackageWrappers(paths, currentPackage)
	if err != nil {
		logrus.Fatalf("failed to list packages: %s", err)
	}

	updatablePackageWrappers := make([]pkg.PackageWrapper, 0, len(packageWrappers))
	for _, packageWrapper := range packageWrappers {
		if packageWrapper.UpstreamYaml.Deprecated {
			logrus.Warnf("Package %s is deprecated; skipping update", packageWrapper.FullName())
			continue
		}

		updatable, err := packageWrapper.Populate(paths)
		if err != nil {
			logrus.Errorf("failed to populate %s: %s", packageWrapper.FullName(), err)
			continue
		}

		if len(packageWrapper.FetchVersions) == 0 {
			logrus.Infof("%s is up-to-date\n", packageWrapper.FullName())
		}
		for _, version := range packageWrapper.FetchVersions {
			logrus.Infof("\n  Package: %s\n  Source: %s\n  Version: %s\n  URL: %s\n",
				packageWrapper.FullName(), packageWrapper.SourceMetadata.Source, version.Version, version.URLs[0])
		}

		if updatable {
			updatablePackageWrappers = append(updatablePackageWrappers, packageWrapper)
		}
	}

	if len(updatablePackageWrappers) == 0 {
		return nil
	}

	updatedPackageWrappers := make([]pkg.PackageWrapper, 0, len(updatablePackageWrappers))
	skippedList := make([]string, 0, len(updatablePackageWrappers))
	for _, packageWrapper := range updatablePackageWrappers {
		if err := ApplyUpdates(paths, packageWrapper); err != nil {
			logrus.Errorf("failed to apply updates for chart %q: %s", packageWrapper.Name, err)
			skippedList = append(skippedList, packageWrapper.Name)
		} else {
			updatedPackageWrappers = append(updatedPackageWrappers, packageWrapper)
		}
	}
	if len(skippedList) >= len(updatablePackageWrappers) {
		logrus.Fatal("All packages skipped. Exiting...")
	}
	if len(skippedList) > 0 {
		logrus.Errorf("Skipped due to error: %v", skippedList)
	}

	if err := writeIndex(paths); err != nil {
		logrus.Fatalf("failed to write index.yaml: %s", err)
	}

	if makeCommit {
		if err := commitChanges(paths, updatedPackageWrappers); err != nil {
			logrus.Fatalf("failed to commit changes: %s", err)
		}
	}

	return nil
}

// CLI function call - Validates repo against released
func validateRepo(c *cli.Context) error {
	paths, err := p.GetPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}
	configYaml, err := validate.ReadConfig(paths.ConfigurationYaml)
	if err != nil {
		logrus.Fatalf("failed to read configuration.yaml: %s\n", err)
	}

	validationErrors := validate.Run(paths, configYaml)
	if len(validationErrors) > 0 {
		fmt.Println(errors.Join(validationErrors...))
		return errors.New("validation failed")
	}

	return nil
}

// cullCharts removes chart versions that are older than the passed number of
// days. Like many other subcommands, the PACKAGE environment variable can be
// used to work on a single package.
func cullCharts(c *cli.Context) error {
	currentPackage := os.Getenv(packageEnvVariable)
	paths, err := p.GetPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}
	packageWrappers, err := pkg.ListPackageWrappers(paths, currentPackage)
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}

	// parse days argument
	rawDays := c.Args().Get(0)
	daysInt64, err := strconv.ParseInt(rawDays, 10, strconv.IntSize)
	if err != nil {
		return fmt.Errorf("failed to convert %q to integer: %w", rawDays, err)
	}
	days := int(daysInt64)

	_, newerChartVersions, err := getOlderAndNewerChartVersions(paths, days)
	if err != nil {
		return fmt.Errorf("failed to get older and newer chart versions: %w", err)
	}

	skippedPackages := make([]string, 0, len(packageWrappers))
	for _, packageWrapper := range packageWrappers {
		logrus.Infof("culling %s", packageWrapper.FullName())
		existingCharts, err := loadExistingCharts(paths, packageWrapper.Vendor, packageWrapper.Name)
		if err != nil {
			logrus.Errorf("failed to load existing charts for %q: %s", packageWrapper.FullName(), err)
			skippedPackages = append(skippedPackages, packageWrapper.FullName())
			continue
		}

		keptCharts := make([]*ChartWrapper, 0, len(existingCharts))
		for _, existingChart := range existingCharts {
			if slices.Contains(newerChartVersions[packageWrapper.Name], existingChart.Metadata.Version) {
				keptCharts = append(keptCharts, existingChart)
			}
		}
		if len(keptCharts) == 0 {
			logrus.Errorf("no versions of %s would remain; skipping...",
				packageWrapper.FullName())
			skippedPackages = append(skippedPackages, packageWrapper.FullName())
			continue
		}

		if err := writeCharts(paths, packageWrapper.Vendor, packageWrapper.Name, keptCharts); err != nil {
			logrus.Errorf("failed to write charts for %q: %s", packageWrapper.FullName(), err)
			skippedPackages = append(skippedPackages, packageWrapper.FullName())
			continue
		}
	}

	if len(skippedPackages) > 0 {
		logrus.Errorf("skipped due to error:\n%s", strings.Join(skippedPackages, "\n"))
	}
	if len(skippedPackages) == len(packageWrappers) {
		logrus.Fatal("all packages skipped")
	}

	if err := writeIndex(paths); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

// getOlderAndNewerChartVersions splits the chartVersions in
// index.yaml into two groups: one that is more than days days old,
// and one that is less than days days old. They are returned as maps
// of chart name to slices of versions, one version per chartVersion.
// The older versions are the first return value and the newer
// versions are the second return value.
func getOlderAndNewerChartVersions(paths p.Paths, days int) (map[string][]string, map[string][]string, error) {
	indexYaml, err := repo.LoadIndexFile(paths.IndexYaml)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read index file: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	olderVersions := make(map[string][]string)
	newerVersions := make(map[string][]string)
	for chartName, chartVersions := range indexYaml.Entries {
		for _, chartVersion := range chartVersions {
			if chartVersion.Created.After(cutoff) {
				newerVersions[chartName] = append(newerVersions[chartName], chartVersion.Version)
			} else {
				olderVersions[chartName] = append(olderVersions[chartName], chartVersion.Version)
			}
		}
	}

	return olderVersions, newerVersions, nil
}

func removePackage(c *cli.Context) error {
	if c.Args().Len() != 1 {
		return errors.New("must provide package name as argument")
	}
	currentPackage := c.Args().Get(0)
	paths, err := p.GetPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	packageWrappers, err := pkg.ListPackageWrappers(paths, currentPackage)
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}
	packageWrapper := packageWrappers[0]

	if !force && !packageWrapper.UpstreamYaml.Deprecated {
		return fmt.Errorf("%s is not deprecated; use --force to force removal", packageWrapper.FullName())
	}

	removalPaths := []string{
		filepath.Join(paths.Packages, packageWrapper.Vendor, packageWrapper.Name),
		filepath.Join(paths.Charts, packageWrapper.Vendor, packageWrapper.Name),
	}

	assetFiles, err := getExistingChartTgzFiles(paths, packageWrapper.Vendor, packageWrapper.Name)
	if err != nil {
		return fmt.Errorf("failed to list asset files for %s: %w", packageWrapper.FullName(), err)
	}
	removalPaths = append(removalPaths, assetFiles...)

	localIconPath, err := icons.GetDownloadedIconPath(paths, packageWrapper.Name)
	if err != nil {
		logrus.Warnf("failed to get icon path for %s: %s", packageWrapper.FullName(), err)
	} else {
		removalPaths = append(removalPaths, localIconPath)
	}

	for _, removalPath := range removalPaths {
		if err := os.RemoveAll(removalPath); err != nil {
			logrus.Errorf("failed to remove %q: %s", removalPath, err)
		}
	}

	if err := writeIndex(paths); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

func deprecatePackage(c *cli.Context) error {
	if c.Args().Len() != 1 {
		return errors.New("must provide package name as argument")
	}
	currentPackage := c.Args().Get(0)
	paths, err := p.GetPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	packageWrappers, err := pkg.ListPackageWrappers(paths, currentPackage)
	if err != nil {
		return fmt.Errorf("failed to list package wrappers: %w", err)
	}
	packageWrapper := packageWrappers[0]

	// set Deprecated: true in upstream.yaml
	packageWrapper.UpstreamYaml.Deprecated = true
	upstreamYamlPath := filepath.Join(packageWrapper.Path, upstreamYamlFile)
	if err := upstreamyaml.Write(upstreamYamlPath, packageWrapper.UpstreamYaml); err != nil {
		return fmt.Errorf("failed to write upstream.yaml: %w", err)
	}

	// set deprecated: true in each chart version's Chart.yaml
	chartWrappers, err := loadExistingCharts(paths, packageWrapper.Vendor, packageWrapper.Name)
	if err != nil {
		return fmt.Errorf("failed to load existing charts: %w", err)
	}
	for _, chartWrapper := range chartWrappers {
		if !chartWrapper.Metadata.Deprecated {
			chartWrapper.Metadata.Deprecated = true
			chartWrapper.Modified = true
		}
	}
	if err := writeCharts(paths, packageWrapper.Vendor, packageWrapper.Name, chartWrappers); err != nil {
		return fmt.Errorf("failed to write charts: %w", err)
	}

	if err := writeIndex(paths); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

func main() {
	if len(os.Getenv("DEBUG")) > 0 {
		logrus.SetLevel(logrus.DebugLevel)
	}

	app := cli.NewApp()
	app.Name = "partner-charts-ci"
	app.Version = fmt.Sprintf("%s (%s)", version, commit)
	app.Usage = "A tool for working with the Rancher Partner Charts helm chart repository"

	app.Commands = []*cli.Command{
		{
			Name:   "list",
			Usage:  "List packages",
			Action: listPackages,
		},
		{
			Name:   "update",
			Usage:  "Download and integrate new chart versions from upstreams",
			Action: autoUpdate,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:        "commit",
					Aliases:     []string{"c"},
					Usage:       "Commit any changes",
					Destination: &makeCommit,
				},
				&cli.BoolFlag{
					Name:        "modify-generated",
					Aliases:     []string{"m"},
					Usage:       `Update the "generated" line of index.yaml`,
					Destination: &modifyGenerated,
				},
			},
		},
		{
			Name:   "hide",
			Usage:  "Apply 'catalog.cattle.io/hidden' annotation to all stored versions of chart",
			Action: hideChart,
		},
		{
			Name:  "feature",
			Usage: "Commands related to the 'catalog.cattle.io/featured' annotation",
			Subcommands: []*cli.Command{
				{
					Name:   "list",
					Usage:  "List currently featured charts",
					Action: listFeaturedCharts,
				},
				{
					Name:   "add",
					Usage:  "Add 'catalog.cattle.io/featured' annotation to chart",
					Action: addFeaturedChart,
				},
				{
					Name:   "remove",
					Usage:  "Remove 'catalog.cattle.io/featured' annotation from chart",
					Action: removeFeaturedChart,
				},
			},
		},
		{
			Name:   "validate",
			Usage:  "Run validations on the repository",
			Action: validateRepo,
		},
		{
			Name:      "cull",
			Usage:     "Remove chart versions older than a number of days",
			Action:    cullCharts,
			ArgsUsage: "<days>",
		},
		{
			Name:      "remove",
			Usage:     "Remove a package and all of its associated chart versions",
			Action:    removePackage,
			ArgsUsage: "<package>",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:        "force",
					Aliases:     []string{"f"},
					Usage:       "Skip check for package deprecation",
					Destination: &force,
				},
			},
		},
		{
			Name:      "deprecate",
			Usage:     "Deprecate a package and all of its associated chart versions",
			Action:    deprecatePackage,
			ArgsUsage: "<package>",
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("error: %s", err)
		os.Exit(1)
	}
}
