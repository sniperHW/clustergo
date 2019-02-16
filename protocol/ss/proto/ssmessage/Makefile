all:
	protoc --go_out=. *.proto
	rm -rf ../../ssmessage/*.go
	mv *.go ../../ssmessage