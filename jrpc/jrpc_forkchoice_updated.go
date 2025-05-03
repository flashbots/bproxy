package jrpc

type ForkchoiceUpdatedV3 struct {
	Params []*struct{} `json:"params"`
}

type ForkchoiceUpdatedV3WithoutExtraParam struct {
	Params []*struct {
		HeadBlockHash      string `json:"headBlockHash"`
		SafeBlockHash      string `json:"safeBlockHash"`
		FinalizedBlockHash string `json:"finalizedBlockHash"`
	} `json:"params"`
}

func (fcuv3 ForkchoiceUpdatedV3) HasExtraParam() bool {
	return len(fcuv3.Params) == 2 && fcuv3.Params[0] != nil && fcuv3.Params[1] != nil
}

func (fcuv3 ForkchoiceUpdatedV3WithoutExtraParam) BlockHashes() (head, safe, finalized string) {
	for _, p := range fcuv3.Params {
		if p != nil {
			return p.HeadBlockHash, p.SafeBlockHash, p.FinalizedBlockHash
		}
	}
	return "", "", ""
}
