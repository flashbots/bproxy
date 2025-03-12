package types_test

import (
	"os"
	"testing"

	"github.com/flashbots/bproxy/types"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
)

func TestUnmarshal(t *testing.T) {
	{
		b, err := os.ReadFile("./testdata/engine_forkchoiceUpdatedV3.json")
		assert.NoError(t, err)

		call := types.EngineForkchoiceUpdatedV3{}
		err = json.Unmarshal(b, &call)
		assert.NoError(t, err)

		assert.Equal(t, "engine_forkchoiceUpdatedV3", call.Method)
		assert.Equal(t, "2.0", call.Version)

		assert.Equal(t, 2, len(call.Params))
		assert.NotNil(t, call.Params[0])
		assert.Nil(t, call.Params[1])
	}

	{
		b, err := os.ReadFile("./testdata/engine_forkchoiceUpdatedV3_payload.json")
		assert.NoError(t, err)

		call := types.EngineForkchoiceUpdatedV3{}
		err = json.Unmarshal(b, &call)
		assert.NoError(t, err)

		assert.Equal(t, "engine_forkchoiceUpdatedV3", call.Method)
		assert.Equal(t, "2.0", call.Version)

		assert.Equal(t, 2, len(call.Params))
		if len(call.Params) == 2 {
			assert.NotNil(t, call.Params[0])
			assert.NotNil(t, call.Params[1])
		}
	}

	{
		b, err := os.ReadFile("./testdata/eth_sendRawTransaction.json")
		assert.NoError(t, err)

		call := types.EthSendRawTransaction{}
		err = json.Unmarshal(b, &call)
		assert.NoError(t, err)

		assert.Equal(t, "eth_sendRawTransaction", call.Method)
		assert.Equal(t, "2.0", call.Version)
	}
}
