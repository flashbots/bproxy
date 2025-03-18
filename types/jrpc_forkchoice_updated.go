package types

type EngineForkchoiceUpdatedV3 struct {
	Params []*struct{} `json:"params"`
}

type EngineForkchoiceUpdatedV3_WithoutExtraPayload struct {
	Params []*struct {
		HeadBlockHash      string `json:"headBlockHash"`
		SafeBlockHash      string `json:"safeBlockHash"`
		FinalizedBlockHash string `json:"finalizedBlockHash"`
	} `json:"params"`
}

func (fcuv3 EngineForkchoiceUpdatedV3) HasExtraPayload() bool {
	return len(fcuv3.Params) == 2 && fcuv3.Params[0] != nil && fcuv3.Params[1] != nil
}

func (fcuv3 EngineForkchoiceUpdatedV3_WithoutExtraPayload) BlockHashes() (head, safe, finalized string) {
	for _, p := range fcuv3.Params {
		if p != nil {
			return p.HeadBlockHash, p.SafeBlockHash, p.FinalizedBlockHash
		}
	}
	return "", "", ""
}
