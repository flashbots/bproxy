package jrpc

import (
	"encoding/json"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type CallWithIdAsString struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func (call CallWithIdAsString) DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error) {
	if call.Method != "eth_sendRawTransaction" {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: invalid method: %s",
			errFailedToDecodeEthSendRawTransaction, call.Method,
		)
	}

	return decodeEthSendRawTransaction(call.Params)
}

func (call CallWithIdAsString) GetID() string {
	return `"` + call.ID + `"`
}

func (call CallWithIdAsString) GetMethod() string {
	return call.Method
}

func (call CallWithIdAsString) GetParams() json.RawMessage {
	return call.Params
}
