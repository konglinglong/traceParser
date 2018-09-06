package main

import "bufio"
import "io/ioutil"
import "time"
import "runtime"
import "strings"
import "runtime/pprof"
import "encoding/binary"
import "encoding/json"
import "path/filepath"
import "strconv"
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
	FieldName       string            `json:"field_name"`
	Offset          int               `json:"offset"`
	Size            int               `json:"size"`
	PrintFmt        int               `json:"print_fmt"`
	ShowFlag        int               `json:"show_flag"`
	IsArray         int               `json:"is_array"`
	ArrayLen        int               `json:"array_len"`
	HaveSubStruct   int               `json:"have_sub_struct"`
	SubStruct       string            `json:"sub_struct"`
	ValueAliasTable map[string]string `json:"value_alias_table"`
}

type StructDescribe struct {
	StructSize int                    `json:"struct_size"`
	MemberList []StructMemberDescribe `json:"member_list"`
}

type TraceDescribeTable struct {
	UserVersion               string                    `json:"user_version"`
	ToolVersion               string                    `json:"tool_version"`
	BuildTime                 string                    `json:"build_time"`
	StructId2StructNameTable  map[string]string         `json:"struct_id_and_struct_name_table"`
	StructId2XlsFileNameTable map[string]string         `json:"struct_id_and_xls_file_name_table"`
	StructDescribeTable       map[string]StructDescribe `json:"struct_describe_table"`
}

type XlsFileCtrlBlock struct {
	file *os.File
	size int
	num  int
}

type AsyncWriteCtrlBlock struct {
	structId   string
	structName string
	data       []byte
}

const TRACE_HDR_SIZE = 16
const CPU_PROCESS_NUM_MAX = 64
const TRACE_CSV_FILE_SIZE_MAX = 10 //10M

var gDescTable TraceDescribeTable
var gFileDir string = ""

var gXlsFile map[string]XlsFileCtrlBlock
var gXlsFileSize *int

//var gXlsFile sync.Map

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
		fmt.Println("can not find structId : ", structId)
		return ""
	}

	xlsFileCb := XlsFileCtrlBlock{nil, 0, 0}
	xlsFileCb, _ = gXlsFile[structId]

	fullPathName := fmt.Sprintf("%s%s(%d).csv", gFileDir, xlsFileName, xlsFileCb.num)

	file, err1 := os.OpenFile(fullPathName, os.O_RDWR|os.O_CREATE, 0766)
	if err1 != nil {
		fmt.Println(err1)
		return ""
	}

	xlsFileCb.file = file
	xlsFileCb.num++

	gXlsFile[structId] = xlsFileCb

	return fullPathName
}

//解析跟踪结构体描述
func parseStructDesc(structName *string, fatherStructName *string, buffer *bytes.Buffer) {
	structDesc, err := gDescTable.StructDescribeTable[*structName]
	if !err {
		fmt.Println("can not find structName : ", *structName)
		return
	}
	//	fmt.Println(structDesc)
	fStructName := ""
	if fatherStructName != nil {
		fStructName = *fatherStructName + "."
	}
	for i := 0; i < len(structDesc.MemberList); i++ {
		//		fmt.Println(structDesc.MemberList[i])
		if structDesc.MemberList[i].ShowFlag != 1 {
			//			fmt.Println("continue")
			continue
		}

		subscript := "" //下标
		for ii := 0; ii < structDesc.MemberList[i].ArrayLen; ii++ {
			if structDesc.MemberList[i].IsArray == 1 {
				subscript = "[" + strconv.FormatUint(uint64(ii), 10) + "]"
			}
			fieldName := structDesc.MemberList[i].FieldName + subscript
			if structDesc.MemberList[i].HaveSubStruct == 1 {
				parseStructDesc(&structDesc.MemberList[i].SubStruct, &fieldName, buffer)
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
		//		fmt.Println("can not find struct : ", *structName)
		return
	}
	//	fmt.Println(structDesc)
	for i := 0; i < len(structDesc.MemberList); i++ {
		if structDesc.MemberList[i].ShowFlag != 1 {
			continue
		}

		for ii := 0; ii < structDesc.MemberList[i].ArrayLen; ii++ {
			valueBytes := structData[structDesc.MemberList[i].Offset+structDesc.MemberList[i].Size*ii:]
			if structDesc.MemberList[i].HaveSubStruct == 1 {
				parseStructData(&structDesc.MemberList[i].SubStruct, valueBytes, buffer)
			} else {
				var valueI64 int64 = 0
				var valueU64 uint64 = 0
				switch structDesc.MemberList[i].Size {
				case 1:
					valueU8 := uint8(valueBytes[0])
					valueU64 = uint64(valueU8)
					valueI64 = int64(int8(valueU8))
				case 2:
					valueU16 := binary.BigEndian.Uint16(valueBytes[:2])
					valueU64 = uint64(valueU16)
					valueI64 = int64(int16(valueU16))
				case 4:
					valueU32 := binary.BigEndian.Uint32(valueBytes[:4])
					valueU64 = uint64(valueU32)
					valueI64 = int64(int32(valueU32))
				case 8:
					valueU64 = binary.BigEndian.Uint64(valueBytes[:8])
					valueI64 = int64(valueU64)
				default:
					fmt.Println("structDesc.MemberList[i].Size error!")
				}

				valueStr := strconv.FormatInt(valueI64, 10)
				valueAlias, err := structDesc.MemberList[i].ValueAliasTable[valueStr]
				if !err {
					switch structDesc.MemberList[i].PrintFmt {
					case 0:
						buffer.WriteString(strconv.FormatInt(valueI64, 10))
					case 1:
						buffer.WriteString(strconv.FormatUint(valueU64, 10))
					case 2:
						buffer.WriteString("0x")
						buffer.WriteString(strconv.FormatUint(valueU64, 16))
					default:
						fmt.Println("structDesc.MemberList[i].PrintFmt error!")
					}
				} else {
					buffer.WriteString(valueAlias)
				}
				buffer.WriteString(",")
			}
		}
	}
}

//检查数据头部的traceSize是否与结构体描述表里面的定义一致
func checkTrcDataHdrSize(hdrSize uint16, structName string) int {
	structDesc, err := gDescTable.StructDescribeTable[structName]
	if !err {
		fmt.Println("checkTrcDataHdrSize() can not find structName : ", structName)
		return -1
	}
	if hdrSize-TRACE_HDR_SIZE != uint16(structDesc.StructSize) {
		fmt.Printf("ERROR : struct(%s) trcDataHdrSize[%d] != trcDescSize[%d], check describe table version!\n",
			structName, hdrSize-TRACE_HDR_SIZE, structDesc.StructSize)
		return -1
	}
	return 0
}

//写文件线程
func writeFileWorker(dataChan <-chan AsyncWriteCtrlBlock) {
	for {
		asyncData, ok := <-dataChan
		if !ok {
			break
		}

		xlsFileCb, err := gXlsFile[asyncData.structId]
		if !err || xlsFileCb.file == nil {
			createXlsFile(asyncData.structId)

			xlsFileCb, _ = gXlsFile[asyncData.structId]
			//写入表头
			buffer := bytes.NewBufferString("time,ms,")
			parseStructDesc(&asyncData.structName, nil, buffer)
			buffer.WriteString("\n")
			xlsFileCb.file.Write(buffer.Bytes())
		}

		xlsFileCb.file.Write(asyncData.data)
		xlsFileCb.size += len(asyncData.data)
		if xlsFileCb.size > *gXlsFileSize {
			xlsFileCb.file.Close()
			xlsFileCb.file = nil
			xlsFileCb.size = 0
		}
		gXlsFile[asyncData.structId] = xlsFileCb
	}
}

//解析跟踪数据线程
func parseDataWorker(jobNum int, jobId int, dataChan <-chan []byte, syncChans []chan int, writeCh chan<- AsyncWriteCtrlBlock) {
	for {
		item, ok := <-dataChan
		if !ok {
			break
		}

		sec := binary.BigEndian.Uint32(item[:4])
		usec := binary.BigEndian.Uint32(item[4:8])
		trcType := binary.BigEndian.Uint16(item[8:10])
		trcSize := binary.BigEndian.Uint16(item[10:12])

		structId := strconv.FormatUint(uint64(trcType), 10)
		structName, _ := gDescTable.StructId2StructNameTable[structId]

		if checkTrcDataHdrSize(trcSize, structName) != 0 {
			if jobNum > 1 {
				<-syncChans[jobId%jobNum]
				syncChans[(jobId+1)%jobNum] <- 1
			}
			continue
		}

		buffer := bytes.NewBufferString(time.Unix(int64(sec), int64(usec)).Format("060102 15:04:05"))
		buffer.WriteString(",")
		buffer.WriteString(strconv.FormatUint(uint64(usec/1000), 10))
		buffer.WriteString(",")
		//减4是少了MAGIC
		parseStructData(&structName, item[TRACE_HDR_SIZE-4:], buffer)
		buffer.WriteString("\n")

		asyncWrite := AsyncWriteCtrlBlock{structId, structName, buffer.Bytes()}

		if jobNum > 1 {
			<-syncChans[jobId%jobNum]
			writeCh <- asyncWrite
			syncChans[(jobId+1)%jobNum] <- 1
		} else {
			writeCh <- asyncWrite
		}
	}
}

func main() {

	start := time.Now()

	help := flag.Bool("h", false, "show this help")
	trcDataFile := flag.String("data", "", "set trace data file.")
	trcDescFile := flag.String("desc", "", "set trace describe file.")
	profile := flag.String("profile", "", "set profile file, for performance test.")
	gXlsFileSize = flag.Int("filesize", TRACE_CSV_FILE_SIZE_MAX, "set xls file size(MB).")

	flag.Parse()

	if *gXlsFileSize <= 0 {
		fmt.Println("filesize too small : ", *gXlsFileSize)
		flag.Usage()
		return
	}
	/* 转换成单位MB */
	*gXlsFileSize = (*gXlsFileSize) * 1024 * 1024

	if *help || *trcDataFile == "" || *trcDescFile == "" {
		flag.Usage()
		return
	}

	jobNum := runtime.NumCPU()
	//	jobNum := 1
	if jobNum > CPU_PROCESS_NUM_MAX {
		jobNum = CPU_PROCESS_NUM_MAX
	}
	runtime.GOMAXPROCS(runtime.NumCPU())

	if *profile != "" {
		pfile, err := os.Create(*profile)
		if err != nil {
			fmt.Println(err)
			return
		}
		pprof.StartCPUProfile(pfile)
		defer pprof.StopCPUProfile()
	}

	gFileDir = createDir(*trcDataFile)
	if gFileDir == "" {
		fmt.Println("createDir error!", gFileDir)
		return
	}

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
	//		fmt.Println(gDescTable)

	gXlsFile = make(map[string]XlsFileCtrlBlock, 1024)
	writeChan := make(chan AsyncWriteCtrlBlock, 512)

	var dataChs [CPU_PROCESS_NUM_MAX]chan []byte //数据发送信道
	var syncChs [CPU_PROCESS_NUM_MAX]chan int    //同步信道,用于控制写入文件的顺序与读取的一致
	dataChans := dataChs[:jobNum]
	syncChans := syncChs[:jobNum]
	for i := 0; i < jobNum; i++ {
		dataChans[i] = make(chan []byte, 4)
		syncChans[i] = make(chan int)
		go parseDataWorker(jobNum, i, dataChans[i], syncChans[:], writeChan)
	}
	go writeFileWorker(writeChan)

	//用于首次触发
	if jobNum > 1 {
		go func() {
			syncChans[0] <- 1
		}()
	}

	loop := 0
	var item []byte
	dataReader := bufio.NewReader(dataFile)
	for {
		itemTmp, err := dataReader.ReadBytes(0xaa)
		if err != nil {
			//			fmt.Println(err)
			break
		}

		if len(item) > 0 {
			item = append(item, itemTmp...)
		} else {
			item = itemTmp[:]
		}

		itemLen := len(item)
		//检查MAGIC
		if itemLen > TRACE_HDR_SIZE && binary.BigEndian.Uint32(item[itemLen-4:itemLen]) == 0xddccbbaa {

			trcType := binary.BigEndian.Uint16(item[8:10])
			trcSize := binary.BigEndian.Uint16(item[10:12])
			structId := strconv.FormatUint(uint64(trcType), 10)
			_, err := gDescTable.StructId2StructNameTable[structId]
			if !err {
				item = item[0:0]
				continue
			}
			if int(trcSize) == itemLen {
				dataChans[loop%jobNum] <- item
				loop++
				if loop%10000 == 0 {
					fmt.Printf("trace data num :%12d\n", loop)
				}
			} else {
				fmt.Printf("HDR : traceSize[%d] error! trcType[%d]\n", trcSize, trcType)
			}
			item = item[0:0]
		}
	}
	fmt.Printf("trace data num :%12d\n", loop)

	//耗时打印
	fmt.Printf("elapsed time : %s\n", time.Since(start))
}
