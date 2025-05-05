package triaged

type Request struct {
	Proxy  bool
	Mirror bool

	JrpcMethod   string
	JrpcID       string
	Transactions RequestTransactions
}
