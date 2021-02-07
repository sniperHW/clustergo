gen_cs:
	test -d cs/message || mkdir -p cs/message;	rm cs/message/*.go; protoc -I $(shell pwd) --go_out=paths=source_relative:. cs/proto/message/*.proto;
	cd cs/proto/message;mv *.go ../../message;cd ../../../

gen_ss:
	test -d ss/ssmessage || mkdir -p ss/ssmessage;	rm ss/ssmessage/*.go; protoc -I $(shell pwd) --go_out=paths=source_relative:. ss/proto/ssmessage/*.proto;
	cd ss/proto/ssmessage;mv *.go ../../ssmessage;cd ../../../
	test -d ss/rpc || mkdir -p ss/rpc;	rm ss/rpc/*.go; protoc -I $(shell pwd) --go_out=paths=source_relative:. ss/proto/rpc/*.proto;
	cd ss/proto/rpc;mv *.go ../../rpc;cd ../../../