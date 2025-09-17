package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"
)

func main() {
	mode := flag.String("mode", "all", "book:更新书籍列表, readTime:更新阅读时长数据, all:更新所有数据")
	flag.Parse()

	fmt.Println("启动 WeRead 数据获取，时间：%v，模式:", time.Now(), *mode)

	switch *mode {
	case "all":
		if err := updateReadTimeData(); err != nil {
			fmt.Println("更新阅读时长数据失败:", err)
		}
		if err := updateBookListData(); err != nil {
			fmt.Println("更新书籍列表数据失败:", err)
		}
	case "readTime":
		if err := updateReadTimeData(); err != nil {
			fmt.Println("更新阅读时长数据失败:", err)
		}
	case "book":
		if err := updateBookListData(); err != nil {
			fmt.Println("更新书籍列表数据失败:", err)
		}
	}
}

// N 定义为常量，控制循环读取的次数
const N = 5

func updateReadTimeData() error {
	// 循环调用GetYearReadingTime函数获取阅读时长数据
	allReadingTimes := make(map[time.Time]int64)
	for i := 0; i < N; i++ {
		_, dailyReadingTime, err := GetYearReadingTime(i)
		if err != nil {
			fmt.Println("获取阅读时长数据失败:", err)
			return err
		}
		for day, readTime := range dailyReadingTime {
			if readTime == 0 {
				continue
			}
			allReadingTimes[day] += readTime
		}
	}

	// 打印合并后的阅读时长数据
	d1, err := json.Marshal(allReadingTimes)
	if err != nil {
		fmt.Println("序列化阅读时长数据失败:", err)
		return err
	}
	fmt.Println("合并后的阅读时长数据:", string(d1))

	// 从飞书多维表格读取已记录的阅读时长数据
	existedReadingTimes, err := ReadSheetRecords(sheetAppID, readTimeTableID)
	if err != nil {
		fmt.Println("读取已记录的阅读时长数据失败:", err)
		return err
	}

	// 打印飞书表格中的数据
	d2, err := json.Marshal(existedReadingTimes)
	if err != nil {
		fmt.Println("序列化飞书表格阅读时长数据失败:", err)
		return err
	}
	fmt.Println("飞书表格阅读时长数据:", string(d2))

	// 调用CalculateReadTime计算需要添加、编辑、删除的内容
	addReadTimes, updateReadTimes, deleteReadTimes, err := CalculateReadTime(allReadingTimes, existedReadingTimes)
	if err != nil {
		fmt.Println("计算阅读时长差异失败:", err)
		return err
	}

	// 添加阅读时长记录到多维表格
	if len(addReadTimes) > 0 {
		if err := BatchAddSheetRecords(sheetAppID, readTimeTableID, addReadTimes); err != nil {
			fmt.Println("添加阅读时长记录失败:", err)
		} else {
			fmt.Println("添加阅读时长记录成功，共添加", len(addReadTimes), "条记录。")
		}
	}

	// 编辑有变更的阅读时长记录到多维表格
	if len(updateReadTimes) > 0 {
		if err := BatchUpdateSheetRecords(sheetAppID, readTimeTableID, updateReadTimes); err != nil {
			fmt.Println("修改有变更的阅读时长记录失败:", err)
		} else {
			fmt.Println("修改有变更的阅读时长记录成功，共编辑", len(updateReadTimes), "条记录。")
		}
	}

	// 从多维表格删除阅读时长记录
	if len(deleteReadTimes) > 0 {
		if err := BatchDeleteSheetRecords(sheetAppID, readTimeTableID, deleteReadTimes); err != nil {
			fmt.Println("删除阅读时长记录失败:", err)
		} else {
			fmt.Println("删除阅读时长记录成功，共删除", len(deleteReadTimes), "条记录。")
		}
	}

	// 检查是否有需要修改的阅读时长记录
	if len(updateReadTimes)+len(deleteReadTimes)+len(addReadTimes) == 0 {
		fmt.Println("无需改动飞书表格的阅读时长记录内容！")
	}

	return nil
}

func updateBookListData() error {
	// 从微信读书读取已读书籍数据
	_, _, readBooks, err := GetMineReadBook()
	if err != nil {
		fmt.Println(err)
		return err
	}

	// 从微信读书获取书架信息
	bookFromWeRead, err := GetWeReadBookShelfInfo()
	if err != nil {
		fmt.Println(err)
		return err
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
		return err
	}

	// 序列化并打印书籍详情内容
	d3, err := json.Marshal(bookDetails)
	if err != nil {
		fmt.Println("序列化书籍详情内容失败:", err)
		return err
	}
	fmt.Println("书籍详情内容序列化结果:", string(d3))

	d1, err := json.Marshal(bookFromWeRead)
	if err != nil {
		fmt.Println("序列化微信读书书籍信息失败:", err)
		return err
	}
	fmt.Println("微信读书书籍信息序列化结果:", string(d1))

	// 从飞书表格获取已存在的书籍列表
	existedBooks, err := ReadSheetRecords(sheetAppID, bookListTableID)
	if err != nil {
		fmt.Println("读取已存在的bookId列表失败:", err)
		return err
	}

	d2, err := json.Marshal(existedBooks)
	if err != nil {
		fmt.Println("序列化飞书表格书籍信息失败:", err)
		return err
	}
	fmt.Println("飞书表格书籍信息序列化结果:", string(d2))

	// 两份数据进行比对，分别计算出要插入的，要编辑的，要删除的记录
	addBooks, updateBooks, deleteBooks, err := CalculateBooks(bookFromWeRead, bookDetails, readBooks, existedBooks)
	if err != nil {
		fmt.Println("计算图书籍差异失败:", err)
		return err
	}

	// 添加书籍到多维表格
	if len(addBooks) > 0 {
		if err := BatchAddSheetRecords(sheetAppID, bookListTableID, addBooks); err != nil {
			fmt.Println("添加书籍失败:", err)
		} else {
			fmt.Println("添加书籍成功，共添加", len(addBooks), "本书籍。")
		}
	}

	// 有变更的书籍更新到多维表格
	if len(updateBooks) > 0 {
		if err := BatchUpdateSheetRecords(sheetAppID, bookListTableID, updateBooks); err != nil {
			fmt.Println("修改有变更的书籍失败:", err)
		} else {
			fmt.Println("修改有变更的书籍成功，共编辑", len(updateBooks), "本书籍。")
		}
	}

	// 从多维表格删除书籍
	if len(deleteBooks) > 0 {
		if err := BatchDeleteSheetRecords(sheetAppID, bookListTableID, deleteBooks); err != nil {
			fmt.Println("删除书籍失败:", err)
		} else {
			fmt.Println("删除书籍成功，共删除", len(deleteBooks), "本书籍。")
		}
	}

	// 检查是否有需要修改的书籍内容
	if len(updateBooks)+len(deleteBooks)+len(addBooks) == 0 {
		fmt.Println("无需改动飞书表格的内容！")
	}
	return nil
}
