package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tysonmote/gommap"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"
)

type TraceItem struct {
	magicNum  uint32
	sec       uint32
	usec      uint32
	traceType uint16
	traceSize uint16
}

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

type CsvFileInfo struct {
	file *os.File
	wter *bufio.Writer
	size int
	num  int
}

type TraceRawData struct {
	stopInd bool
	data    []byte
}

type TraceJsonData struct {
	stopInd    bool
	structId   string
	structName string
	data       []byte
}

const (
	TRACE_MAGIC             uint32 = 0xddccbbaa
	TRACE_HDR_SIZE                 = 16
	CPU_PROCESS_NUM_MAX            = 64
	TRACE_CSV_FILE_SIZE_MAX        = 10 //10M
)

type TraceParser struct {
	help        *bool
	trcDataFile *string
	trcDescFile *string
	profile     *string
	xlsFileSize *int

	fileDir   string
	descTable TraceDescribeTable
	csvFile   map[string]CsvFileInfo

	readCount       int
	readHdrErrCount int

	jobNum    int
	dataChs   [CPU_PROCESS_NUM_MAX]chan TraceRawData
	writeChs  [CPU_PROCESS_NUM_MAX]chan TraceJsonData
	wg        sync.WaitGroup
	byteOrder binary.ByteOrder
}

func NewParser() *TraceParser {
	return &TraceParser{}
}

func (parser *TraceParser) ParseFlag() error {
	parser.help = flag.Bool("h", false, "show this help")
	parser.trcDataFile = flag.String("data", "", "set trace data file.")
	parser.trcDescFile = flag.String("desc", "", "set trace describe file.")
	parser.profile = flag.String("profile", "", "set profile file, for performance test.")
	parser.xlsFileSize = flag.Int("filesize", TRACE_CSV_FILE_SIZE_MAX, "set xls file size(MB).")

	flag.Parse()

	if *parser.help {
		return fmt.Errorf("")
	}

	if *parser.xlsFileSize <= 0 {
		return fmt.Errorf("file size too small! size[%d]", *parser.xlsFileSize)
	}
	/* 转换成单位MB */
	*parser.xlsFileSize = (*parser.xlsFileSize) * 1024 * 1024

	if *parser.trcDataFile == "" {
		return fmt.Errorf("trace data file list is empty")
	}

	if *parser.trcDescFile == "" {
		return fmt.Errorf("trace describe file list is empty")
	}

	return nil
}

func (parser *TraceParser) Parse() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	parser.jobNum = runtime.NumCPU()
	if parser.jobNum > CPU_PROCESS_NUM_MAX {
		parser.jobNum = CPU_PROCESS_NUM_MAX
	}

	fmt.Println("jobNum : ", parser.jobNum)

	err1 := parser.CreateDir(*parser.trcDataFile)
	if err1 != nil {
		fmt.Println("CreateDir : ", err1)
		return
	}

	dataFile, err2 := os.Open(*parser.trcDataFile)
	if err2 != nil {
		fmt.Println(err2)
		return
	}
	defer dataFile.Close()

	data, err3 := gommap.Map(dataFile.Fd(), gommap.PROT_READ, gommap.MAP_PRIVATE)
	if err3 != nil {
		fmt.Println(err3)
		return
	}

	// data, err3 := ioutil.ReadFile(*parser.trcDataFile)
	// if err3 != nil {
	// 	fmt.Print(err3)
	// 	return
	// }

	parser.JudgeByteOrder(data)

	descData, err3 := ioutil.ReadFile(*parser.trcDescFile)
	if err3 != nil {
		fmt.Println(err3)
		return
	}

	err4 := json.Unmarshal(descData, &parser.descTable)
	if err4 != nil {
		fmt.Println(err4)
		return
	}

	parser.csvFile = make(map[string]CsvFileInfo, 1024)

	for i := 0; i < parser.jobNum; i++ {
		parser.dataChs[i] = make(chan TraceRawData, 4096)
		parser.writeChs[i] = make(chan TraceJsonData, 4096)
		parser.wg.Add(1)
		go parser.ParseDataWorker(i)
	}
	parser.wg.Add(1)
	go parser.WriteFileWorker()

	loop := 0
	dataSize := len(data)
	readOffset := 0
	hdrErrCount := 0
	var rawData TraceRawData
	for {
		if readOffset+TRACE_HDR_SIZE > dataSize {
			break
		}
		if parser.byteOrder.Uint32(data[readOffset:readOffset+4]) == TRACE_MAGIC {
			traceSize := int(parser.byteOrder.Uint16(data[readOffset+14 : readOffset+16]))
			if readOffset+traceSize > dataSize {
				readOffset += 4
				hdrErrCount++
				continue
			}
			if readOffset+traceSize <= dataSize-4 {
				if parser.byteOrder.Uint32(data[readOffset+traceSize:readOffset+traceSize+4]) != TRACE_MAGIC {
					readOffset += 4
					hdrErrCount++
					continue
				}
			}
			rawData.stopInd = false
			rawData.data = data[readOffset : readOffset+traceSize]
			parser.dataChs[loop%parser.jobNum] <- rawData
			readOffset += traceSize

			loop++
			if loop%10000 == 0 {
				fmt.Printf("readCount :%12d\n", loop)
			}
		} else {
			readOffset++
			hdrErrCount++
		}
	}

	rawData.stopInd = true
	rawData.data = nil
	for i := 0; i < parser.jobNum; i++ {
		parser.dataChs[(i+loop)%parser.jobNum] <- rawData
	}

	parser.readCount = loop
	parser.readHdrErrCount = hdrErrCount

	fmt.Printf("readCount :%12d\n", loop)
	// fmt.Printf("readCount :%12d, headerErrorCount :%d\n", loop, hdrErrCount)
}

//创建文件夹
func (parser *TraceParser) CreateDir(fileName string) error {

	f, err1 := os.Stat(fileName)
	if err1 != nil {
		return err1
	}
	if f.IsDir() {
		return fmt.Errorf("%q is not a file", fileName)
	}

	fileNameBase := filepath.Base(fileName)
	fileNameExt := filepath.Ext(fileName)
	if fileNameExt != "" {
		fileNameBase = strings.TrimSuffix(fileNameBase, fileNameExt)
	} else {
		fileNameBase = fileNameBase + "_"
	}

	//使用数据文件路径
	dir, err2 := filepath.Abs(filepath.Dir(fileName))
	if err2 != nil {
		return err2
	}
	dir = filepath.Join(dir, fileNameBase)

	err3 := os.Mkdir(dir, os.ModePerm)
	if err3 != nil {
		return err3
	}

	parser.fileDir = dir

	return nil
}

//判断数据的大小端
func (parser *TraceParser) JudgeByteOrder(data []byte) {
	dataLen := 10 * 1024 * 1024 //只读前10M来判断
	if dataLen > len(data) {
		dataLen = len(data)
	}
	readOffset := 0
	counter := 0

	for {
		if binary.BigEndian.Uint32(data[readOffset:readOffset+4]) == TRACE_MAGIC {
			counter++
		}
		readOffset++
		if readOffset+4 > dataLen {
			break
		}
	}
	readOffset = 0
	for {
		if binary.LittleEndian.Uint32(data[readOffset:readOffset+4]) == TRACE_MAGIC {
			counter--
		}
		readOffset++
		if readOffset+4 > dataLen {
			break
		}
	}

	if counter >= 0 {
		parser.byteOrder = &binary.BigEndian
		fmt.Printf("\ndata is BigEndian. magic[0x%x]\n\n", TRACE_MAGIC)
	} else {
		parser.byteOrder = &binary.LittleEndian
		fmt.Printf("\ndata is LittleEndian. magic[0x%x]\n\n", TRACE_MAGIC)
	}
}

//解析跟踪数据线程
func (parser *TraceParser) ParseDataWorker(jobId int) {

	var item TraceItem
	var jsonData TraceJsonData

	for {
		rawData, ok := <-parser.dataChs[jobId]
		if !ok || rawData.stopInd {
			jsonData = TraceJsonData{true, "", "", nil}
			parser.writeChs[jobId] <- jsonData
			break
		}

		item.magicNum = parser.byteOrder.Uint32(rawData.data[0:4])
		item.sec = parser.byteOrder.Uint32(rawData.data[4:8])
		item.usec = parser.byteOrder.Uint32(rawData.data[8:12])
		item.traceType = (uint16(rawData.data[12]) << 8) | uint16(rawData.data[13])
		item.traceSize = parser.byteOrder.Uint16(rawData.data[14:16])

		structId, structName, err := parser.CheckTrcHeader(&item)
		if err == nil {
			buffer := bytes.NewBufferString(time.Unix(int64(item.sec), int64(item.usec)).Format("060102 15:04:05"))
			buffer.WriteString(",")
			buffer.WriteString(strconv.FormatUint(uint64(item.usec/1000), 10))
			buffer.WriteString(",")
			parser.ParseStructData(&structName, rawData.data[TRACE_HDR_SIZE:], buffer)
			buffer.WriteString("\n")

			jsonData = TraceJsonData{false, structId, structName, buffer.Bytes()}
		} else {
			fmt.Println("CheckTrcHeader : ", err)
			jsonData = TraceJsonData{false, "", "", nil}
		}

		parser.writeChs[jobId] <- jsonData
	}
	parser.wg.Done()
}

//写文件线程
func (parser *TraceParser) WriteFileWorker() {
	loop := 0
	for {
		jsonData, ok := <-parser.writeChs[loop%parser.jobNum]
		if !ok || jsonData.stopInd {
			break
		}
		if jsonData.data == nil {
			continue
		}
		loop++

		csvFileInfo, err := parser.csvFile[jsonData.structId]
		if !err || csvFileInfo.file == nil {
			parser.CreateCsvFile(jsonData.structId)

			csvFileInfo, _ = parser.csvFile[jsonData.structId]
			//写入表头
			buffer := bytes.NewBufferString("time,ms,")
			parser.ParseStructDesc(&jsonData.structName, nil, buffer)
			buffer.WriteString("\n")
			csvFileInfo.wter.Write(buffer.Bytes())
		}

		csvFileInfo.wter.Write(jsonData.data)
		csvFileInfo.size += len(jsonData.data)
		if csvFileInfo.size >= *parser.xlsFileSize {
			csvFileInfo.wter.Flush()
			csvFileInfo.wter = nil
			csvFileInfo.file.Close()
			csvFileInfo.file = nil
			csvFileInfo.size = 0
		}
		parser.csvFile[jsonData.structId] = csvFileInfo
	}

	for _, value := range parser.csvFile {
		value.wter.Flush()
		value.wter = nil
		value.file.Close()
		value.file = nil
	}

	fmt.Printf("\nreadCount : %d, writeCount : %d, headerErrorCount : %d\n\n", parser.readCount, loop, parser.readHdrErrCount)

	parser.wg.Done()
}

//检查数据头部是否与结构体描述表里面的定义一致
func (parser *TraceParser) CheckTrcHeader(item *TraceItem) (sid string, sn string, err error) {

	structId := strconv.FormatUint(uint64(item.traceType), 10)

	structName, err1 := parser.descTable.StructId2StructNameTable[structId]
	if !err1 {
		return "", "", fmt.Errorf("structId[%q] not found", structId)
	}

	structDesc, err2 := parser.descTable.StructDescribeTable[structName]
	if !err2 {
		return "", "", fmt.Errorf("structName[%q] not found", structName)
	}

	if item.traceSize-TRACE_HDR_SIZE != uint16(structDesc.StructSize) {
		return "", "", fmt.Errorf("struct(%s) : data header size[%d] != desc header size[%d], check describe table version",
			structName, item.traceSize-TRACE_HDR_SIZE, structDesc.StructSize)
	}

	return structId, structName, nil
}

//解析跟踪数据
func (parser *TraceParser) ParseStructData(structName *string, structData []byte, buffer *bytes.Buffer) {
	structDesc, err := parser.descTable.StructDescribeTable[*structName]
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
				parser.ParseStructData(&structDesc.MemberList[i].SubStruct, valueBytes, buffer)
			} else {
				var valueI64 int64 = 0
				var valueU64 uint64 = 0
				switch structDesc.MemberList[i].Size {
				case 1:
					valueU8 := uint8(valueBytes[0])
					valueU64 = uint64(valueU8)
					valueI64 = int64(int8(valueU8))
				case 2:
					valueU16 := parser.byteOrder.Uint16(valueBytes[:2])
					valueU64 = uint64(valueU16)
					valueI64 = int64(int16(valueU16))
				case 4:
					valueU32 := parser.byteOrder.Uint32(valueBytes[:4])
					valueU64 = uint64(valueU32)
					valueI64 = int64(int32(valueU32))
				case 8:
					valueU64 = parser.byteOrder.Uint64(valueBytes[:8])
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

//创建CSV文件
func (parser *TraceParser) CreateCsvFile(structId string) error {

	xlsFileName, err := parser.descTable.StructId2XlsFileNameTable[structId]
	if !err {
		return fmt.Errorf("structId(%q) not found", structId)
	}

	csvFileInfo := CsvFileInfo{nil, nil, 0, 0}
	csvFileInfo, _ = parser.csvFile[structId]

	fullPathName := fmt.Sprintf("%s/%s(%d).csv", parser.fileDir, xlsFileName, csvFileInfo.num)

	file, err1 := os.OpenFile(fullPathName, os.O_RDWR|os.O_CREATE, 0766)
	if err1 != nil {
		return err1
	}

	csvFileInfo.file = file
	csvFileInfo.wter = bufio.NewWriter(file)
	csvFileInfo.num++

	parser.csvFile[structId] = csvFileInfo

	return nil
}

//解析跟踪结构体描述
func (parser *TraceParser) ParseStructDesc(structName *string, fatherStructName *string, buffer *bytes.Buffer) {
	structDesc, err := parser.descTable.StructDescribeTable[*structName]
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
				parser.ParseStructDesc(&structDesc.MemberList[i].SubStruct, &fieldName, buffer)
			} else {
				buffer.WriteString(fStructName)
				buffer.WriteString(fieldName)
				buffer.WriteString(",")
			}
		}
	}
}

func (parser *TraceParser) WaitRoutines() {
	parser.wg.Wait()
}

func main() {
	start := time.Now()

	parser := NewParser()
	err := parser.ParseFlag()
	if err != nil {
		fmt.Println("Options : ", err)
		flag.Usage()
		return
	}

	if *parser.profile != "" {
		pfile, err := os.Create(*parser.profile)
		if err != nil {
			fmt.Println(err)
			return
		}
		pprof.StartCPUProfile(pfile)
		defer pprof.StopCPUProfile()
	}

	parser.Parse()
	parser.WaitRoutines()

	//耗时打印
	fmt.Printf("elapsed time : %s\n", time.Since(start))
}
