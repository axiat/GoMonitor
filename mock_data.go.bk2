package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	//打开文件
	f, err := os.OpenFile("./access.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Sprintf("open file error:%s", err.Error()))
	}
	//defer f.Close() Go写文件时不能defer Close()
	//https://www.joeshaw.org/dont-defer-close-on-writable-files/
	defer func() {
		cErr := f.Close()
		if err != nil {
			err = cErr
			panic(fmt.Sprintf("Error when Closing file,%s", err.Error()))
		}
	}()

	for i := 0; i <= 100; i++ {
		date := time.Now().Format("02/Jan/2006:15:04:05 +0000")
		ret := strings.Join([]string{"172.0.0.12 - - [", date, "] http \"GET /foo?query=t HTTP/1.0\" 200 2133 \"-\" \"KeepAliveClient\" \"-\" 1.005 1.854\n"}, "")
		//fmt.Println(ret)
		if _, err := f.WriteString(ret); err != nil {
			panic(fmt.Sprintf("Write file error:%s", err.Error()))
		}

		time.Sleep(time.Second)
	}
}
