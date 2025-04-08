package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type JrpcCall interface {
	DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error)
	GetID() string
	GetMethod() string
}

type JrpcCall_Uint64 struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type JrpcCall_String struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

var (
	errFailedToDecodeEthSendRawTransaction = errors.New("failed to decode eth_sendRawTransaction")
)

// uint64

func (call JrpcCall_Uint64) DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error) {
	if call.Method != "eth_sendRawTransaction" {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: invalid method: %s",
			errFailedToDecodeEthSendRawTransaction, call.Method,
		)
	}

	return decodeEthSendRawTransaction(call.Params)
}

func (call JrpcCall_Uint64) GetID() string {
	return strconv.FormatUint(call.ID, 10)
}

func (call JrpcCall_Uint64) GetMethod() string {
	return call.Method
}

// string

func (call JrpcCall_String) DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error) {
	if call.Method != "eth_sendRawTransaction" {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: invalid method: %s",
			errFailedToDecodeEthSendRawTransaction, call.Method,
		)
	}

	return decodeEthSendRawTransaction(call.Params)
}

func (call JrpcCall_String) GetID() string {
	return `"` + call.ID + `"`
}
func (call JrpcCall_String) GetMethod() string {
	return call.Method
}

// common

func decodeEthSendRawTransaction(params json.RawMessage) (ethcommon.Address, *ethtypes.Transaction, error) {
	var inputs []string
	if err := json.Unmarshal(params, &inputs); err != nil {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: %w",
			errFailedToDecodeEthSendRawTransaction, err,
		)
	}
	if len(inputs) != 1 {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: expected 1 parameter, got %d",
			errFailedToDecodeEthSendRawTransaction, len(inputs))
	}

	bytes, err := hexutil.Decode(inputs[0])
	if err != nil {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: %w",
			errFailedToDecodeEthSendRawTransaction, err,
		)
	}

	tx := new(ethtypes.Transaction)
	if err := tx.UnmarshalBinary(bytes); err != nil {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: %w",
			errFailedToDecodeEthSendRawTransaction, err,
		)
	}

	from, err := ethtypes.Sender(ethtypes.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: %w",
			errFailedToDecodeEthSendRawTransaction, err,
		)
	}

	return from, tx, nil

}
