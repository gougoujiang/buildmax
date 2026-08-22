//go:build windows

package tool

import "errors"

func makeFIFO(string) error { return errors.New("fifos are not available on Windows") }
