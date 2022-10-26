all:
	protoc --go_out=./service ./proto/*.proto
	go run genrpc/gen.go
