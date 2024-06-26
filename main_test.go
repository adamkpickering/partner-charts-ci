package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"helm.sh/helm/v3/pkg/chart"
)

func TestMain(t *testing.T) {
	t.Run("PackageWrapper", func(t *testing.T) {
		t.Run("GetOverlayFiles", func(t *testing.T) {
			t.Run("should parse overlay files properly", func(t *testing.T) {
				packageWrapper := PackageWrapper{
					Path: filepath.Join("testdata", "getOverlayFiles"),
				}
				actualOverlayFiles, err := packageWrapper.GetOverlayFiles()
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				expectedOverlayFiles := map[string][]byte{
					"file1.txt":                        []byte("this is file 1\n"),
					"file2.txt":                        []byte("this is file 2\n"),
					filepath.Join("dir1", "file3.txt"): []byte("this is file 3\n"),
				}
				for expectedPath, expectedValue := range expectedOverlayFiles {
					actualValue, ok := actualOverlayFiles[expectedPath]
					assert.True(t, ok)
					assert.Equal(t, expectedValue, actualValue)
				}
				assert.Equal(t, len(expectedOverlayFiles), len(actualOverlayFiles))
			})
		})
	})

	t.Run("applyOverlayFiles", func(t *testing.T) {
		t.Run("should add files that do not already exist", func(t *testing.T) {
			filename := "file1.txt"
			overlayFiles := map[string][]byte{
				filename: []byte("this is file 1"),
			}
			helmChart := &chart.Chart{}
			if err := applyOverlayFiles(overlayFiles, helmChart); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			found := false
			for _, file := range helmChart.Files {
				if file.Name == filename {
					found = true
					assert.Equal(t, overlayFiles[filename], file.Data)
				}
			}
			assert.True(t, found)
		})

		t.Run("should overwrite existing files", func(t *testing.T) {
			filename := "file1.txt"
			filedata := []byte("this is file 1")
			overlayFiles := map[string][]byte{
				filename: filedata,
			}
			helmChart := &chart.Chart{
				Files: []*chart.File{
					{
						Name: filename,
						Data: []byte("these are different contents"),
					},
				},
			}
			if err := applyOverlayFiles(overlayFiles, helmChart); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			found := false
			for _, file := range helmChart.Files {
				if file.Name == filename {
					found = true
					assert.Equal(t, overlayFiles[filename], file.Data)
				}
			}
			assert.True(t, found)
			assert.Equal(t, 1, len(helmChart.Files))
		})
	})
}
