package proxy

import (
	"compress/gzip"
	"io"
	"unsafe"
)

func str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func unzip(src io.Reader) ([]byte, error) {
	if src == nil {
		return nil, nil
	}

	reader, err := gzip.NewReader(src)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}
