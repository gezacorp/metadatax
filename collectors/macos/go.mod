module github.com/gezacorp/metadatax/collectors/macos

go 1.26.0

require (
	emperror.dev/errors v0.8.1
	github.com/gezacorp/metadatax v0.0.0-20250619152456-c2ae8300820c
	github.com/stretchr/testify v1.8.4
	golang.org/x/sys v0.48.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.4.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/gezacorp/metadatax => ../../
