package validate

import (
	"fmt"

	"github.com/rancher/partner-charts-ci/pkg/pkg"
)

// Packages (i.e. packages/vendor/name) are namespaced by vendor, but the charts
// that are eventually served from the partner charts repository are not. The
// charts' names come from the package names. So, we need to ensure that users
// cannot create a package with a name that another package already has.
func preventDuplicatePackageNames(_ ConfigurationYaml) []error {
	packageWrappers, err := pkg.ListPackageWrappers("")
	if err != nil {
		return []error{fmt.Errorf("failed to list package wrappers: %w", err)}
	}
	return findDuplicateNames(packageWrappers)
}

func findDuplicateNames(packageWrappers pkg.PackageList) []error {
	errors := make([]error, 0, len(packageWrappers))
	packageNames := make(map[string]string)
	for _, packageWrapper := range packageWrappers {
		if existingName, ok := packageNames[packageWrapper.Name]; !ok {
			packageNames[packageWrapper.Name] = packageWrapper.FullName()
		} else {
			error := fmt.Errorf("duplicate package names %s and %s", existingName, packageWrapper.FullName())
			errors = append(errors, error)
		}
	}
	return errors
}
