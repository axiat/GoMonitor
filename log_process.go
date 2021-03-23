package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	client "github.com/influxdata/influxdb1-client/v2"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//定义接口
type Reader interface {
	Read(rc chan []byte)
}

func (r *ReadFromFile) Read(rc chan []byte) {
	// 读取模块

	// 打开文件
	f, err := os.Open(r.path)
	if err != nil {
		TypeMonitorChan <- TypeErrNum
		panic(fmt.Sprintf("open file error:%s", err.Error()))
	}

	// 从文件末尾开始逐行读取文件内容
	if _, err = f.Seek(0, 2); err != nil {
		TypeMonitorChan <- TypeErrNum
		panic(fmt.Sprintf("F Seek error:%s", err.Error()))
	} // 2表示把字符指针移动到文件末尾

	rd := bufio.NewReader(f)

	for {
		line, err := rd.ReadBytes('\n')
		if err == io.EOF {
			time.Sleep(500 * time.Millisecond)
			continue
		} else if err != nil {
			TypeMonitorChan <- TypeErrNum
			panic(fmt.Sprintf("Readbyte error:%s", err.Error()))
		}
		TypeMonitorChan <- TypeHandleLine
		rc <- line[:len(line)-1] // 去除换行符
	}
}

type ReadFromFile struct {
	path string //读取文件的路径
}

type Writer interface {
	Write(wc chan *Message)
}

func (w *WriteToInfluxDB) Write(wc chan *Message) {
	// 写入模块
	infSli := strings.Split(w.influxDBDsn, "@")
	// Create a new HTTPClient
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     infSli[0],
		Username: infSli[1],
		Password: infSli[2],
	})
	if err != nil {
		log.Fatal(err)
	}
	//defer c.Close()
	defer func() {
		cErr := c.Close()
		if cErr != nil {
			err = cErr
			TypeMonitorChan <- TypeErrNum
			panic(fmt.Sprintf("Error closing HTTPClient, %s", err.Error()))
		}
	}()

	for v := range wc { // Create a new point batch
		bp, err := client.NewBatchPoints(client.BatchPointsConfig{
			Database:  infSli[3],
			Precision: infSli[4],
		})
		if err != nil {
			log.Fatal(err)
		}

		// Create a point and add to batch
		// Tags: Path, Method, Scheme, Status
		tags := map[string]string{"Path": v.Path, "Method": v.Method, "Scheme": v.Scheme, "Status": v.Status}
		// Fields: UpstreamTime, RequestTime, BytesSent
		fields := map[string]interface{}{
			"UpstreamTime": v.UpstreamTime,
			"RequestTime":  v.RequestTime,
			"BytesSent":    v.BytesSent,
		}

		pt, err := client.NewPoint("nginx_log", tags, fields, v.TimeLocal)
		if err != nil {
			log.Fatal(err)
		}
		bp.AddPoint(pt)

		// Write the batch
		if err := c.Write(bp); err != nil {
			log.Fatal(err)
		}
		log.Println("write success")
	}
	// Close client resources
	if err := c.Close(); err != nil {
		log.Fatal(err)
	}

}

type WriteToInfluxDB struct {
	influxDBDsn string //读取文件的路径
}

type LogProcess struct {
	rc    chan []byte
	wc    chan *Message
	read  Reader
	write Writer
	//path        string //读取文件的路径
	//influxDBDsn string //influx data source
}

type Message struct {
	TimeLocal                    time.Time
	BytesSent                    int
	Path, Method, Scheme, Status string
	UpstreamTime, RequestTime    float64
}

type SystemInfo struct {
	HandleLine int     `json:"HandleLine"`
	Tps        float64 `json:"Tps"`
	RcLength   int     `json:"RcLength"`
	WcLength   int     `json:"WcLength"`
	RunTime    string  `json:"RunTime"`
	ErrNum     int     `json:"ErrNum"`
}

const TypeHandleLine = 0
const TypeErrNum = 1

var TypeMonitorChan = make(chan int, 200)

type Monitor struct {
	startTime time.Time
	data      SystemInfo
	tpsSli    []int
}

func (m *Monitor) start(lp *LogProcess) {

	go func() {
		for n := range TypeMonitorChan {
			switch n {
			case TypeHandleLine:
				m.data.HandleLine += 1
			case TypeErrNum:
				m.data.ErrNum += 1
			}
		}
	}()

	ticker := time.NewTicker(time.Second * 5)
	go func() {
		for {
			<-ticker.C
			m.tpsSli = append(m.tpsSli, m.data.HandleLine)
			if len(m.tpsSli) > 2 {
				m.tpsSli = m.tpsSli[1:]
			}
		}
	}()

	http.HandleFunc("/monitor", func(writer http.ResponseWriter, request *http.Request) {
		m.data.RunTime = time.Now().Sub(m.startTime).String()
		m.data.RcLength = len(lp.wc)
		m.data.WcLength = len(lp.wc)
		if len(m.tpsSli) >= 2 {
			m.data.Tps = float64(m.tpsSli[1]-m.tpsSli[0]) / 5
		}
		ret, _ := json.MarshalIndent(m.data, "", "\t")
		if _, err := io.WriteString(writer, string(ret)); err != nil {
			TypeMonitorChan <- TypeErrNum
			panic(fmt.Sprintf("WriteString Err: %s", err.Error()))
		}
	})
	if err := http.ListenAndServe(":9193", nil); err != nil {
		TypeMonitorChan <- TypeErrNum
		panic(fmt.Sprintf("ListenAndServer Err:%s", err.Error()))
	}
}

func (l *LogProcess) Process() {
	//解析模块
	/**
	172.0.0.12 - - [04/Mar/2018:13:49:52 +0000] http "GET /foo?query=t HTTP/1.0" 200 2133 "-"
	"KeepAliveClient" "-" 1.005 1.854
	([\d\.]+)\s+([^ \[]+)\s+([^ \[]+)\s+\[([^\]]+)\]\s+([a-z]+)\s+\"([^"]+)\"\s+(\d{3})\s+(
	\d+)\s+\"([^"]+)\"\s+\"(.*?)\"\s+\"([\d\.\-]+)\"\s+([\d\.-]+)\s+([\d\.-]+)
	*/
	// 这里reg[2],reg[3] [^ \[]等同于[^\s\[]
	//r := regexp.MustCompile(`([\d\.]+)\s+([^ \[]+)\s+([^ \[]+)\s+\[([^\]]+)\]\s+([a-z]+)\s+\"([^"]+)\"\s+(\d{3})\s+(\d+)\s+\"([^"]+)\"\s+\"(.*?)\"\s+\"([\d\.\-]+)\"\s+([\d\.-]+)\s+([\d\.-]+)`)
	r := regexp.MustCompile(`([\d.]+)\s+([^ \[]+)\s+([^ \[]+)\s+\[([^]]+)]\s+([a-z]+)\s+"([^"]+)"\s+(\d{3})\s+(\d+)\s+"([^"]+)"\s+"(.*?)"\s+"([\d.\-]+)"\s+([\d.-]+)\s+([\d.-]+)`)

	loc, _ := time.LoadLocation("Asia/Shanghai")

	for v := range l.rc {
		ret := r.FindStringSubmatch(string(v)) // FindStringSubmatch返回正则表达式匹配后 括号()中的内容
		//log.Println(ret, len(ret))
		if len(ret) != 14 { // 共有13个开阔好
			TypeMonitorChan <- TypeErrNum
			log.Println("FindStringSubmatch fail:", string(v))
			continue
		} else {

			message := &Message{}

			// 04/Mar/2018:13:49:52 +0000
			t, err := time.ParseInLocation("02/Jan/2006:15:04:05 +0000", ret[4], loc)
			if err != nil {
				TypeMonitorChan <- TypeErrNum
				log.Println("ParseInLocation fail:", err.Error(), ret[4])
				continue
			}
			message.TimeLocal = t

			// 2133
			byteSent, _ := strconv.Atoi(ret[8])
			message.BytesSent = byteSent

			// GET /foo?query=t HTTP/1.0
			reqSli := strings.Split(ret[6], " ")
			if len(reqSli) != 3 {
				TypeMonitorChan <- TypeErrNum
				log.Println("string.Split fail:", ret[6])
				continue
			}
			message.Method = reqSli[0]

			u, err := url.Parse(reqSli[1])
			if err != nil {
				TypeMonitorChan <- TypeErrNum
				log.Println("url parse fail:", err)
				continue
			}
			message.Path = u.Path

			//http
			message.Scheme = ret[5]
			message.Status = ret[7]
			upstreamTime, _ := strconv.ParseFloat(ret[12], 64)
			requestTime, _ := strconv.ParseFloat(ret[13], 64)
			message.UpstreamTime = upstreamTime
			message.RequestTime = requestTime

			l.wc <- message
		}
	}
}

func main() {
	var path, influxDsn string
	flag.StringVar(&path, "path", "./access.log", "read file path")
	flag.StringVar(&influxDsn, "influxDsn", "http://127.0.0.1:8086@qin@qinning@mydb@s", "influx data source")
	flag.Parse()

	r := &ReadFromFile{
		//path: "/tmp/access.log",
		//path: "./access.log",
		path: path,
	}

	w := &WriteToInfluxDB{
		//influxDBDsn: "./access.log",
		//influxDBDsn: "http://127.0.0.1:8086@qin@qinning@mydb@s",
		influxDBDsn: influxDsn,
	}

	lp := &LogProcess{
		rc: make(chan []byte),
		wc: make(chan *Message),
		//path:        "/tmp/access.log",
		//influxDBDsn: "username&password..",
		read:  r,
		write: w,
	}

	go (*lp).read.Read(lp.rc) //lp就是(*lp),golang的编译器优化
	// 接口的好处：约束了实现它的类的功能
	go lp.Process()
	go lp.write.Write(lp.wc)

	m := &Monitor{startTime: time.Now(), data: SystemInfo{}}
	m.start(lp)

	//time.Sleep(300 * time.Second)
}

//func (l *LogProcess) ReadFromFile() {
//	//读取模块
//	//为什么用* (引用)
//	// 1.不用拷贝结构体,性能优势
//	// 2.可以使用l修改自身定义的一些参数
//	line := "message"
//	l.rc <- line
//}

//func (l *LogProcess) WriteToInfluxDB() {
//	//写入模块
//	fmt.Println(<-l.wc)
//}
