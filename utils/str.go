package utils

import "unsafe"

func Str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
