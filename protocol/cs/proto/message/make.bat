	protoc --go_out=. *.proto
	
	cd  ../../message
	del *.go
	cd ../proto/message
	
	move *.go ../../message
pause