package validate

type ValidationFunc func(configYaml ConfigurationYaml) []error

func Run(configYaml ConfigurationYaml) []error {
	validationErrors := []error{}
	validationFuncs := []ValidationFunc{
		PreventReleasedChartModifications,
	}
	for _, validationFunc := range validationFuncs {
		errors := validationFunc(configYaml)
		validationErrors = append(validationErrors, errors...)
	}
	return validationErrors
}
