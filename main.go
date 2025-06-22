package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	// 从微信读书获取已读书籍数据
	_, _, readBooks, err := GetMineReadBook()
	if err != nil {
		fmt.Println(err)
		return
	}

	// 从微信读书获取书架信息
	bookFromWeRead, err := GetWeReadBookShelfInfo()
	if err != nil {
		fmt.Println(err)
		return
	}

	// 提取bookId列表
	var bookIds []string
	seen := make(map[string]bool)
	for _, book := range bookFromWeRead.Book {
		if !seen[book.BookId] {
			bookIds = append(bookIds, book.BookId)
			seen[book.BookId] = true
		}
	}
	for _, book := range readBooks {
		if !seen[book.BookId] {
			bookIds = append(bookIds, book.BookId)
			seen[book.BookId] = true
		}
	}

	// 批量获取书籍详情
	bookDetails, err := BatchGetBookDetail(bookIds)
	if err != nil {
		fmt.Println("批量获取书籍详情失败:", err)
		return
	}

	// 序列化并打印书籍详情内容
	d3, err := json.Marshal(bookDetails)
	if err != nil {
		fmt.Println("序列化书籍详情内容失败:", err)
		return
	}
	fmt.Println("书籍详情内容序列化结果:", string(d3))

	d1, err := json.Marshal(bookFromWeRead)
	if err != nil {
		fmt.Println("序列化微信读书书籍信息失败:", err)
		return
	}
	fmt.Println("微信读书书籍信息序列化结果:", string(d1))

	// 从飞书表格获取已存在的书籍列表
	existedBooks, err := ReadSheetRecords()
	if err != nil {
		fmt.Println("读取已存在的bookId列表失败:", err)
		return
	}

	d2, err := json.Marshal(existedBooks)
	if err != nil {
		fmt.Println("序列化飞书表格书籍信息失败:", err)
		return
	}
	fmt.Println("飞书表格书籍信息序列化结果:", string(d2))

	// 两份数据进行比对，分别计算出要插入的，要编辑的，要删除的记录
	addBooks, updateBooks, deleteBooks, err := CalculateBooks(bookFromWeRead, bookDetails, readBooks, existedBooks)
	if err != nil {
		fmt.Println("计算图书籍差异失败:", err)
		return
	}

	// 添加书籍到多维表格
	if len(addBooks) > 0 {
		if err := BatchAddSheetRecords(addBooks); err != nil {
			fmt.Println("添加书籍失败:", err)
		} else {
			fmt.Println("添加书籍成功，共添加", len(addBooks), "本书籍。")
		}
	}

	// 有变更的书籍更新到多维表格
	if len(updateBooks) > 0 {
		if err := BatchUpdateSheetRecords(updateBooks); err != nil {
			fmt.Println("修改有变更的书籍失败:", err)
		} else {
			fmt.Println("修改有变更的书籍成功，共编辑", len(updateBooks), "本书籍。")
		}
	}

	// 从多维表格删除书籍
	if len(deleteBooks) > 0 {
		if err := BatchDeleteSheetRecords(deleteBooks); err != nil {
			fmt.Println("删除书籍失败:", err)
		} else {
			fmt.Println("删除书籍成功，共删除", len(deleteBooks), "本书籍。")
		}
	}

	// 检查是否有需要修改的书籍内容
	if len(updateBooks)+len(deleteBooks)+len(addBooks) == 0 {
		fmt.Println("无需改动飞书表格的内容！")
	}
}
