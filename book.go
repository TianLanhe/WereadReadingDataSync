package main

import (
	"fmt"
	"strings"
	"time"
)

const (
	picUploadConcurrentQPS = 5
	maxPicUploadWaitTime   = time.Hour
)

type Book struct {
	Title      string   `json:"title"`
	Author     string   `json:"author"`
	Cover      string   `json:"cover"`
	CanRead    bool     `json:"canRead"`
	Price      float64  `json:"price"`
	BookId     string   `json:"bookId"`
	Categories []string `json:"categories"`
	ReadTime   int64    `json:"readTime"`  // 阅读时长，秒数
	ShelfName  string   `json:"shelfName"` // 书架分类名称
	Score      float64  `json:"score"`     // 评分，0-100，保留一位小数
	Intro      string   `json:"intro"`
	Words      float64  `json:"words"`      // 书籍字数，单位：万字
	Progress   float64  `json:"progress"`   // 进度，0-1，小数
	FinishTime int64    `json:"finishTime"` // 阅读完成时间，毫秒，没有读完为0
}

func TransferSheetRecordToBook(records []*SheetRecord) ([]*Book, map[string]string) {
	var books []*Book
	bookIdToRecordId := make(map[string]string)
	for _, record := range records {
		bookId := getStringValue(record.Fields, "bookId")
		if bookId == "" {
			continue
		}
		book := &Book{
			BookId:     bookId,
			Title:      getStringValue(record.Fields, "书名"),
			ShelfName:  getStringValue(record.Fields, "书架分类"),
			Price:      getFloat64Value(record.Fields, "价格"),
			Author:     getStringValue(record.Fields, "作者"),
			Categories: getStringSliceValue(record.Fields, "分类"),
			CanRead:    getBoolValue(record.Fields, "是否可读"),
			Score:      getFloat64Value(record.Fields, "评分"),
			ReadTime:   getInt64Value(record.Fields, "阅读时长（秒）"),
			Intro:      getStringValue(record.Fields, "简介"),
			Words:      getFloat64Value(record.Fields, "字数（单位：万字）"),
			Progress:   getFloat64Value(record.Fields, "阅读进度"),
			FinishTime: getInt64Value(record.Fields, "阅读完成时间"),
			// Cover:      getCoverURL(record.Fields["封面"]), 不填充这个字段了，封面也不比较
			// 评分（可视化）不需要处理
			// 阅读时长格式化不需要处理
		}
		books = append(books, book)
		bookIdToRecordId[bookId] = record.RecordID
	}
	return books, bookIdToRecordId
}

func TransferShelfResponseToBook(bookFromWeRead *BookShelfInfoResponse, bookDetailList map[string]*BookDetailResponse, finishBooks map[string]*MineReadBook) map[string]*Book {
	if bookFromWeRead == nil || bookFromWeRead.Book == nil {
		return make(map[string]*Book)
	}

	// 构建bookId到阅读时长的映射
	bookIdToReadingTime := make(map[string]int64)
	for _, progress := range bookFromWeRead.BookProgress {
		bookIdToReadingTime[progress.BookId] = progress.ReadingTime
	}

	// 构建bookId到书架分类的映射
	bookIdToShelfName := make(map[string]string)
	for _, archive := range bookFromWeRead.Archive {
		for _, bookId := range archive.BookIds {
			bookIdToShelfName[bookId] = archive.Name
		}
	}

	// 构建bookId到阅读进度的映射
	bookIdToProgress := make(map[string]float64)
	for _, progress := range bookFromWeRead.BookProgress {
		bookIdToProgress[progress.BookId] = float64(progress.Progress) / 100
	}

	// 构建booId到阅读完成时间的映射
	bookIdToFinishTime := make(map[string]int64)
	for bookID, book := range finishBooks {
		bookIdToProgress[bookID] = 1
		bookIdToFinishTime[bookID] = book.FinishTime * 1000

	}

	weReadBookMap := make(map[string]*Book)
	for _, item := range bookFromWeRead.Book {
		if item.BookId == "" {
			continue
		}
		// 提取Categories中的title
		var categories []string
		for _, cat := range item.Categories {
			if cat.Title != "" {
				categories = append(categories, cat.Title)
			}
		}
		// 没有的话尝试从书本详情获取
		if len(categories) == 0 && bookDetailList[item.BookId] != nil {
			for _, cat := range bookDetailList[item.BookId].Categories {
				if cat.Title != "" {
					categories = append(categories, cat.Title)
				}
			}
		}

		// 从bookDetail中获取简介和字数字段
		intro := ""
		words := 0
		var score float64
		if bookDetailList[item.BookId] != nil {
			bookDetail := bookDetailList[item.BookId]
			intro = bookDetail.Intro
			words = bookDetail.TotalWords
			score = float64(bookDetail.NewRating) / 10
		}

		book := &Book{
			BookId:     item.BookId,
			Title:      item.Title,
			Author:     item.Author,
			Price:      item.Price,
			CanRead:    item.Paid == 1 || item.PayingStatus != 2, // 已经购买过，且不是会员卡才能读的
			Categories: categories,
			ReadTime:   bookIdToReadingTime[item.BookId], // 从映射中获取阅读时长
			ShelfName:  bookIdToShelfName[item.BookId],   // 从映射中获取书架分类
			Score:      score,
			Cover:      item.Cover,
			Intro:      intro,
			Words:      float64(words) / 10000,
			Progress:   bookIdToProgress[item.BookId],   // 从映射中获取阅读进度
			FinishTime: bookIdToFinishTime[item.BookId], // 从映射中获取阅读完成时间
		}

		weReadBookMap[book.BookId] = book
	}
	return weReadBookMap
}

// CalculateBooks 比较书籍列表，返回需要添加、编辑和删除的书籍列表
func CalculateBooks(shelf *BookShelfInfoResponse, details map[string]*BookDetailResponse, finished map[string]*MineReadBook, records []*SheetRecord) ([]map[string]interface{}, []*SheetRecord, []string, error) {
	plan, err := makeBookPlan(shelf, details, finished, records)
	if err != nil {
		return nil, nil, nil, err
	}
	manager := NewCoverUploadManager(picUploadConcurrentQPS, maxPicUploadWaitTime)
	defer manager.Close()
	add, update := materializeBookPlan(plan, manager, nil)
	return add, update, plan.Deletes, nil
}

func coverTokenOrEmpty(coverURL string, getResult func(string) (string, error)) string {
	token, err := getResult(coverURL)
	if err != nil {
		// The caller reports the failure once with its book context.
		return ""
	}
	return token
}

func convertBookToMap(book *Book, coverToken string) map[string]interface{} {
	canRead := "否"
	if book.CanRead {
		canRead = "是"
	}

	hours := book.ReadTime / 3600
	minutes := (book.ReadTime % 3600) / 60
	seconds := book.ReadTime % 60
	var readTimeFormatted string
	if hours > 0 {
		readTimeFormatted += fmt.Sprintf("%d时", hours)
	}
	if minutes > 0 {
		readTimeFormatted += fmt.Sprintf("%d分", minutes)
	}
	if seconds > 0 {
		readTimeFormatted += fmt.Sprintf("%d秒", seconds)
	}

	finishRead := "否"
	if book.FinishTime > 0 {
		finishRead = "是"
	}

	var firstLevelCategories []string
	for _, category := range book.Categories {
		parts := strings.Split(category, "-")
		if len(parts) > 0 {
			firstLevelCategories = append(firstLevelCategories, parts[0])
		}
	}

	ret := map[string]interface{}{
		"bookId":    book.BookId,
		"书名":        book.Title,
		"书架分类":      book.ShelfName,
		"价格":        book.Price,
		"作者":        book.Author,
		"分类":        book.Categories,
		"一级分类":      firstLevelCategories,
		"是否可读":      canRead,
		"评分":        book.Score,
		"阅读时长（秒）":   book.ReadTime,
		"阅读时长（时）":   float64(book.ReadTime) / 3600,
		"阅读时长（分）":   float64(book.ReadTime) / 60,
		"评分（可视化）":   int(book.Score / 10),
		"阅读时长格式化":   readTimeFormatted,
		"字数（单位：万字）": book.Words,
		"简介":        book.Intro,
		"阅读进度":      book.Progress,
		"是否已读完":     finishRead,
	}
	if coverToken != "" {
		ret["封面"] = []map[string]string{{"file_token": coverToken}}
	}

	// 如果已经阅读完成，才设置阅读完成时间字段
	if book.Progress == 1 {
		ret["阅读完成时间"] = book.FinishTime
		ret["已读完年"] = time.UnixMilli(book.FinishTime).Format("2006年")
		ret["已读完年月"] = time.UnixMilli(book.FinishTime).Format("2006年01月")
	}

	return ret
}

// 辅助函数：比较两本书是否相同
func isBookEqual(book1, book2 *Book, _ bool) bool {
	return len(bookChanges(book1, book2)) == 0
}

// 辅助函数：比较两个字符串切片是否相同，不考虑元素顺序
func compareStringSlices(slice1, slice2 []string) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	countMap1 := make(map[string]int)
	countMap2 := make(map[string]int)

	for _, str := range slice1 {
		countMap1[str]++
	}

	for _, str := range slice2 {
		countMap2[str]++
	}

	for key, val1 := range countMap1 {
		if val2, ok := countMap2[key]; !ok || val2 != val1 {
			return false
		}
	}

	return true
}

// 辅助函数：获取布尔值
func getBoolValue(fields map[string]interface{}, key string) bool {
	if value, ok := fields[key]; ok {
		switch v := value.(type) {
		case bool:
			return v
		case string:
			return v == "true" || v == "是"
		case float64:
			return v != 0
		}
	}
	return false
}

// 辅助函数：获取浮点数
func getFloat64Value(fields map[string]interface{}, key string) float64 {
	if value, ok := fields[key]; ok {
		switch v := value.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case string:
			var f float64
			fmt.Sscanf(v, "%f", &f)
			return f
		}
	}
	return 0
}

// 辅助函数：获取整数
func getInt64Value(fields map[string]interface{}, key string) int64 {
	if value, ok := fields[key]; ok {
		switch v := value.(type) {
		case int:
			return int64(v)
		case int64:
			return v
		case float64:
			return int64(v)
		case string:
			var i int64
			fmt.Sscanf(v, "%d", &i)
			return i
		}
	}
	return 0
}

// 辅助函数：获取字符串切片
func getStringSliceValue(fields map[string]interface{}, key string) []string {
	var result []string
	if value, ok := fields[key]; ok {
		switch v := value.(type) {
		case []string:
			return append([]string(nil), v...)
		case []interface{}:
			for _, item := range v {
				if strValue, ok := item.(string); ok {
					result = append(result, strValue)
				}
			}
		case string:
			result = append(result, v)
		}
	}
	return result
}

// 辅助函数：获取字符串值
func getStringValue(fields map[string]interface{}, key string) string {
	if value, ok := fields[key]; ok {
		switch v := value.(type) {
		case string:
			return v
		case map[string]interface{}:
			if strValue, ok := v["text"].(string); ok {
				return strValue
			}
		case []interface{}:
			if len(v) > 0 {
				switch v[0].(type) {
				case string:
					return v[0].(string)
				case map[string]interface{}:
					if strValue, ok := v[0].(map[string]interface{})["text"].(string); ok {
						return strValue
					}
				}
			}
		}
	}
	return ""
}

// UploadCoverToSheet 下载封面图片并上传到飞书表格
func UploadCoverToSheet(cover string) (string, error) {
	manager := NewCoverUploadManager(picUploadConcurrentQPS, maxPicUploadWaitTime)
	defer manager.Close()
	return manager.GetResult(cover)
}

// TransferMineReadBookToShelfResponse 将 MineReadBook 映射到 BookShelfInfoResponse
func TransferMineReadBookToShelfResponse(finishedBooks map[string]*MineReadBook) *BookShelfInfoResponse {
	if len(finishedBooks) == 0 {
		return nil
	}

	var books []*BookInResponse
	var progress []*BookProgress

	// 阅读时长、Paid = 1
	for _, book := range finishedBooks {
		bookInfo := &BookInResponse{
			BookId: book.BookId,
			Title:  book.Title,
			Author: book.Author,
			Cover:  book.Cover,
			Paid:   1, // 看完的都作为已购买看待
		}
		books = append(books, bookInfo)

		progressInfo := &BookProgress{
			BookId:      book.BookId,
			Progress:    100, // 已完成
			ReadingTime: book.Readtime,
		}
		progress = append(progress, progressInfo)
	}

	return &BookShelfInfoResponse{
		Book:         books,
		BookProgress: progress,
	}
}
