package main

import (
	"path/filepath"
	"testing"

	"github.com/rancher/partner-charts-ci/pkg/parse"
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

	t.Run("addAnnotations", func(t *testing.T) {
		t.Run("should set auto-install annotation properly", func(t *testing.T) {
			for _, autoInstall := range []string{"", "some-chart"} {
				packageWrapper := PackageWrapper{
					UpstreamYaml: &parse.UpstreamYaml{
						AutoInstall: autoInstall,
					},
				}
				helmChart := &chart.Chart{
					Metadata: &chart.Metadata{
						Dependencies: []*chart.Dependency{},
					},
				}
				if err := addAnnotations(packageWrapper, helmChart); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				value, ok := helmChart.Metadata.Annotations[annotationAutoInstall]
				assert.Equal(t, autoInstall != "", ok)
				if autoInstall != "" {
					assert.Equal(t, autoInstall, value)
				}
			}
		})

		t.Run("should set experimental annotation properly", func(t *testing.T) {
			for _, experimental := range []bool{false, true} {
				packageWrapper := PackageWrapper{
					UpstreamYaml: &parse.UpstreamYaml{
						Experimental: experimental,
					},
				}
				helmChart := &chart.Chart{
					Metadata: &chart.Metadata{
						Dependencies: []*chart.Dependency{},
					},
				}
				if err := addAnnotations(packageWrapper, helmChart); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				value, ok := helmChart.Metadata.Annotations[annotationExperimental]
				assert.Equal(t, experimental, ok)
				if experimental {
					assert.Equal(t, "true", value)
				}
			}
		})

		t.Run("should set hidden annotation properly", func(t *testing.T) {
			for _, hidden := range []bool{false, true} {
				packageWrapper := PackageWrapper{
					UpstreamYaml: &parse.UpstreamYaml{
						Hidden: hidden,
					},
				}
				helmChart := &chart.Chart{
					Metadata: &chart.Metadata{
						Dependencies: []*chart.Dependency{},
					},
				}
				if err := addAnnotations(packageWrapper, helmChart); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				value, ok := helmChart.Metadata.Annotations[annotationHidden]
				assert.Equal(t, hidden, ok)
				if hidden {
					assert.Equal(t, "true", value)
				}
			}
		})

		t.Run("should always set certified annotation", func(t *testing.T) {
			packageWrapper := PackageWrapper{
				UpstreamYaml: &parse.UpstreamYaml{},
			}
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Dependencies: []*chart.Dependency{},
				},
			}
			if err := addAnnotations(packageWrapper, helmChart); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			value, ok := helmChart.Metadata.Annotations[annotationCertified]
			assert.True(t, ok)
			assert.Equal(t, "partner", value)
		})

		t.Run("should always set display-name annotation", func(t *testing.T) {
			displayName := "Display Name"
			packageWrapper := PackageWrapper{
				DisplayName:  displayName,
				UpstreamYaml: &parse.UpstreamYaml{},
			}
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Dependencies: []*chart.Dependency{},
				},
			}
			if err := addAnnotations(packageWrapper, helmChart); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			value, ok := helmChart.Metadata.Annotations[annotationDisplayName]
			assert.True(t, ok)
			assert.Equal(t, displayName, value)
		})

		t.Run("should set release-name annotation properly", func(t *testing.T) {
			testCases := []struct {
				PackageWrapperName       string
				UpstreamYamlName         string
				ShouldBeUpstreamYamlName bool
			}{
				{
					PackageWrapperName:       "packageWrapperName",
					UpstreamYamlName:         "upstreamYamlName",
					ShouldBeUpstreamYamlName: true,
				},
				{
					PackageWrapperName:       "packageWrapperName",
					UpstreamYamlName:         "",
					ShouldBeUpstreamYamlName: false,
				},
			}
			for _, testCase := range testCases {
				packageWrapper := PackageWrapper{
					Name: testCase.PackageWrapperName,
					UpstreamYaml: &parse.UpstreamYaml{
						ReleaseName: testCase.UpstreamYamlName,
					},
				}
				helmChart := &chart.Chart{
					Metadata: &chart.Metadata{
						Dependencies: []*chart.Dependency{},
					},
				}
				if err := addAnnotations(packageWrapper, helmChart); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				value, ok := helmChart.Metadata.Annotations[annotationReleaseName]
				assert.True(t, ok)
				if testCase.ShouldBeUpstreamYamlName {
					assert.Equal(t, testCase.UpstreamYamlName, value)
				} else {
					assert.Equal(t, testCase.PackageWrapperName, value)
				}
			}
		})

		t.Run("should set namespace annotation properly", func(t *testing.T) {
			for _, namespace := range []string{"", "test-namespace"} {
				packageWrapper := PackageWrapper{
					UpstreamYaml: &parse.UpstreamYaml{
						Namespace: namespace,
					},
				}
				helmChart := &chart.Chart{
					Metadata: &chart.Metadata{
						Dependencies: []*chart.Dependency{},
					},
				}
				if err := addAnnotations(packageWrapper, helmChart); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				value, ok := helmChart.Metadata.Annotations[annotationNamespace]
				assert.Equal(t, namespace != "", ok)
				if namespace != "" {
					assert.Equal(t, namespace, value)
				}
			}
		})

		t.Run("should set kube-version annotation properly", func(t *testing.T) {
			testCases := []struct {
				CurrentKubeVersion      string
				UpstreamYamlKubeVersion string
				ExpectedValue           string
			}{
				{
					CurrentKubeVersion:      "currentKubeVersion",
					UpstreamYamlKubeVersion: "upstreamYamlKubeVersion",
					ExpectedValue:           "upstreamYamlKubeVersion",
				},
				{
					CurrentKubeVersion:      "",
					UpstreamYamlKubeVersion: "upstreamYamlKubeVersion",
					ExpectedValue:           "upstreamYamlKubeVersion",
				},
				{
					CurrentKubeVersion:      "currentKubeVersion",
					UpstreamYamlKubeVersion: "",
					ExpectedValue:           "currentKubeVersion",
				},
				{
					CurrentKubeVersion:      "",
					UpstreamYamlKubeVersion: "",
					ExpectedValue:           "",
				},
			}
			for _, testCase := range testCases {
				packageWrapper := PackageWrapper{
					UpstreamYaml: &parse.UpstreamYaml{
						ChartYaml: chart.Metadata{
							KubeVersion: testCase.UpstreamYamlKubeVersion,
						},
					},
				}
				helmChart := &chart.Chart{
					Metadata: &chart.Metadata{
						KubeVersion:  testCase.CurrentKubeVersion,
						Dependencies: []*chart.Dependency{},
					},
				}
				if err := addAnnotations(packageWrapper, helmChart); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				value, ok := helmChart.Metadata.Annotations[annotationKubeVersion]
				if testCase.ExpectedValue == "" {
					assert.False(t, ok)
					continue
				}
				assert.True(t, ok)
				assert.Equal(t, testCase.ExpectedValue, value)
			}
		})
	})

	t.Run("ensureFeaturedAnnotation", func(t *testing.T) {
		t.Run("should return nil, nil when featured annotation not present on existing charts", func(t *testing.T) {
			existingCharts := []*chart.Chart{
				{
					Metadata: &chart.Metadata{
						Name: "existingChart1",
					},
				},
				{
					Metadata: &chart.Metadata{
						Name: "existingChart2",
					},
				},
			}
			newCharts := []*chart.Chart{
				{
					Metadata: &chart.Metadata{
						Name: "newChart1",
					},
				},
			}
			modifiedCharts, err := ensureFeaturedAnnotation(existingCharts, newCharts)
			assert.Equal(t, []*chart.Chart{}, modifiedCharts)
			assert.Equal(t, nil, err)
		})

		t.Run("should return error when there are two featured annotations of differing values in existing charts", func(t *testing.T) {
			existingCharts := []*chart.Chart{
				{
					Metadata: &chart.Metadata{
						Name: "existingChart1",
						Annotations: map[string]string{
							annotationFeatured: "1",
						},
					},
				},
				{
					Metadata: &chart.Metadata{
						Name: "existingChart2",
						Annotations: map[string]string{
							annotationFeatured: "2",
						},
					},
				},
			}
			newCharts := []*chart.Chart{
				{
					Metadata: &chart.Metadata{
						Name: "newChart1",
					},
				},
			}
			_, err := ensureFeaturedAnnotation(existingCharts, newCharts)
			assert.NotNil(t, err)
			assert.Contains(t, err.Error(), "found two different values for featured annotation")
		})

		t.Run("should modify charts correctly and return modified charts", func(t *testing.T) {
			featuredAnnotationValue := "2"
			existingCharts := []*chart.Chart{
				{
					Metadata: &chart.Metadata{
						Name: "existingChart1",
					},
				},
				{
					Metadata: &chart.Metadata{
						Name: "existingChart2",
						Annotations: map[string]string{
							annotationFeatured: featuredAnnotationValue,
						},
					},
				},
			}
			newCharts := []*chart.Chart{
				{
					Metadata: &chart.Metadata{
						Name: "newChart1",
					},
				},
				{
					Metadata: &chart.Metadata{
						Name: "newChart2",
					},
				},
			}
			modifiedCharts, err := ensureFeaturedAnnotation(existingCharts, newCharts)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			for _, existingChart := range existingCharts {
				_, ok := existingChart.Metadata.Annotations[annotationFeatured]
				assert.False(t, ok)
			}
			for _, newChart := range newCharts[0 : len(newCharts)-1] {
				_, ok := newChart.Metadata.Annotations[annotationFeatured]
				assert.False(t, ok)
			}
			val, ok := newCharts[len(newCharts)-1].Metadata.Annotations[annotationFeatured]
			assert.True(t, ok)
			assert.Equal(t, featuredAnnotationValue, val)
			expectedModifiedCharts := []*chart.Chart{
				newCharts[1],
				existingCharts[1],
			}
			assert.Equal(t, expectedModifiedCharts, modifiedCharts)
		})
	})
}
