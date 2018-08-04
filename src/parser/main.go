package main

import "bufio"
import "io/ioutil"

//import "strings"
import "encoding/binary"
import "encoding/json"

//import "strconv"
import "fmt"
import "os"

type TraceItem struct {
	magicNum  uint32
	sec       uint32
	usec      uint32
	traceType uint16
	traceSize uint16
	data      [4096]uint8
}

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

const traceHdrSize = 16

var descTable TraceDescribeTable

func parseStructDesc(structName *string, fatherStructName *string, desc *string) {
	structDesc, err := descTable.Struct_describe_table[*structName]
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

func main() {
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

	err = json.Unmarshal(descData, &descTable)
	if err != nil {
		fmt.Println("Unmarshal error!", err)
		return
	}
	//	fmt.Println(descTable)

	structName := "S_RarMsg3SchedResult"
	desc := ""
	parseStructDesc(&structName, nil, &desc)
	fmt.Println(desc)

	return

	dataReader := bufio.NewReader(dataFile)
	for {
		item, err := dataReader.ReadBytes(0xaa)
		if err != nil {
			fmt.Println(err)
			break
		}

		itemLen := len(item)
		if itemLen > traceHdrSize && item[itemLen-4] == 0xdd && item[itemLen-3] == 0xcc && item[itemLen-2] == 0xbb {
			//			fmt.Println(item)

			trcSize := binary.BigEndian.Uint16(item[10:12])
			if int(trcSize) == itemLen {
				//				fmt.Println(item)
				sec := binary.BigEndian.Uint32(item[:4])
				usec := binary.BigEndian.Uint32(item[4:8])
				trcType := binary.BigEndian.Uint16(item[8:10])
				trcSize := binary.BigEndian.Uint16(item[10:12])
				fmt.Println(sec, usec, trcType, trcSize)
			}
		}
	}
}
