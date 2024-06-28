package conform

import (
	"fmt"
	"reflect"
	"testing"

	"helm.sh/helm/v3/pkg/chart"

	"github.com/stretchr/testify/assert"
)

const testAnnotation = "test-annotation"

func TestMain(t *testing.T) {
	t.Run("AnnotateChart", func(t *testing.T) {
		t.Run("should not modify an existing annotation when override is false", func(t *testing.T) {
			firstValue := "testvalue"
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Annotations: map[string]string{
						testAnnotation: firstValue,
					},
				},
			}
			result := AnnotateChart(helmChart, testAnnotation, "newtestvalue", false)
			assert.False(t, result)
			assert.Equal(t, firstValue, helmChart.Metadata.Annotations[testAnnotation])
		})

		t.Run("should modify an existing annotation when override is true", func(t *testing.T) {
			firstValue := "testvalue"
			secondValue := "newtestvalue"
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Annotations: map[string]string{
						testAnnotation: firstValue,
					},
				},
			}
			result := AnnotateChart(helmChart, testAnnotation, secondValue, true)
			assert.True(t, result)
			assert.Equal(t, secondValue, helmChart.Metadata.Annotations[testAnnotation])
		})

		t.Run("should set an annotation that is not already set", func(t *testing.T) {
			testValue := "testvalue"
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Annotations: map[string]string{},
				},
			}
			result := AnnotateChart(helmChart, testAnnotation, testValue, true)
			assert.True(t, result)
			assert.Equal(t, testValue, helmChart.Metadata.Annotations[testAnnotation])
		})
	})

	t.Run("DeannotateChart", func(t *testing.T) {
		t.Run("should not change Chart.Metadata.Annotation if it is nil", func(t *testing.T) {
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{},
			}
			result := DeannotateChart(helmChart, testAnnotation, "")
			assert.False(t, result)
			assert.Nil(t, helmChart.Metadata.Annotations)
		})

		t.Run("should remove the annotation when an empty value is passed", func(t *testing.T) {
			testValue := "testvalue"
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Annotations: map[string]string{
						testAnnotation: testValue,
					},
				},
			}
			result := DeannotateChart(helmChart, testAnnotation, "")
			assert.True(t, result)
			_, ok := helmChart.Metadata.Annotations[testAnnotation]
			assert.False(t, ok)
		})

		t.Run("should remove the annotation when the exact value is passed", func(t *testing.T) {
			testValue := "testvalue"
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Annotations: map[string]string{
						testAnnotation: testValue,
					},
				},
			}
			result := DeannotateChart(helmChart, testAnnotation, testValue)
			assert.True(t, result)
			_, ok := helmChart.Metadata.Annotations[testAnnotation]
			assert.False(t, ok)
		})

		t.Run("should not remove the annotation when a non-empty non-matching value is passed", func(t *testing.T) {
			testValue := "testvalue"
			passedValue := "passedvalue"
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Annotations: map[string]string{
						testAnnotation: testValue,
					},
				},
			}
			result := DeannotateChart(helmChart, testAnnotation, passedValue)
			assert.False(t, result)
			val, ok := helmChart.Metadata.Annotations[testAnnotation]
			assert.True(t, ok)
			assert.Equal(t, testValue, val)
		})
	})

	t.Run("OverlayChartMetadata", func(t *testing.T) {
		stringFieldNames := []string{"Name", "Home", "Version", "Description", "Icon", "APIVersion", "Condition", "Tags", "AppVersion", "KubeVersion", "Type"}
		for _, stringFieldName := range stringFieldNames {
			t.Run(fmt.Sprintf("should modify %s field properly", stringFieldName), func(t *testing.T) {
				expectedNewValue := "new" + stringFieldName
				metadata := &chart.Metadata{}
				reflect.ValueOf(metadata).Elem().FieldByName(stringFieldName).SetString("old" + stringFieldName)
				helmChart := &chart.Chart{
					Metadata: metadata,
				}
				overlay := chart.Metadata{}
				reflect.ValueOf(&overlay).Elem().FieldByName(stringFieldName).SetString(expectedNewValue)
				OverlayChartMetadata(helmChart, overlay)
				actualNewValue := reflect.ValueOf(helmChart.Metadata).Elem().FieldByName(stringFieldName).String()
				assert.Equal(t, expectedNewValue, actualNewValue)
			})
		}

		t.Run("should modify Deprecated field properly", func(t *testing.T) {
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Deprecated: false,
				},
			}
			overlay := chart.Metadata{
				Deprecated: true,
			}
			OverlayChartMetadata(helmChart, overlay)
			assert.True(t, helmChart.Metadata.Deprecated)
		})

		t.Run("should modify Sources field properly", func(t *testing.T) {
			newSources := []string{"value2", "value3"}
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Sources: []string{"value1"},
				},
			}
			overlay := chart.Metadata{
				Sources: newSources,
			}
			OverlayChartMetadata(helmChart, overlay)
			expectedSources := []string{"value1", "value2", "value3"}
			assert.Equal(t, expectedSources, helmChart.Metadata.Sources)
		})

		t.Run("should modify Keywords field properly", func(t *testing.T) {
			newKeywords := []string{"value2", "value3"}
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Keywords: []string{"value1"},
				},
			}
			overlay := chart.Metadata{
				Keywords: newKeywords,
			}
			OverlayChartMetadata(helmChart, overlay)
			expectedKeywords := []string{"value1", "value2", "value3"}
			assert.Equal(t, expectedKeywords, helmChart.Metadata.Keywords)
		})

		t.Run("should modify Maintainers field properly", func(t *testing.T) {
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Maintainers: []*chart.Maintainer{
						{
							Name: "maintainer1",
						},
					},
				},
			}
			overlay := chart.Metadata{
				Maintainers: []*chart.Maintainer{
					{
						Name: "maintainer2",
					},
					{
						Name: "maintainer3",
					},
				},
			}
			expectedMaintainers := make([]*chart.Maintainer, 0, len(helmChart.Metadata.Maintainers)+len(overlay.Maintainers))
			expectedMaintainers = append(expectedMaintainers, helmChart.Metadata.Maintainers...)
			expectedMaintainers = append(expectedMaintainers, overlay.Maintainers...)
			OverlayChartMetadata(helmChart, overlay)
			assert.Equal(t, expectedMaintainers, helmChart.Metadata.Maintainers)
		})

		t.Run("should modify Dependencies field properly", func(t *testing.T) {
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Dependencies: []*chart.Dependency{
						{
							Name:       "dependency1",
							Version:    "1.2.3",
							Repository: "https://test.repository.com",
						},
					},
				},
			}
			overlay := chart.Metadata{
				Dependencies: []*chart.Dependency{
					{
						Name:       "dependency2",
						Version:    "1.2.3",
						Repository: "https://test.repository.com",
					},
					{
						Name:       "dependency3",
						Version:    "1.2.3",
						Repository: "https://test.repository.com",
					},
				},
			}
			expectedDependencies := make([]*chart.Dependency, 0, len(helmChart.Metadata.Dependencies)+len(overlay.Dependencies))
			expectedDependencies = append(expectedDependencies, helmChart.Metadata.Dependencies...)
			expectedDependencies = append(expectedDependencies, overlay.Dependencies...)
			OverlayChartMetadata(helmChart, overlay)
			assert.Equal(t, expectedDependencies, helmChart.Metadata.Dependencies)
		})

		t.Run("should modify Annotations field properly", func(t *testing.T) {
			helmChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Annotations: map[string]string{
						"annotation1": "oldValue1",
						"annotation2": "oldValue2",
					},
				},
			}
			overlay := chart.Metadata{
				Annotations: map[string]string{
					"annotation1": "newValue1",
				},
			}
			OverlayChartMetadata(helmChart, overlay)
			assert.Equal(t, "newValue1", helmChart.Metadata.Annotations["annotation1"])
			assert.Equal(t, "oldValue2", helmChart.Metadata.Annotations["annotation2"])
		})
	})
}
