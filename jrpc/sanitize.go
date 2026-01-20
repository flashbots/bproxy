package jrpc

// Sanitize sanitizes a JSON-RPC message by replacing raw transaction data
// with transaction hashes. It handles both single messages and batch requests.
// The input should be an unmarshalled JSON value.
func Sanitize(message any) {
	switch msg := message.(type) {
	case []any:
		// Batch request
		for _, item := range msg {
			Sanitize(item)
		}
	case map[string]any:
		sanitizeMessage(msg)
	}
}

func sanitizeMessage(message map[string]any) {
	method, _ := message["method"].(string)

	if method != "" {
		// Request: sanitize params based on method
		params, ok := message["params"].([]any)
		if !ok {
			return
		}

		switch method {
		case "engine_forkchoiceUpdatedV3":
			if len(params) < 2 {
				return
			}
			executionPayload, ok := params[1].(map[string]any)
			if !ok {
				return
			}
			sanitizeTransactions(executionPayload, "transactions")

		case "engine_newPayloadV4":
			if len(params) == 0 {
				return
			}
			executionPayload, ok := params[0].(map[string]any)
			if !ok {
				return
			}
			sanitizeTransactions(executionPayload, "transactions")

		case "eth_sendBundle":
			if len(params) == 0 {
				return
			}
			bundleParams, ok := params[0].(map[string]any)
			if !ok {
				return
			}
			sanitizeTransactions(bundleParams, "txs")

		case "eth_sendRawTransaction":
			for i := range params {
				rawTransactionToHash(&params[i])
			}
		}
		return
	}

	// Response: check for result.executionPayload.transactions (engine_getPayloadV4)
	result, ok := message["result"].(map[string]any)
	if !ok {
		return
	}
	executionPayload, ok := result["executionPayload"].(map[string]any)
	if !ok {
		return
	}
	sanitizeTransactions(executionPayload, "transactions")
}

func sanitizeTransactions(obj map[string]any, key string) {
	transactions, ok := obj[key].([]any)
	if !ok {
		return
	}
	for i := range transactions {
		rawTransactionToHash(&transactions[i])
	}
}

func rawTransactionToHash(transaction *any) {
	str, ok := (*transaction).(string)
	if !ok {
		*transaction = "[error casting tx to string]"
		return
	}

	_, tx, err := DecodeEthRawTransaction(str)
	if err != nil {
		*transaction = "[error decoding tx: " + err.Error() + "]"
		return
	}

	*transaction = tx.Hash().Hex()
}
