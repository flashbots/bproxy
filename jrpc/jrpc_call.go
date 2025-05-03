package jrpc

import (
	"encoding/json"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type Call interface {
	DecodeEthSendRawTransaction() (ethcommon.Address, *ethtypes.Transaction, error)
	GetID() string
	GetMethod() string
	GetParams() json.RawMessage
}
