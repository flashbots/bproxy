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

func decodeEthSendRawTransaction(params json.RawMessage) (
	addr ethcommon.Address, tx *ethtypes.Transaction, err error,
) {
	defer func() {
		// ethtypes.LatestSignerForChainID panics on invalid chain ID => it can
		// be that other underlying libraries have this bad habit as well
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: panic: %v",
				errFailedToDecodeEthSendRawTransaction, r,
			)
		}
	}()

	var inputs []string
	if err := json.Unmarshal(params, &inputs); err != nil {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: %w",
			errFailedToDecodeEthSendRawTransaction, err,
		)
	}
	if len(inputs) != 1 {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: expected 1 parameter, got %d",
			errFailedToDecodeEthSendRawTransaction, len(inputs),
		)
	}

	bytes, err := hexutil.Decode(inputs[0])
	if err != nil {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: %w",
			errFailedToDecodeEthSendRawTransaction, err,
		)
	}

	tx = new(ethtypes.Transaction)
	if err := tx.UnmarshalBinary(bytes); err != nil {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: %w",
			errFailedToDecodeEthSendRawTransaction, err,
		)
	}

	if tx.ChainId() == nil || tx.ChainId().Sign() <= 0 {
		return ethcommon.Address{}, nil, fmt.Errorf("%w: invalid chain id: %v",
			errFailedToDecodeEthSendRawTransaction, tx.ChainId(),
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
