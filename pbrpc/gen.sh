#!/bin/bash
 
function make_proto(){
    for file in `ls ./proto`       	
    do
        if [ "${file##*.}"x = "proto"x ];then
            protoc --go_out=./service "./proto/"$file
        fi
    done
}   
 
# 执行命令
make_proto
# 生成rpc文件
go run genrpc/gen.go