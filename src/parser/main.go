package main

//import "bufio"
//import "bytes"
//import "encoding/binary"
import "encoding/json"
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
	Field_name string      `json:"field_name"`
	Offset     json.Number `json:"offset"`
	Size       json.Number `json:"size"`
	Print_fmt  json.Number `json:"print_fmt"`
	Show_flag  json.Number `json:"show_flag"`
	Sub_struct string      `json:"sub_struct"`
	Array_len  json.Number `json:"array_len"`
}

type StructDescribe struct {
	Struct_size json.Number            `json:"struct_size"`
	Member_list []StructMemberDescribe `json:"member_list"`
}

type TraceDataDescribe struct {
	Version                           string                    `json:"version"`
	Build_time                        string                    `json:"build_time"`
	Struct_id_and_struct_name_table   map[string]string         `json:"struct_id_and_struct_name_table"`
	Struct_id_and_xls_file_name_table map[string]string         `json:"struct_id_and_xls_file_name_table"`
	Struct_describe_table             map[string]StructDescribe `json:"struct_describe_table"`
}

const traceHeadSize = 16

func main() {
	//	file, err := os.Open("trc_(17-04-13 18.02.25)(0).trc")
	file, err := os.Open("trc_data_describe_table.txt")
	if err != nil {
		fmt.Println("open error!", err)
		return
	}
	defer file.Close()

	//	count := 0

	//	for {
	//		//		buffer := make([]byte, 2048, 4096)
	//		var traceItem TraceItem
	//		for {
	//			var word [4]byte
	//			var halfword [2]byte
	//
	//			n, err := file.Read(word[:])
	//			if err != nil || n < len(word[:]) {
	//				fmt.Println("file.Read failed:", err)
	//				return
	//			}
	//
	//			traceItem.magicNum = binary.BigEndian.Uint32(word[:])
	//
	//			if traceItem.magicNum != 0xddccbbaa {
	//				file.Seek(-3, os.SEEK_CUR)
	//				continue
	//			} else {
	//				n, err := file.Read(word[:])
	//				if err != nil || n < len(word[:]) {
	//					fmt.Println("file.Read failed:", err)
	//					return
	//				}
	//				traceItem.sec = binary.BigEndian.Uint32(word[:])
	//
	//				n, err = file.Read(word[:])
	//				if err != nil || n < len(word[:]) {
	//					fmt.Println("file.Read failed:", err)
	//					return
	//				}
	//				traceItem.usec = binary.BigEndian.Uint32(word[:])
	//
	//				n, err = file.Read(halfword[:])
	//				if err != nil || n < len(halfword[:]) {
	//					fmt.Println("file.Read failed:", err)
	//					return
	//				}
	//				traceItem.traceType = binary.BigEndian.Uint16(halfword[:])
	//
	//				n, err = file.Read(halfword[:])
	//				if err != nil || n < len(halfword[:]) {
	//					fmt.Println("file.Read failed:", err)
	//					return
	//				}
	//				traceItem.traceSize = binary.BigEndian.Uint16(halfword[:])
	//
	//				if traceItem.traceSize > 1500 {
	//					fmt.Printf("msg size[%d] err!\n", traceItem.traceSize)
	//					continue
	//				}
	//
	//				n, err = file.Read(traceItem.data[0:traceItem.traceSize])
	//				if err != nil || n < len(halfword) {
	//					fmt.Println("file.Read failed:", err)
	//					return
	//				}
	//
	//				break
	//			}
	//		}
	//
	//		fmt.Printf("magic_num = %x\n", traceItem.magicNum)
	//		json.Unmarshal(data, v)
	//	}

	var data [1024 * 1024]byte
	n, err := file.Read(data[:])
	if err != nil {
		fmt.Println("file.Read failed:", n, err)
		return
	}

	//	var traceItem TraceItem
	//	fmt.Println(traceItem)

	//	b := []byte(`{"Name":"Wednesday","Age":6,"Parents":["Gomez","Morticia"]}`)
	//	var f interface{}
	var m TraceDataDescribe
	json.Unmarshal(data[:n], &m)
	//	fmt.Printf("%+v", m)
	//	fmt.Println(m)
	fmt.Println(m.Struct_id_and_struct_name_table["1287"])
	fmt.Println(m)
}
