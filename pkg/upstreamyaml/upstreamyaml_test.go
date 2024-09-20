package upstreamyaml

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMain(t *testing.T) {
	t.Run("UpstreamYaml", func(t *testing.T) {
		t.Run("SetDefaults", func(t *testing.T) {
			t.Run("should set Fetch field if not set", func(t *testing.T) {
				upstreamYaml := &UpstreamYaml{}
				upstreamYaml.SetDefaults()
				assert.Equal(t, "latest", upstreamYaml.Fetch)
			})

			t.Run("should not change Fetch field if set", func(t *testing.T) {
				fetchValue := "newer"
				upstreamYaml := &UpstreamYaml{
					Fetch: fetchValue,
				}
				upstreamYaml.SetDefaults()
				assert.Equal(t, fetchValue, upstreamYaml.Fetch)
			})

			t.Run("should set ReleaseName field if not set", func(t *testing.T) {
				helmChartValue := "helm-chart-value"
				upstreamYaml := &UpstreamYaml{
					HelmChart: helmChartValue,
				}
				upstreamYaml.SetDefaults()
				assert.Equal(t, helmChartValue, upstreamYaml.ReleaseName)
			})

			t.Run("should not change ReleaseName field if set", func(t *testing.T) {
				releaseNameValue := "release-name-value"
				upstreamYaml := &UpstreamYaml{
					HelmChart:   "helm-chart-value",
					ReleaseName: releaseNameValue,
				}
				upstreamYaml.SetDefaults()
				assert.Equal(t, releaseNameValue, upstreamYaml.ReleaseName)
			})
		})
	})
}
