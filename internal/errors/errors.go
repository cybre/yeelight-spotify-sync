package errors

import (
	"fmt"

	"github.com/go-errors/errors"
)

func New(err interface{}) error {
	if err == nil {
		return nil
	}

	return errors.New(err)
}

func Errorf(format string, a ...interface{}) error {
	return errors.Errorf(format, a...)
}

func Wrap(err interface{}) error {
	if err == nil {
		return nil
	}

	return errors.Wrap(err, 1)
}

func Wrapf(err interface{}, format string, a ...interface{}) error {
	if err == nil {
		return nil
	}

	return errors.WrapPrefix(err, fmt.Sprintf(format, a...), 1)
}

func Is(err, target error) bool {
	return errors.Is(err, target)
}

func As(err error, target interface{}) bool {
	return errors.As(err, target)
}
