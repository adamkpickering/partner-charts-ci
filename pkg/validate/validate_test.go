package validate

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain(t *testing.T) {

	t.Run("matchHelmCharts", func(t *testing.T) {
		testCases := []struct {
			Description   string
			UpdateChart   string
			ExpectedMatch bool
		}{
			{
				Description:   "should report a modification if tgz files differ",
				UpdateChart:   "testchart-modified.tgz",
				ExpectedMatch: false,
			},
			{
				Description:   "should not report a modification if tgz files do not differ",
				UpdateChart:   "testchart-base.tgz",
				ExpectedMatch: true,
			},
			{
				Description:   "should not report a modification if tgz files only differ in catalog.cattle.io-prefixed annotations",
				UpdateChart:   "testchart-annotation-added.tgz",
				ExpectedMatch: true,
			},
			{
				Description:   "should not report a modification if tgz files only differ in deprecated field of Chart.yaml",
				UpdateChart:   "testchart-deprecated-set.tgz",
				ExpectedMatch: true,
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.Description, func(t *testing.T) {
				upstreamPath, err := filepath.Abs(filepath.Join("testdata", "testchart-base.tgz"))
				if err != nil {
					t.Fatalf("failed to get absolute path to upstream tgz: %s", err)
				}
				updatePath, err := filepath.Abs(filepath.Join("testdata", testCase.UpdateChart))
				if err != nil {
					t.Fatalf("failed to get absolute path to update tgz: %s", err)
				}
				match, err := matchHelmCharts(upstreamPath, updatePath)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				assert.Equal(t, testCase.ExpectedMatch, match)
			})
		}
	})

	t.Run("CompareDirectories", func(t *testing.T) {

		t.Run("should report a modification if directories differ", func(t *testing.T) {
			upstreamPath, err := filepath.Abs(filepath.Join("testdata", "modification-directories-differ", "upstream"))
			if err != nil {
				t.Fatalf("failed to get absolute path to upstream testing directory: %s", err)
			}
			updatePath, err := filepath.Abs(filepath.Join("testdata", "modification-directories-differ", "update"))
			if err != nil {
				t.Fatalf("failed to get absolute path to update testing directory: %s", err)
			}
			directoryComparison, err := compareDirectories(upstreamPath, updatePath)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			assert.Equal(t, []string{filepath.Join(updatePath, "testfile")}, directoryComparison.Modified)
			assert.Len(t, directoryComparison.Added, 0)
			assert.Len(t, directoryComparison.Removed, 0)
		})

		t.Run("should not report anything if directories are the same", func(t *testing.T) {
			upstreamPath, err := filepath.Abs(filepath.Join("testdata", "modification-directories-same", "upstream"))
			if err != nil {
				t.Fatalf("failed to get absolute path to upstream testing directory: %s", err)
			}
			updatePath, err := filepath.Abs(filepath.Join("testdata", "modification-directories-same", "update"))
			if err != nil {
				t.Fatalf("failed to get absolute path to update testing directory: %s", err)
			}
			directoryComparison, err := compareDirectories(upstreamPath, updatePath)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			assert.Len(t, directoryComparison.Modified, 0)
			assert.Len(t, directoryComparison.Added, 0)
			assert.Len(t, directoryComparison.Removed, 0)
		})

		t.Run("should report an addition if a file has been added", func(t *testing.T) {
			upstreamPath, err := filepath.Abs(filepath.Join("testdata", "addition-new-file", "upstream"))
			if err != nil {
				t.Fatalf("failed to get absolute path to upstream testing directory: %s", err)
			}
			updatePath, err := filepath.Abs(filepath.Join("testdata", "addition-new-file", "update"))
			if err != nil {
				t.Fatalf("failed to get absolute path to update testing directory: %s", err)
			}
			directoryComparison, err := compareDirectories(upstreamPath, updatePath)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			assert.Len(t, directoryComparison.Modified, 0)
			assert.Equal(t, []string{filepath.Join(updatePath, "testfile")}, directoryComparison.Added)
			assert.Len(t, directoryComparison.Removed, 0)
		})

		t.Run("should report a removal if a file has been removed", func(t *testing.T) {
			upstreamPath, err := filepath.Abs(filepath.Join("testdata", "removal-removed-file", "upstream"))
			if err != nil {
				t.Fatalf("failed to get absolute path to upstream testing directory: %s", err)
			}
			updatePath, err := filepath.Abs(filepath.Join("testdata", "removal-removed-file", "update"))
			if err != nil {
				t.Fatalf("failed to get absolute path to update testing directory: %s", err)
			}
			directoryComparison, err := compareDirectories(upstreamPath, updatePath)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			assert.Len(t, directoryComparison.Modified, 0)
			assert.Len(t, directoryComparison.Added, 0)
			assert.Equal(t, []string{filepath.Join(updatePath, "testfile")}, directoryComparison.Removed)
		})
	})
}
