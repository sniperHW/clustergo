protoc --go_out=. *.proto

cd ../../rpc
del *.go
cd ../proto/rpc

move *.go ../../rpc