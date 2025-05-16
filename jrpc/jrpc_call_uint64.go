package jrpc

import (
	"encoding/json"
	"fmt"
	"strconv"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type callUint64 struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func (call callUint64) DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error) {
	if call.Method != "eth_sendRawTransaction" {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: invalid method: %s",
			errFailedToDecodeEthSendRawTransaction, call.Method,
		)
	}

	return decodeEthSendRawTransaction(call.Params)
}

func (call callUint64) GetID() string {
	return strconv.FormatUint(call.ID, 10)
}

func (call callUint64) GetMethod() string {
	return call.Method
}

func (call callUint64) GetParams() json.RawMessage {
	return call.Params
}

func (call callUint64) SetMethod(method string) {
	call.Method = method
}
