package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
}
