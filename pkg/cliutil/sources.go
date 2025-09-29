package cliutil

import (
	"fmt"

	"github.com/urfave/cli/v3"
)

type defaultValueSource struct {
	value string
}

func DefaultValue(value string) cli.ValueSource {
	return &defaultValueSource{value: value}
}

// GoString implements cli.ValueSource.
func (d *defaultValueSource) GoString() string {
	return fmt.Sprintf("&defaultValueSource{value:%[1]q}", d.value)
}

// Lookup implements cli.ValueSource.
func (d *defaultValueSource) Lookup() (string, bool) {
	return d.value, true
}

// String implements cli.ValueSource.
func (d *defaultValueSource) String() string {
	return d.value
}

var _ cli.ValueSource = (*defaultValueSource)(nil)
