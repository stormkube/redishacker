package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	//"reflect"
	"time"

	"github.com/attic-labs/noms/go/types"
	"github.com/stormasm/redishacker/firego"
	"github.com/stormasm/redishacker/redisc"
)

type datum struct {
	index float64
	value types.Struct
}

func main() {
	hv := bigSync()
	fmt.Println(hv.Hash().String())
	/*
		fmt.Print("Enter text: ")
		var input string
		fmt.Scanln(&input)
		fmt.Println(input)
	*/
}

func bigSync() types.Value {
	newIndex := make(chan float64, 1000)
	newDatum := make(chan datum, 100)
	streamData := make(chan types.Value, 100)
	newMap := types.NewStreamingMap(types.NewTestValueStore(), streamData)

	go func() {
		for i := 8432709.0; i < 8432712.0; i++ {
			newIndex <- i
		}
		close(newIndex)
	}()

	workerPool(500, func() {
		churn(newIndex, newDatum)
	}, func() {
		close(newDatum)
	})

	for datum := range newDatum {
		streamData <- types.Number(datum.index)
		streamData <- datum.value
	}
	close(streamData)

	fmt.Println("generating map...")
	mm := <-newMap
	return types.NewStruct("HackerNoms", types.StructData{
		"items": mm,
		"top":   types.NewList(types.Number(0)),
	})
}

func workerPool(count int, work func(), done func()) {
	workerDone := make(chan bool, 1)
	for i := 0; i < count; i += 1 {
		go func() {
			work()
			workerDone <- true
		}()
	}

	go func() {
		for i := 0; i < count; i += 1 {
			_ = <-workerDone
		}
		close(workerDone)
		done()
	}()
}

func makeClient() *http.Client {
	var tr *http.Transport
	tr = &http.Transport{
		Dial: func(network, address string) (net.Conn, error) {
			return net.DialTimeout(network, address, 30*time.Second)
		},
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 30,
	}
	return client
}

func churn(newIndex <-chan float64, newData chan<- datum) {
	client := makeClient()

	for index := range newIndex {
		id := int(index)
		url := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d", id)
		fb := firego.New(url, client)
		bytes, err := fb.DoRequest("GET", nil)
		if err != nil {
			fmt.Printf("DoRequest failed for %d %s\n", id, err)
		}
		n := len(bytes)
		var dst []byte
		dst = make([]byte, n, n)
		copy(dst, bytes)
		name := getTypeFromBytes(id, dst)
		fmt.Println(id, name)
		redisc.Write_json_bytes("hackernews", name, id, bytes)
	}
}

func getTypeFromBytes(id int, bytes []byte) (hntype string) {
	var val map[string]interface{}
	err := json.Unmarshal(bytes, &val)
	if err != nil {
		fmt.Printf("json Unmarshal failed for %d %s\n", id, err)
	}

	hntype, ok := val["type"].(string)
	if !ok {
		fmt.Printf("no type for id %d; trying again\n", id)
	}
	return hntype
}