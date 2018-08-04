package main

import "bufio"
import "io/ioutil"
import "time"
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
	Field_name string       `json:"field_name"`
	Offset     json.Number  `json:"offset"`
	Size       json.Number  `json:"size"`
	Print_fmt  json.Number  `json:"print_fmt"`
	Show_flag  json.Number  `json:"show_flag"`
	Sub_struct *string      `json:"sub_struct"`
	Array_len  *json.Number `json:"array_len"`
}

type StructDescribe struct {
	Struct_size json.Number            `json:"struct_size"`
	Member_list []StructMemberDescribe `json:"member_list"`
}

type TraceDescribeTable struct {
	Version                           string                    `json:"version"`
	Build_time                        string                    `json:"build_time"`
	Struct_id_and_struct_name_table   map[string]string         `json:"struct_id_and_struct_name_table"`
	Struct_id_and_xls_file_name_table map[string]string         `json:"struct_id_and_xls_file_name_table"`
	Struct_describe_table             map[string]StructDescribe `json:"struct_describe_table"`
}

const gTraceHdrSize = 16

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

	xlsFileName, err := gDescTable.Struct_id_and_xls_file_name_table[structId]
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
	structDesc, err := gDescTable.Struct_describe_table[*structName]
	if !err {
		fmt.Println("can not find struct : ", *structName)
		return
	}
	//	fmt.Println(structDesc)
	fStructName := ""
	if fatherStructName != nil {
		fStructName = *fatherStructName + "."
	}
	for i := 0; i < len(structDesc.Member_list); i++ {
		//		fmt.Println(structDesc.Member_list[i])
		if structDesc.Member_list[i].Show_flag != "1" {
			//			fmt.Println("continue")
			continue
		}

		isSubscriptExist := false //是否存在下标
		arrayLen := 1             //数组长度
		if structDesc.Member_list[i].Array_len != nil {
			alen, err := structDesc.Member_list[i].Array_len.Int64()
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
			fieldName := structDesc.Member_list[i].Field_name + subscript
			if structDesc.Member_list[i].Sub_struct != nil {
				parseStructDesc(structDesc.Member_list[i].Sub_struct, &fieldName, desc)
			} else {
				*desc = *desc + fStructName + fieldName + ","
			}
		}
	}
}

func parseStructData(structName *string, structData []byte, desc *string) {
	structDesc, err := gDescTable.Struct_describe_table[*structName]
	if !err {
		fmt.Println("can not find struct : ", *structName)
		return
	}
	//	fmt.Println(structDesc)
	for i := 0; i < len(structDesc.Member_list); i++ {
		//		fmt.Println(structDesc.Member_list[i])
		if structDesc.Member_list[i].Show_flag != "1" {
			//			fmt.Println("continue")
			continue
		}

		arrayLen := 1 //数组长度
		if structDesc.Member_list[i].Array_len != nil {
			alen, err := structDesc.Member_list[i].Array_len.Int64()
			if err != nil {
				fmt.Println("Array_len.Int64() : ", err)
				return
			}
			arrayLen = int(alen)
		}

		for ii := 0; ii < arrayLen; ii++ {
			mSize, err := structDesc.Member_list[i].Size.Int64()
			if err != nil {
				fmt.Println("Size.Int64() : ", err)
				return
			}
			mOffset, err := structDesc.Member_list[i].Offset.Int64()
			if err != nil {
				fmt.Println("Offset.Int64() : ", err)
				return
			}
			valueBytes := structData[int(mOffset)+int(mSize)*ii:]
			if structDesc.Member_list[i].Sub_struct != nil {
				parseStructData(structDesc.Member_list[i].Sub_struct, valueBytes, desc)
			} else {
				var value uint64 = 0
				switch structDesc.Member_list[i].Size {
				case "1":
					value = uint64(valueBytes[0])
				case "2":
					value = uint64(binary.BigEndian.Uint16(valueBytes[:2]))
				case "4":
					value = uint64(binary.BigEndian.Uint32(valueBytes[:4]))
				case "8":
					value = uint64(binary.BigEndian.Uint64(valueBytes[:8]))
				default:
					fmt.Println("structDesc.Member_list[i].Size error!")
				}

				valueStr := ""
				switch structDesc.Member_list[i].Print_fmt {
				case "0":
					valueStr = fmt.Sprintf("%d", int64(value))
				case "1":
					valueStr = fmt.Sprintf("%d", value)
				case "2":
					valueStr = fmt.Sprintf("%#x", value)
				default:
					fmt.Println("structDesc.Member_list[i].Print_fmt error!")
				}
				*desc = *desc + valueStr + ","
			}
		}
	}
}

func main() {
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

	dataReader := bufio.NewReader(dataFile)
	for {
		item, err := dataReader.ReadBytes(0xaa)
		if err != nil {
			fmt.Println(err)
			break
		}

		itemLen := len(item)
		if itemLen > gTraceHdrSize && item[itemLen-4] == 0xdd && item[itemLen-3] == 0xcc && item[itemLen-2] == 0xbb {

			trcSize := binary.BigEndian.Uint16(item[10:12])
			if int(trcSize) == itemLen {
				sec := binary.BigEndian.Uint32(item[:4])
				usec := binary.BigEndian.Uint32(item[4:8])
				trcType := binary.BigEndian.Uint16(item[8:10])
				structId := fmt.Sprintf("%d", trcType)
				structName := gDescTable.Struct_id_and_struct_name_table[structId]

				file, err := gXlsFile[structId]
				if !err {
					createXlsFile(structId)
					file, err = gXlsFile[structId]
					desc := "time,msec,"
					parseStructDesc(&structName, nil, &desc)
					desc += "\n"
					file.WriteString(desc)
				}

				desc := fmt.Sprintf("%s, %d, ", time.Unix(int64(sec), int64(usec)).Format("060102 15:04:05"), usec/1000)
				parseStructData(&structName, item[12:], &desc)
				desc += "\n"
				file.WriteString(desc)
			}
		}
	}
}
