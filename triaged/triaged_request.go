package triaged

type Request struct {
	Proxy      bool
	Prioritise bool
	Mirror     bool

	JrpcMethod   string
	JrpcID       string
	Transactions RequestTransactions
}
