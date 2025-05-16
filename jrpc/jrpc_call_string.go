package jrpc

import (
	"encoding/json"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type callString struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func (call callString) DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error) {
	if call.Method != "eth_sendRawTransaction" {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: invalid method: %s",
			errFailedToDecodeEthSendRawTransaction, call.Method,
		)
	}

	return decodeEthSendRawTransaction(call.Params)
}

func (call callString) GetID() string {
	return `"` + call.ID + `"`
}

func (call callString) GetMethod() string {
	return call.Method
}

func (call callString) GetParams() json.RawMessage {
	return call.Params
}
