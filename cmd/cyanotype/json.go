package main

import (
	"fmt"
	"io"

	"github.com/cyanotype-dev/cyanotype/internal/contract"
)

// jsonFlagHelp is the help of --json, shared by every command that has it.
const jsonFlagHelp = "print the result as JSON on stdout and nothing else; never asks"

// printJSON writes o to w as the JSON contract, then ends the command with
// code: nil for codeOK, or an exitError whose outcome is already printed.
func printJSON(w io.Writer, o contract.Output, code int) error {
	data, err := contract.Marshal(o)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write the JSON output: %w", err)
	}
	if code == codeOK {
		return nil
	}
	return &exitError{code: code}
}

// usageDiagnostics holds a usage error found before the command ran, such
// as two flags that cannot be combined, for an output that ends with exit
// code 2.
func usageDiagnostics(err error) contract.Diagnostics {
	return contract.Diagnostics{Errors: []contract.Diagnostic{contract.Error(contract.CodeUsage, err.Error())}}
}
