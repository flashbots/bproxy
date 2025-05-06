package triaged

import "time"

type Request struct {
	Proxy      bool
	Prioritise bool
	Mirror     bool

	Deadline time.Time

	JrpcMethod   string
	JrpcID       string
	Transactions RequestTransactions
}
