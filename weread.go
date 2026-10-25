package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"
)

var (
	cachedAccessToken string
	tokenExpiryTime   time.Time
)

const (
	bookShelfInfoURL     = "https://i.weread.qq.com/shelf/sync?synckey=0&teenmode=0&album=1&onlyBookid=0&localBookCount=0"
	bookInfoURL          = "https://i.weread.qq.com/book/info"
	mineReadBookURL      = "https://i.weread.qq.com/mine/readbook?count=100&rating=0&star=0&listType=3&yearRange=0_0&maxidx=%d"
	MaxConcurrentWorkers = 10 // 控制并发协程数量
	readingTimeURL       = "https://i.weread.qq.com/readdata/detail?mode=anually&baseTime=%d&defaultPreferBook=0"
)

////////////// 我的书架返回结构 /////////////////////

type BookShelfInfoResponse struct {
	PureBookCount int               `json:"pureBookCount"`
	BookCount     int               `json:"bookCount"`
	BookProgress  []*BookProgress   `json:"bookProgress"`
	Book          []*BookInResponse `json:"books"`
	Archive       []*Archive        `json:"archive"`

	Errcode int    `json:"errcode"`
	Errmsg  string `json:"errmsg"`
}

type BookProgress struct {
	BookId        string `json:"bookId"`
	Progress      int    `json:"progress"`
	ChapterUid    int    `json:"chapterUid"`
	ChapterOffset int    `json:"chapterOffset"`
	ChapterIdx    int    `json:"chapterIdx"`
	AppId         string `json:"appId"`
	UpdateTime    int    `json:"updateTime"`
	ReadingTime   int64  `json:"readingTime"`
	Synckey       int    `json:"synckey"`
}

type BookInResponse struct {
	BookId                string     `json:"bookId"`
	Title                 string     `json:"title"`
	Author                string     `json:"author"`
	Cover                 string     `json:"cover"`
	Version               int        `json:"version"`
	Format                string     `json:"format"`
	Type                  int        `json:"type"`
	Price                 float64    `json:"price"`
	OriginalPrice         float64    `json:"originalPrice"`
	Soldout               int        `json:"soldout"`
	BookStatus            int        `json:"bookStatus"`
	PayingStatus          int        `json:"payingStatus"`
	PayType               int        `json:"payType"`
	LastChapterCreateTime int        `json:"lastChapterCreateTime"`
	CentPrice             int        `json:"centPrice"`
	Finished              int        `json:"finished"`
	MaxFreeChapter        int        `json:"maxFreeChapter"`
	Free                  int        `json:"free"`
	McardDiscount         int        `json:"mcardDiscount"`
	Ispub                 int        `json:"ispub"`
	ExtraType             int        `json:"extra_type"`
	UpdateTime            int        `json:"updateTime"`
	PublishTime           string     `json:"publishTime"`
	Category              string     `json:"category"`
	HasLecture            int        `json:"hasLecture"`
	LastChapterIdx        int        `json:"lastChapterIdx"`
	Language              string     `json:"language"`
	IsTraditionalChinese  bool       `json:"isTraditionalChinese"`
	HideUpdateTime        bool       `json:"hideUpdateTime"`
	IsEPUBComics          int        `json:"isEPUBComics"`
	IsVerticalLayout      int        `json:"isVerticalLayout"`
	IsShowTTS             int        `json:"isShowTTS"`
	WebBookControl        int        `json:"webBookControl"`
	SelfProduceIncentive  bool       `json:"selfProduceIncentive"`
	IsAutoDownload        int        `json:"isAutoDownload"`
	ShowLectureButton     int        `json:"showLectureButton"`
	Secret                int        `json:"secret"`
	ReadUpdateTime        int        `json:"readUpdateTime"`
	FinishReading         int        `json:"finishReading"`
	Paid                  int        `json:"paid"`
	Categories            []Category `json:"categories"`
}

type Category struct {
	CategoryId    int    `json:"categoryId"`
	SubCategoryId int    `json:"subCategoryId"`
	CategoryType  int    `json:"categoryType"`
	Title         string `json:"title"`
}

type Archive struct {
	ArchiveId    int      `json:"archiveId"`
	Name         string   `json:"name"`
	BookIds      []string `json:"bookIds"`
	Removed      []string `json:"removed"`
	AlbumIds     []string `json:"albumIds"`
	AlbumRemoved []string `json:"albumRemoved"`
}

////////////// 我的书架返回结构 /////////////////////

// 书籍详情响应结构体
type BookDetailResponse struct {
	Intro                string     `json:"intro"`
	TotalWords           int        `json:"totalWords"`
	FinishReading        int        `json:"finishReading"`
	NewRating            int        `json:"newRating"`
	NewRatingCount       int        `json:"newRatingCount"`
	DeepVRating          int        `json:"deepVRating"`
	ShowDeepVRatingLabel int        `json:"showDeepVRatingLabel"`
	Categories           []Category `json:"categories"`
	NewRatingDetail      struct {
		Good     int    `json:"good"`
		Fair     int    `json:"fair"`
		Poor     int    `json:"poor"`
		Recent   int    `json:"recent"`
		DeepV    int    `json:"deepV"`
		MyRating string `json:"myRating"`
		Title    string `json:"title"`
	} `json:"newRatingDetail"`

	Errcode int    `json:"errcode"`
	Errmsg  string `json:"errmsg"`
}

////////////// 我的阅读数据返回结构 /////////////////////

type MineReadBookResponse struct {
	Errcode        int              `json:"errcode"`
	Errmsg         string           `json:"errmsg"`
	Stars          []Star           `json:"stars"`
	Years          []Year           `json:"years"`
	Ratings        []Rating         `json:"ratings"`
	ReadBooks      []*MineReadBook  `json:"readBooks"`
	YearPreference []YearPreference `json:"yearPreference"`
	HasMore        int              `json:"hasMore"`
	TotalCount     int              `json:"totalCount"`
	Synckey        int64            `json:"synckey"`
}

type Star struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Type  int    `json:"type"`
}

type Year struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  int    `json:"type"`
}

type Rating struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Type  int    `json:"type"`
}

type MineReadBook struct {
	BookId           string `json:"bookId"`
	StartReadingTime int64  `json:"startReadingTime"`
	FinishTime       int64  `json:"finishTime"`
	MarkStatus       int    `json:"markStatus"`
	Progress         int    `json:"progress"`
	Readtime         int64  `json:"readtime"`
	Title            string `json:"title"`
	Author           string `json:"author"`
	Cover            string `json:"cover"`
}

type YearPreference struct {
	Year       int    `json:"year"`
	Count      int    `json:"count"`
	Preference string `json:"preference"`
}

////////////// 我的阅读数据返回结构 /////////////////////

////////////// 我的阅读时长返回结构 /////////////////////

type ReadingTimeResponse struct {
	MonthReadTimes map[string]int64 `json:"readTimes"`
	DailyReadTimes map[string]int64 `json:"dailyReadTimes"`
	Errcode        int              `json:"errcode"`
	Errmsg         string           `json:"errmsg"`
}

// 封装HTTP请求逻辑
func GetWeReadBookShelfInfo(reporters ...Reporter) (*BookShelfInfoResponse, error) {
	respBody, err := sendWeReadGET(bookShelfInfoURL, firstReporter(reporters))
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	var data BookShelfInfoResponse
	err = json.Unmarshal(respBody, &data)
	if err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	// 检查是否存在错误响应
	if data.Errcode != 0 {
		return nil, fmt.Errorf("HTTP 请求失败: %s", data.Errmsg)
	}

	return &data, nil
}

// GetAccessToken 刷新并缓存当前进程使用的微信读书 accessToken。
func GetAccessToken() (string, error) {
	return getWeReadAccessToken()
}

// GetBookDetail 获取书籍详情
func GetBookDetail(bookID string, reporters ...Reporter) (*BookDetailResponse, error) {
	// 构建请求URL
	url := fmt.Sprintf("%s?bookId=%s", bookInfoURL, bookID)
	respBody, err := sendWeReadGET(url, firstReporter(reporters))
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	var data BookDetailResponse
	err = json.Unmarshal(respBody, &data)
	if err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	// 检查是否存在错误响应
	if data.Errcode != 0 {
		return nil, fmt.Errorf("HTTP 请求失败: %s", data.Errmsg)
	}

	return &data, nil
}

// BatchGetBookDetail 批量获取书籍详情
func BatchGetBookDetail(bookIDs []string, reporters ...Reporter) (map[string]*BookDetailResponse, error) {
	r := firstReporter(reporters)
	return batchGetBookDetails(bookIDs, func(id string) (*BookDetailResponse, error) { return GetBookDetail(id, r) }, r)
}

func batchGetBookDetails(ids []string, fetch func(string) (*BookDetailResponse, error), r Reporter) (map[string]*BookDetailResponse, error) {
	result := make(map[string]*BookDetailResponse, len(ids))
	var mu sync.Mutex
	var firstErr error
	jobs := make(chan string)
	var wg sync.WaitGroup
	workers := MaxConcurrentWorkers
	if len(ids) < workers {
		workers = len(ids)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				mu.Lock()
				failed := firstErr != nil
				mu.Unlock()
				if failed {
					continue
				}
				detail, err := fetch(id)
				mu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("获取书籍详情失败: %w", err)
					}
				} else if detail == nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("书籍详情响应为空")
					}
				} else {
					result[id] = detail
				}
				done := len(result)
				mu.Unlock()
				if err == nil {
					report(r, Event{Kind: EventProgress, Stage: "book", Task: "details", Done: done, Total: len(ids)})
				}
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if len(result) != len(ids) {
		return nil, fmt.Errorf("书籍详情数量不完整: %d/%d", len(result), len(ids))
	}
	return result, nil
}

// GetMineReadBook 获取我的阅读数据，包含已读和在读书籍的数据
func GetMineReadBook(reporters ...Reporter) ([]*MineReadBook, map[string]*MineReadBook, map[string]*MineReadBook, error) {
	maxidx := 0
	var allBooks []*MineReadBook
	finishBooks := make(map[string]*MineReadBook)
	readingBooks := make(map[string]*MineReadBook)

	for {
		// 构建请求URL
		url := fmt.Sprintf(mineReadBookURL, maxidx)

		respBody, err := sendWeReadGET(url, firstReporter(reporters))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("发送请求失败: %w", err)
		}

		var data MineReadBookResponse
		err = json.Unmarshal(respBody, &data)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("解析JSON失败: %w", err)
		}

		if data.Errcode != 0 {
			return nil, nil, nil, fmt.Errorf("已读书籍接口错误码 %d", data.Errcode)
		}
		// 按markStatus区分已读和在读书籍
		for _, book := range data.ReadBooks {
			allBooks = append(allBooks, book)
			if book.MarkStatus == 4 {
				finishBooks[book.BookId] = book
			} else if book.MarkStatus == 2 {
				readingBooks[book.BookId] = book
			}
		}

		// 判断是否还有更多数据
		if data.HasMore == 0 {
			break
		}

		if len(data.ReadBooks) == 0 {
			return nil, nil, nil, fmt.Errorf("已读书籍分页未前进")
		}
		maxidx += len(data.ReadBooks)
	}

	return allBooks, readingBooks, finishBooks, nil
}

// GetYearReadingTime 获取一整年阅读时长
func GetYearReadingTime(yearOffset int, reporters ...Reporter) (map[time.Time]int64, map[time.Time]int64, error) {
	// 计算baseTime
	var baseTime int64
	if yearOffset == 0 {
		baseTime = 0
	} else {
		if yearOffset < 0 {
			yearOffset = -yearOffset
		}
		currentYear := time.Now().Year()
		year := currentYear - yearOffset
		baseTime = time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local).Unix()
	}

	// 构建请求URL
	url := fmt.Sprintf(readingTimeURL, baseTime)

	respBody, err := sendWeReadGET(url, firstReporter(reporters))
	if err != nil {
		return nil, nil, fmt.Errorf("发送请求失败: %w", err)
	}

	var data ReadingTimeResponse
	err = json.Unmarshal(respBody, &data)
	if err != nil {
		return nil, nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	// 检查是否存在错误响应
	if data.Errcode != 0 {
		return nil, nil, fmt.Errorf("HTTP 请求失败: %s", data.Errmsg)
	}

	// 转换每月阅读时长
	monthReadTimesMap := make(map[time.Time]int64)
	for key, value := range data.MonthReadTimes {
		timestamp, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("解析月份时间戳失败: %w", err)
		}
		t := time.Unix(timestamp, 0)
		monthReadTimesMap[t] = value
	}

	// 转换每日阅读时长
	dailyReadTimesMap := make(map[time.Time]int64)
	for key, value := range data.DailyReadTimes {
		timestamp, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("解析日期时间戳失败: %w", err)
		}
		t := time.Unix(timestamp, 0)
		dailyReadTimesMap[t] = value
	}

	return monthReadTimesMap, dailyReadTimesMap, nil
}
