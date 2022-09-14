package test

import (
	"sync"

	"google.golang.org/grpc/test/bufconn"
)

var (
	Lis  *bufconn.Listener = nil
	Once sync.Once
)
