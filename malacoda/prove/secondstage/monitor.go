package main

import (
	"fmt"
	"time"
)


func main() {
	fmt.Println("Alarm")
	for {
		time.Sleep(2 * time.Second)
		list_suspected()
	}
}

func list_suspected() {
    sampleCgroup()
}
