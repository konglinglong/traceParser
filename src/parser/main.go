package main

import "bufio"
import "io/ioutil"
import "time"
import "runtime"
import "strings"
import "encoding/binary"
import "encoding/json"
import "path/filepath"
import "flag"
import "bytes"
import "fmt"
import "os"

//type TraceItem struct {
//	magicNum  uint32
//	sec       uint32
//	usec      uint32
//	traceType uint16
//	traceSize uint16
//	data      [4096]uint8
//}

type StructMemberDescribe struct {
	FieldName string       `json:"field_name"`
	Offset    json.Number  `json:"offset"`
	Size      json.Number  `json:"size"`
	PrintFmt  json.Number  `json:"print_fmt"`
	ShowFlag  json.Number  `json:"show_flag"`
	SubStruct *string      `json:"sub_struct"`
	ArrayLen  *json.Number `json:"array_len"`
}

type StructDescribe struct {
	StructSize json.Number            `json:"struct_size"`
	MemberList []StructMemberDescribe `json:"member_list"`
}

type TraceDescribeTable struct {
	Version                   string                    `json:"version"`
	BuildTime                 string                    `json:"build_time"`
	StructId2StructNameTable  map[string]string         `json:"struct_id_and_struct_name_table"`
	StructId2XlsFileNameTable map[string]string         `json:"struct_id_and_xls_file_name_table"`
	StructDescribeTable       map[string]StructDescribe `json:"struct_describe_table"`
}

type WriteFileInfo struct {
	file *os.File
	data string
}

const TRACE_HDR_SIZE = 16
const CPU_PROCESS_NUM_MAX = 64

var gDescTable TraceDescribeTable
var gFileDir string = ""

var gXlsFile map[string]*os.File

//创建文件夹
func createDir(fileName string) string {

	//使用解析工具路径
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		fmt.Println("filepath.Abs error!", err)
		return ""
	}

	diagonal := "\\"
	if strings.Contains(dir, "/") {
		diagonal = "/"
	}
	dir = dir + diagonal + fileName

	dotPos := strings.LastIndex(dir, ".")
	if dotPos != -1 {
		dir = strings.TrimSuffix(dir, dir[dotPos:])
	}
	dir += diagonal

	err = os.Mkdir(dir, os.ModePerm)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	return dir
}

//创建XLS文件
func createXlsFile(structId string) string {

	xlsFileName, err := gDescTable.StructId2XlsFileNameTable[structId]
	if !err {
		fmt.Println("can not find struct : ", structId)
		return ""
	}
	fullPathName := gFileDir + xlsFileName + ".csv"

	file, err1 := os.OpenFile(fullPathName, os.O_RDWR|os.O_CREATE, 0766)
	if err1 != nil {
		fmt.Println(err1)
		return ""
	}

	gXlsFile[structId] = file

	return fullPathName
}

//解析跟踪结构体描述
func parseStructDesc(structName *string, fatherStructName *string, buffer *bytes.Buffer) {
	structDesc, err := gDescTable.StructDescribeTable[*structName]
	if !err {
		fmt.Println("can not find struct : ", *structName)
		return
	}
	//	fmt.Println(structDesc)
	fStructName := ""
	if fatherStructName != nil {
		fStructName = *fatherStructName + "."
	}
	for i := 0; i < len(structDesc.MemberList); i++ {
		//		fmt.Println(structDesc.MemberList[i])
		if structDesc.MemberList[i].ShowFlag != "1" {
			//			fmt.Println("continue")
			continue
		}

		isSubscriptExist := false //是否存在下标
		arrayLen := 1             //数组长度
		if structDesc.MemberList[i].ArrayLen != nil {
			alen, err := structDesc.MemberList[i].ArrayLen.Int64()
			if err != nil {
				fmt.Println("Int64() : ", err)
				return
			}
			isSubscriptExist = true
			arrayLen = int(alen)
		}

		subscript := "" //下标
		for ii := 0; ii < arrayLen; ii++ {
			if isSubscriptExist {
				subscript = fmt.Sprintf("[%d]", ii)
			}
			fieldName := structDesc.MemberList[i].FieldName + subscript
			if structDesc.MemberList[i].SubStruct != nil {
				parseStructDesc(structDesc.MemberList[i].SubStruct, &fieldName, buffer)
			} else {
				buffer.WriteString(fStructName)
				buffer.WriteString(fieldName)
				buffer.WriteString(",")
			}
		}
	}
}

//解析跟踪数据
func parseStructData(structName *string, structData []byte, buffer *bytes.Buffer) {
	structDesc, err := gDescTable.StructDescribeTable[*structName]
	if !err {
		fmt.Println("can not find struct : ", *structName)
		return
	}
	//	fmt.Println(structDesc)
	for i := 0; i < len(structDesc.MemberList); i++ {
		//		fmt.Println(structDesc.MemberList[i])
		if structDesc.MemberList[i].ShowFlag != "1" {
			//			fmt.Println("continue")
			continue
		}

		arrayLen := 1 //数组长度
		if structDesc.MemberList[i].ArrayLen != nil {
			alen, err := structDesc.MemberList[i].ArrayLen.Int64()
			if err != nil {
				fmt.Println("ArrayLen.Int64() : ", err)
				return
			}
			arrayLen = int(alen)
		}

		for ii := 0; ii < arrayLen; ii++ {
			mSize, err := structDesc.MemberList[i].Size.Int64()
			if err != nil {
				fmt.Println("Size.Int64() : ", err)
				return
			}
			mOffset, err := structDesc.MemberList[i].Offset.Int64()
			if err != nil {
				fmt.Println("Offset.Int64() : ", err)
				return
			}
			valueBytes := structData[int(mOffset)+int(mSize)*ii:]
			if structDesc.MemberList[i].SubStruct != nil {
				parseStructData(structDesc.MemberList[i].SubStruct, valueBytes, buffer)
			} else {
				var value uint64 = 0
				switch structDesc.MemberList[i].Size {
				case "1":
					value = uint64(valueBytes[0])
				case "2":
					value = uint64(binary.BigEndian.Uint16(valueBytes[:2]))
				case "4":
					value = uint64(binary.BigEndian.Uint32(valueBytes[:4]))
				case "8":
					value = uint64(binary.BigEndian.Uint64(valueBytes[:8]))
				default:
					fmt.Println("structDesc.MemberList[i].Size error!")
				}

				valueStr := ""
				switch structDesc.MemberList[i].PrintFmt {
				case "0":
					valueStr = fmt.Sprintf("%d", int64(value))
				case "1":
					valueStr = fmt.Sprintf("%d", value)
				case "2":
					valueStr = fmt.Sprintf("%#x", value)
				default:
					fmt.Println("structDesc.MemberList[i].PrintFmt error!")
				}
				buffer.WriteString(valueStr)
				buffer.WriteString(",")
			}
		}
	}
}

//检查数据头部的traceSize是否与结构体描述表里面的定义一致
func checkTrcDataHdrSize(hdrSize uint16, structName string) int {
	structDesc, err := gDescTable.StructDescribeTable[structName]
	if !err {
		fmt.Println("can not find struct : ", structName)
		return -1
	}
	structSize, err1 := structDesc.StructSize.Int64()
	if err1 != nil {
		fmt.Println("StructSize.Int64() : ", err1)
		return -1
	}
	if hdrSize-TRACE_HDR_SIZE != uint16(structSize) {
		fmt.Printf("ERROR : struct(%s) trcDataHdrSize[%d] != trcDescSize[%d], check describe table version!\n", structName, hdrSize-TRACE_HDR_SIZE, structSize)
		return -1
	}
	return 0
}

//写文件线程
func writeFileWorker(dataChan <-chan WriteFileInfo) {
	for {
		wdata, ok := <-dataChan
		if !ok {
			break
		}
		wdata.file.WriteString(wdata.data)
	}
}

//解析跟踪数据线程
func parseDataWorker(jobNum int, jobId int, dataChan <-chan []byte, syncChans []chan int, writeCh chan<- WriteFileInfo) {
	for {
		item, ok := <-dataChan
		if !ok {
			break
		}

		sec := binary.BigEndian.Uint32(item[:4])
		usec := binary.BigEndian.Uint32(item[4:8])
		trcType := binary.BigEndian.Uint16(item[8:10])
		trcSize := binary.BigEndian.Uint16(item[10:12])

		structId := fmt.Sprintf("%d", trcType)
		structName := gDescTable.StructId2StructNameTable[structId]

		if checkTrcDataHdrSize(trcSize, structName) != 0 {
			if jobNum > 1 {
				<-syncChans[jobId%jobNum]
				syncChans[(jobId+1)%jobNum] <- 1
			}
			continue
		}

		buffer := bytes.NewBufferString(fmt.Sprintf("%s, %d, ", time.Unix(int64(sec), int64(usec)).Format("060102 15:04:05"), usec/1000))
		//减4是少了MAGIC
		parseStructData(&structName, item[TRACE_HDR_SIZE-4:], buffer)
		buffer.WriteString("\n")

		var writeInfo WriteFileInfo
		writeInfo.file = gXlsFile[structId]
		writeInfo.data = buffer.String()

		if jobNum > 1 {
			<-syncChans[jobId%jobNum]
			writeCh <- writeInfo
			syncChans[(jobId+1)%jobNum] <- 1
		} else {
			writeCh <- writeInfo
		}
	}
}

func main() {

	start := time.Now()

	help := flag.Bool("h", false, "show this help")
	trcDataFile := flag.String("data", "", "set trace data file")
	trcDescFile := flag.String("desc", "", "set trace describe file")

	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	if *trcDataFile == "" || *trcDescFile == "" {
		flag.Usage()
		return
	}

	cpuNum := runtime.NumCPU()
	if cpuNum > CPU_PROCESS_NUM_MAX {
		cpuNum = CPU_PROCESS_NUM_MAX
	}
	if cpuNum > 1 {
		cpuNum = cpuNum - 1
		runtime.GOMAXPROCS(cpuNum)
	}

	gFileDir = createDir(*trcDataFile)
	if gFileDir == "" {
		fmt.Println("createDir error!", gFileDir)
		return
	}

	gXlsFile = make(map[string]*os.File, 512)

	dataFile, err := os.Open(*trcDataFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer dataFile.Close()

	descData, err := ioutil.ReadFile(*trcDescFile)
	if err != nil {
		fmt.Println("open trc_data_describe_table.txt error!", err)
		return
	}

	err = json.Unmarshal(descData, &gDescTable)
	if err != nil {
		fmt.Println("Unmarshal error!", err)
		return
	}
	//	fmt.Println(gDescTable)

	writeChan := make(chan WriteFileInfo, 512)

	var dataChs [CPU_PROCESS_NUM_MAX]chan []byte //数据发送信道
	var syncChs [CPU_PROCESS_NUM_MAX]chan int    //同步信道,用于控制写入文件的顺序与读取的一致
	dataChans := dataChs[:cpuNum]
	syncChans := syncChs[:cpuNum]
	for i := 0; i < cpuNum; i++ {
		dataChans[i] = make(chan []byte)
		syncChans[i] = make(chan int)
		go parseDataWorker(cpuNum, i, dataChans[i], syncChans[:], writeChan)
	}
	go writeFileWorker(writeChan)

	//用于首次触发
	if cpuNum > 1 {
		go func() {
			syncChans[0] <- 1
		}()
	}

	loop := 0
	dataReader := bufio.NewReader(dataFile)
	for {
		item, err := dataReader.ReadBytes(0xaa)
		if err != nil {
			//			fmt.Println(err)
			break
		}

		itemLen := len(item)
		//检查MAGIC
		if itemLen > TRACE_HDR_SIZE && item[itemLen-4] == 0xdd && item[itemLen-3] == 0xcc && item[itemLen-2] == 0xbb {

			trcType := binary.BigEndian.Uint16(item[8:10])
			trcSize := binary.BigEndian.Uint16(item[10:12])
			structId := fmt.Sprintf("%d", trcType)
			structName, err := gDescTable.StructId2StructNameTable[structId]
			if !err {
				continue
			}
			if int(trcSize) == itemLen {

				file, err := gXlsFile[structId]
				//如果还没有创建文件,先创建,并写入表头第一行
				if !err {
					createXlsFile(structId)
					file, err = gXlsFile[structId]

					//写入表头
					buffer := bytes.NewBufferString("time,ms,")
					parseStructDesc(&structName, nil, buffer)
					buffer.WriteString("\n")
					file.WriteString(buffer.String())
				}

				dataChans[loop%cpuNum] <- item
				loop++
				if loop%10000 == 0 {
					fmt.Printf("trace data num :%12d\n", loop)
				}

			} else {
				fmt.Printf("HDR : traceSize[%d] error! trcType[%d]\n", trcSize, trcType)
			}
		}
	}
	fmt.Printf("trace data num :%12d\n", loop)

	//耗时打印
	fmt.Printf("elapsed time : %s\n", time.Since(start))
}
