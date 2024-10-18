package validate

// A ValidationFunc is a function that checks one specific thing about the
// partner charts repository. If anything is wrong, it may return one or
// more errors explaining what is wrong.
type ValidationFunc func(configYaml ConfigurationYaml) []error

func Run(configYaml ConfigurationYaml) []error {
	validationErrors := []error{}
	validationFuncs := []ValidationFunc{
		preventReleasedChartModifications,
		preventDuplicatePackageNames,
	}
	for _, validationFunc := range validationFuncs {
		errors := validationFunc(configYaml)
		validationErrors = append(validationErrors, errors...)
	}
	return validationErrors
}
