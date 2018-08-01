package main

import (
    "fmt"
    "log"
    "github.com/globalsign/mgo"
    "github.com/globalsign/mgo/bson"
    "github.com/sniperHW/kendynet/util/asyn"
    "github.com/sniperHW/kendynet"    
)

type Ua struct {
    Username  string
    Nousing   int32
    Short_id  string
    Pid       string
}

func main() {
    session, err := mgo.Dial("localhost:27017")
    if err != nil {
        panic(err)
    }
    defer session.Close()

    // Optional. Switch the session to a monotonic behavior.
    session.SetMode(mgo.Monotonic, true)

    c := session.DB("game").C("ua")

    result := Ua{}

    //基于回调的异步调用

    queue := kendynet.NewEventQueue()

    one := asyn.AsynWrap(queue,c.Find(bson.M{"username": "xq07091"}).One)

    one(func(ret []interface{}) {
        if nil != ret[0] {
            log.Fatal(ret[0].(error))
        } else {
            fmt.Println(result)
        }
        queue.Close()
    },&result)

    queue.Run()

    /*
    //同步调用
    err = c.Find(bson.M{"username": "xq07091"}).One(&result)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result)*/
}