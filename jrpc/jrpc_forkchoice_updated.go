package jrpc

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/flashbots/bproxy/utils"
)

type ForkchoiceUpdatedV3 struct {
	Params []json.RawMessage `json:"params"`
}

func (fcuv3 ForkchoiceUpdatedV3) ParamsCount() int {
	res := 0
	for _, param := range fcuv3.Params {
		if utils.Str(param) == "null" {
			continue
		}
		res++
	}
	return res
}

type ForkchoiceUpdatedV3_ForkchoiceState struct {
	HeadBlockHash      string `json:"headBlockHash"`
	SafeBlockHash      string `json:"safeBlockHash"`
	FinalizedBlockHash string `json:"finalizedBlockHash"`
}

func (p ForkchoiceUpdatedV3_ForkchoiceState) GetHashes() (head, safe, finalized string) {
	return p.HeadBlockHash, p.SafeBlockHash, p.FinalizedBlockHash
}

type ForkchoiceUpdatedV3_PayloadAttributes struct {
	Timestamp string `json:"timestamp"`
}

func (p ForkchoiceUpdatedV3_PayloadAttributes) GetTimestamp() (time.Time, error) {
	epoch, err := strconv.ParseInt(
		strings.TrimPrefix(p.Timestamp, "0x"),
		16, 64,
	)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(epoch, 0), nil
}
