package jrpc

import (
	"encoding/json"
	"errors"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type Call interface {
	DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error)
	GetID() string
	GetMethod() string
	GetParams() json.RawMessage
	SetMethod(string)
}

func Unmarshal(body []byte) (Call, error) {
	call_uint64 := callUint64{}
	err_uint64 := json.Unmarshal(body, &call_uint64)
	if err_uint64 == nil {
		return call_uint64, nil
	}

	call_string := callString{}
	err_string := json.Unmarshal(body, &call_string)
	if err_string == nil {
		return call_string, nil
	}

	return nil, errors.Join(err_uint64, err_string)
}

func UnmarshalBatch(body []byte) ([]Call, error) {
	batch_uint64 := []callUint64{}
	err_uint64 := json.Unmarshal(body, &batch_uint64)
	if err_uint64 == nil {
		batch := make([]Call, 0, len(batch_uint64))
		for _, call := range batch_uint64 {
			batch = append(batch, call)
		}
		return batch, nil
	}

	batch_string := []callString{}
	err_string := json.Unmarshal(body, &batch_string)
	if err_string == nil {
		batch := make([]Call, 0, len(batch_uint64))
		for _, call := range batch_string {
			batch = append(batch, call)
		}
	}

	return nil, errors.Join(err_uint64, err_string)
}
