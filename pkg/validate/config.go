package validate

import (
	"errors"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"sigs.k8s.io/yaml"
)

type ConfigurationYaml struct {
	ValidateUpstreams []validateUpstream `json:"validate"`
}

type validateUpstream struct {
	Url    string
	Branch string
}

func (configYaml ConfigurationYaml) Validate() error {
	if len(configYaml.ValidateUpstreams) == 0 {
		return errors.New("must provide validation configuration")
	}
	if configYaml.ValidateUpstreams[0].Branch == "" {
		return errors.New("must provide branch in validation configuration")
	}
	if configYaml.ValidateUpstreams[0].Url == "" {
		return errors.New("must provide URL in validation configuration")
	}
	return nil
}

func ReadConfig(configYamlPath string) (ConfigurationYaml, error) {
	upstreamYamlFile, err := os.ReadFile(configYamlPath)
	configYaml := ConfigurationYaml{}
	if err != nil {
		logrus.Debug(err)
	} else {
		err = yaml.Unmarshal(upstreamYamlFile, &configYaml)
	}
	if err := configYaml.Validate(); err != nil {
		return ConfigurationYaml{}, fmt.Errorf("invalid config: %w", err)
	}
	return configYaml, err
}
