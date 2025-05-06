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
}

func Unmarshal(body []byte) (Call, error) {
	call_uint64 := CallWithIdAsUint64{}
	err_uint64 := json.Unmarshal(body, &call_uint64)
	if err_uint64 == nil {
		return call_uint64, nil
	}

	call_string := CallWithIdAsString{}
	err_string := json.Unmarshal(body, &call_string)
	if err_string == nil {
		return call_string, nil
	}

	return nil, errors.Join(err_uint64, err_string)
}
