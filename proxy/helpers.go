package proxy

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"unsafe"

	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/flashbots/bproxy/types"
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

func decodeTxHash(req []byte) (string, error) {
	var jsonRequest types.EthSendRawTransaction
	if err := json.Unmarshal(req, &jsonRequest); err != nil {
		return "", err
	}

	// this is a bit ugly, but it works
	var inputs []interface{}
	if err := json.Unmarshal(jsonRequest.Params, &inputs); err != nil {
		return "", err
	}
	if len(inputs) != 1 {
		return "", fmt.Errorf("expected 1 input, got %d", len(inputs))
	}

	input, ok := inputs[0].(string)
	if !ok {
		return "", fmt.Errorf("expected string input, got %T", inputs[0])
	}
	inputBytes, err := hexutil.Decode(input)
	if err != nil {
		return "", err
	}

	tx := new(ethtypes.Transaction)
	if err := tx.UnmarshalBinary(inputBytes); err != nil {
		return "", err
	}
	return tx.Hash().Hex(), nil
}
