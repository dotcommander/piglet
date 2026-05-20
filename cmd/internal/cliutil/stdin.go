package cliutil

import (
	"errors"
	"fmt"
	"io"
)

const DefaultMaxStdinBytes int64 = 16 << 20

var ErrInputTooLarge = errors.New("stdin input too large")

// ReadAllLimit reads r up to max bytes and returns ErrInputTooLarge if there is
// additional data beyond the limit.
func ReadAllLimit(r io.Reader, max int64) ([]byte, error) {
	if max < 0 {
		return nil, fmt.Errorf("negative stdin limit: %d", max)
	}
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%w: max %d bytes", ErrInputTooLarge, max)
	}
	return data, nil
}
