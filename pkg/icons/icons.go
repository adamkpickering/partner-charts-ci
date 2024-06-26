package icons

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	partnerFilePath     = "index.yaml"
	partnerDownloadPath = "assets/icons"
)

// possible extensions for the icons
var extensions []string = []string{".png", ".jpg", ".jpeg", ".svg", ".ico"}

// PackageIconOverride is a struct to hold the package name, path, and icon to override
type PackageIconOverride struct {
	Name string
	Path string
	Icon string
}

// PackageIconList is a list of PackageIconOverride
type PackageIconList []PackageIconOverride

// PackageIconMap is a map of PackageIconOverride
type PackageIconMap map[string]PackageIconOverride

// ParsePackageToPackageIconOverride will convert a Package from a PackageList to a PackageIconOverride for a PackageIconList, this will be used to override the icon in the index.yaml
func ParsePackageToPackageIconOverride(name, path, icon string) PackageIconOverride {
	return PackageIconOverride{
		Name: name,
		Path: path,
		Icon: icon,
	}
}

// CheckFilesStructure will check if index.yaml and assets/icons exist
// will log and exit if the file structure does not exist
func CheckFilesStructure() {
	exists := Exists(partnerFilePath)
	if !exists {
		fmt.Printf("File not found: %s\n", partnerFilePath)
		logrus.Fatalf("File not found: %s\n", partnerFilePath)
	}
	_, err := os.Stat(partnerDownloadPath)
	if os.IsNotExist(err) {
		logrus.Fatalf("Directory not found: %s\n", partnerDownloadPath)
	}
}

// GetDownloadedIconPath checks if the icon is already downloaded and return the path
func GetDownloadedIconPath(packageName string) (string, error) {

	for _, ext := range extensions {
		filePath := fmt.Sprintf("assets/icons/%s%s", packageName, ext)
		if exist := Exists(filePath); exist {
			return fmt.Sprintf("file://%s", filePath), nil
		}
	}

	return "", fmt.Errorf("no icon found for package %q", packageName)
}

// OverrideIconValues will change the metade icon URL to a local icon path for the index.yaml
func OverrideIconValues(helmIndexYaml *repo.IndexFile, packageIconList PackageIconList) {
	for _, pkg := range packageIconList {
		for _, pk := range helmIndexYaml.Entries[pkg.Name] {
			pk.Metadata.Icon = pkg.Icon
		}
	}
}

// ValidateIconsAndIndexYaml will if Icons and IndexYaml have matching paths
func ValidateIconsAndIndexYaml(packageIconList PackageIconList, helmIndexFile *repo.IndexFile) error {
	var err error
	var scanErrors []error

	for _, pkg := range packageIconList {
		// Check if the icon has the correct path prefix
		if !strings.HasPrefix(pkg.Icon, "file://") {
			continue // Skip if the icon is not a local file
		}
		iconPath := strings.TrimPrefix(pkg.Icon, "file://")
		// Check if icon exists in the path
		exist := Exists(iconPath)
		if !exist {
			logrus.Errorf("Icon not found; icon:%s ", pkg.Icon)
			err = fmt.Errorf("icon not found: %s", iconPath)
			scanErrors = append(scanErrors, err)
		}

		// Check if the icon path is the same as the index.yaml
		iconEntry := helmIndexFile.Entries[pkg.Name][0].Metadata.Icon
		ok := iconEntry == pkg.Icon
		if !ok {
			logrus.Errorf("icon differ from index.yaml entry; icon: %s ; entry: %s", pkg.Icon, iconEntry)
			err = fmt.Errorf("icon differ from index.yaml entry; icon: %s ; entry: %s", pkg.Icon, iconEntry)
			scanErrors = append(scanErrors, err)
		}
	}

	if len(scanErrors) > 0 {
		logrus.Fatalf("Errors found in ValidateIconsAndIndexYaml")
	}

	// Count downloaded icons and icons entries in index yaml
	downloadedIconFiles, err := countDownloadedIconFiles()
	presentIconEntries := countIconEntriesInIndex(helmIndexFile)
	if err != nil {
		logrus.Errorf("Error reading assets/icons: %v", err)
		return err
	}
	if downloadedIconFiles != presentIconEntries {
		logrus.Errorf("Icon files in assets/icons: %d, Icon files in index.yaml: %d", downloadedIconFiles, presentIconEntries)
		return fmt.Errorf("icon files in assets/icons and index.yaml do not match")
	}
	return err
}

func countDownloadedIconFiles() (int, error) {
	files, err := os.ReadDir("assets/icons")
	if err != nil {
		return 0, err
	}

	count := 0
	for _, file := range files {
		if !file.IsDir() {
			count++
		}
	}

	return count, nil
}

func countIconEntriesInIndex(helmIndexFile *repo.IndexFile) int {
	var counter int
	for _, entry := range helmIndexFile.Entries {
		icon := entry[0].Metadata.Icon
		if strings.HasPrefix(icon, "file://") {
			counter++
		} else {
			logrus.Warnf("Icon %s of entry %s is not a local file", icon, entry[0].Name)
		}
	}
	return counter
}
