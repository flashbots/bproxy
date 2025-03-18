package utils

import "errors"

func FlattenErrors(errs []error) error {
	switch len(errs) {
	default:
		return errors.Join(errs...)
	case 1:
		return errs[0]
	case 0:
		return nil
	}
}
