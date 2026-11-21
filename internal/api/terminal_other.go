//go:build !linux

package api

import "errors"

func startPTY(string, int, string) (ptySession, error) {
	return nil, errors.New("the terminal requires Linux")
}
