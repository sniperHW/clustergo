gen_ss:
	cd protocol/ss/gen;go run gen_proto_go.go;cd ../../../
	cd protocol/ss/proto;make;cd ../../../
gen_cs:
	cd protocol/cs/gen;go run gen_proto_go.go;cd ../../../
	cd protocol/cs/proto;make;cd ../../../
gen_rpc:
	cd rpc;go run gen_rpc.go;cd ../