package jrpc

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type ForkchoiceUpdatedV3 struct {
	Params []json.RawMessage `json:"params"`
}

type ForkchoiceUpdatedV3Param0 struct {
	HeadBlockHash      string `json:"headBlockHash"`
	SafeBlockHash      string `json:"safeBlockHash"`
	FinalizedBlockHash string `json:"finalizedBlockHash"`
}

func (p ForkchoiceUpdatedV3Param0) GetHashes() (head, safe, finalized string) {
	return p.HeadBlockHash, p.SafeBlockHash, p.FinalizedBlockHash
}

type ForkchoiceUpdatedV3Param1 struct {
	Timestamp string `json:"timestamp"`
}

func (p ForkchoiceUpdatedV3Param1) GetTimestamp() (time.Time, error) {
	epoch, err := strconv.ParseInt(
		strings.TrimPrefix(p.Timestamp, "0x"),
		16, 64,
	)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(epoch, 0), nil
}
