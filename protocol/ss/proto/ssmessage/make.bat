protoc --go_out=. *.proto

cd ../../ssmessage
del *.go
cd ../proto/ssmessage

move *.go ../../ssmessage