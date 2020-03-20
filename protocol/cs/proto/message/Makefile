all:
	protoc --go_out=. *.proto
	rm -rf ../../message/*.go
	mv *.go ../../message