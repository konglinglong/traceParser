package main

import "bufio"
import "io/ioutil"
import "time"
import "runtime"
import "strings"
import "encoding/binary"
import "encoding/json"
import "path/filepath"
import "fmt"
import "os"

//trace header
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

const TRACE_HDR_SIZE = 16
const CPU_PROCESS_NUM_MAX = 64

var gDescTable TraceDescribeTable
var gFileDir string = ""

var gXlsFile map[string]*os.File

func createDir(fileName string) string {

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

func parseStructDesc(structName *string, fatherStructName *string, desc *string) {
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
				parseStructDesc(structDesc.MemberList[i].SubStruct, &fieldName, desc)
			} else {
				*desc = *desc + fStructName + fieldName + ","
			}
		}
	}
}

func parseStructData(structName *string, structData []byte, desc *string) {
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
				parseStructData(structDesc.MemberList[i].SubStruct, valueBytes, desc)
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
				*desc = *desc + valueStr + ","
			}
		}
	}
}

func worker(jobNum int, jobId int, dataChan <-chan []byte, syncChans []chan int) {
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

		structDesc, err := gDescTable.StructDescribeTable[structName]
		if !err {
			fmt.Println("can not find struct : ", structName)
			if jobNum > 1 {
				<-syncChans[jobId%jobNum]
				syncChans[(jobId+1)%jobNum] <- 1
			}
			continue
		}
		structSize, err1 := structDesc.StructSize.Int64()
		if err1 != nil {
			fmt.Println("StructSize.Int64() : ", err1)
			if jobNum > 1 {
				<-syncChans[jobId%jobNum]
				syncChans[(jobId+1)%jobNum] <- 1
			}
			continue
		}
		if trcSize-TRACE_HDR_SIZE != uint16(structSize) {
			fmt.Printf("ERROR : struct(%s) trcDataHdrSize[%d] != trcDescSize[%d], check describe table version!\n", structName, trcSize-TRACE_HDR_SIZE, structSize)
			if jobNum > 1 {
				<-syncChans[jobId%jobNum]
				syncChans[(jobId+1)%jobNum] <- 1
			}
			continue
		}

		desc := fmt.Sprintf("%s, %d, ", time.Unix(int64(sec), int64(usec)).Format("060102 15:04:05"), usec/1000)
		parseStructData(&structName, item[12:], &desc)
		desc += "\n"

		file, _ := gXlsFile[structId]

		if jobNum > 1 {
			<-syncChans[jobId%jobNum]
			file.WriteString(desc)
			syncChans[(jobId+1)%jobNum] <- 1
		} else {
			file.WriteString(desc)
		}
	}
}

func main() {

	start := time.Now()

	cpuNum := runtime.NumCPU()
	if cpuNum > CPU_PROCESS_NUM_MAX {
		cpuNum = CPU_PROCESS_NUM_MAX
	}
	if cpuNum > 1 {
		cpuNum = cpuNum - 1
		runtime.GOMAXPROCS(cpuNum)
	}

	gFileDir = createDir("trc_data.dat")
	if gFileDir == "" {
		fmt.Println("createDir error!", gFileDir)
		return
	}

	gXlsFile = make(map[string]*os.File, 512)

	dataFile, err := os.Open("trc_data.dat")
	if err != nil {
		fmt.Println("open trc_data.dat error!", err)
		return
	}
	defer dataFile.Close()

	descData, err := ioutil.ReadFile("trc_data_describe_table.txt")
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

	var dataChs [CPU_PROCESS_NUM_MAX]chan []byte
	var syncChs [CPU_PROCESS_NUM_MAX]chan int
	dataChans := dataChs[:cpuNum]
	syncChans := syncChs[:cpuNum]
	for i := 0; i < cpuNum; i++ {
		dataChans[i] = make(chan []byte)
		syncChans[i] = make(chan int)
		go worker(cpuNum, i, dataChans[i], syncChans[:])
	}

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
				if !err {
					createXlsFile(structId)
					file, err = gXlsFile[structId]
					desc := "time,msec,"
					parseStructDesc(&structName, nil, &desc)
					desc += "\n"
					file.WriteString(desc)
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

	fmt.Printf("elapsed time : %s\n", time.Since(start))
}
