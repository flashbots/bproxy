package proxy

import (
	"unsafe"
)

func str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
