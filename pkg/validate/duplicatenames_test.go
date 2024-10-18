package validate

import (
	"testing"

	"github.com/rancher/partner-charts-ci/pkg/pkg"
	"github.com/stretchr/testify/assert"
)

func TestPreventDuplicatePackageNames(t *testing.T) {
	t.Run("should return no errors when no duplicates are present", func(t *testing.T) {
		packageWrappers := pkg.PackageList{
			{
				Name:   "package1",
				Vendor: "vendor1",
			},
			{
				Name:   "package2",
				Vendor: "vendor1",
			},
		}
		errors := findDuplicateNames(packageWrappers)
		assert.Len(t, errors, 0)
	})

	t.Run("should return correct number of errors when duplicates are present", func(t *testing.T) {
		packageWrappers := pkg.PackageList{
			{
				Name:   "package1",
				Vendor: "vendor1",
			},
			{
				Name:   "package2",
				Vendor: "vendor1",
			},
			{
				Name:   "package1",
				Vendor: "vendor3",
			},
			{
				Name:   "package1",
				Vendor: "vendor4",
			},
		}
		errors := findDuplicateNames(packageWrappers)
		assert.Len(t, errors, 2)
		assert.ErrorContains(t, errors[0], "duplicate package names vendor1/package1 and vendor3/package1")
		assert.ErrorContains(t, errors[1], "duplicate package names vendor1/package1 and vendor4/package1")
	})
}
