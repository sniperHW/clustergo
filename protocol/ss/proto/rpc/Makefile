all:
	protoc --go_out=. *.proto
	rm -rf ../../rpc/*.go
	mv *.go ../../rpc
