package types

import (
	"encoding/json"
	"errors"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type JrpcCall struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	ID      uint64          `json:"id"`
	Params  json.RawMessage `json:"params,omitempty"`
}

var (
	errFailedToDecodeEthSendRawTransaction = errors.New("failed to decode eth_sendRawTransaction")
)

func (call JrpcCall) DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error) {
	if call.Method != "eth_sendRawTransaction" {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: invalid method: %s",
			errFailedToDecodeEthSendRawTransaction, call.Method,
		)
	}

	var inputs []string
	if err := json.Unmarshal(call.Params, &inputs); err != nil {
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
