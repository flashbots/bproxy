package jrpc

import (
	"encoding/json"
	"fmt"
	"strconv"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type CallWithIdAsUint64 struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func (call CallWithIdAsUint64) DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error) {
	if call.Method != "eth_sendRawTransaction" {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: invalid method: %s",
			errFailedToDecodeEthSendRawTransaction, call.Method,
		)
	}

	return decodeEthSendRawTransaction(call.Params)
}

func (call CallWithIdAsUint64) GetID() string {
	return strconv.FormatUint(call.ID, 10)
}

func (call CallWithIdAsUint64) GetMethod() string {
	return call.Method
}

func (call CallWithIdAsUint64) GetParams() json.RawMessage {
	return call.Params
}
