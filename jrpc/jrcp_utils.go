package jrpc

import (
	"encoding/json"
	"errors"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

var (
	errFailedToDecodeEthSendRawTransaction = errors.New("failed to decode eth_sendRawTransaction")
)

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
