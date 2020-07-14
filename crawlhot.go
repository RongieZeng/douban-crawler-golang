package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"os"

	"github.com/PuerkitoBio/goquery"
)

var lockObj = make(chan struct{}, 3)
var lockMap = make(chan struct{}, 1)
var lockForeach = make(chan struct{}, 1)
var crawlResult = make(map[string]*book)

type book struct {
	Title  string
	Score  float64
	People int64
	Link   string
}

type searchCriteria struct {
	Tag    string
	Score  float64
	People int64
}

func main() {
	var criteriaList []searchCriteria = []searchCriteria{
		{Tag: "生活", Score: 8.5, People: 2000},
		{Tag: "生活", Score: 8.5, People: 5000},
		{Tag: "生活", Score: 9, People: 2000},
		{Tag: "生活", Score: 9, People: 10000},
		{Tag: "科技", Score: 8.5, People: 2000},
		{Tag: "科技", Score: 8.5, People: 5000},
		{Tag: "科技", Score: 9, People: 2000},
		{Tag: "科技", Score: 9, People: 10000},
		{Tag: "文化", Score: 8.5, People: 2000},
		{Tag: "文化", Score: 8.5, People: 5000},
		{Tag: "文化", Score: 9, People: 2000},
		{Tag: "文化", Score: 9, People: 10000},
		{Tag: "经管", Score: 8.5, People: 2000},
		{Tag: "经管", Score: 8.5, People: 5000},
		{Tag: "经管", Score: 9, People: 2000},
		{Tag: "经管", Score: 9, People: 10000},
	}

	start := time.Now()
	crawlResult = make(map[string]*book)
	for _, criteria := range criteriaList {
		lockForeach <- struct{}{}
		crawlResult = make(map[string]*book)
		linkList := getTagLinks(criteria.Tag)

		var wg sync.WaitGroup
		wg.Add(len(linkList))
		for _, link := range linkList {
			go parseBookDesc(link, criteria, &wg)
		}

		wg.Wait()

		fileName := fmt.Sprintf("%v-%v-%v.csv", criteria.Tag, criteria.Score, criteria.People)
		fmt.Printf("----------------开始保存数据:%v\n", fileName)

		resultStr := "标题,评分,人数,链接\n"
		for title, bookObj := range crawlResult {
			resultStr += fmt.Sprintf("%v,%v,%v,%v\n", title, bookObj.Score, bookObj.People, bookObj.Link)
		}

		if fileObj, err := os.OpenFile("./"+fileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err == nil {
			defer fileObj.Close()
			_, err = fileObj.WriteString(resultStr)
			if err != nil {
				fmt.Printf("%s", err)
			} else {
				fmt.Printf("----------------数据保存成功:%v\n", fileName)
			}
		}
		<-lockForeach
	}

	end := time.Now()
	fmt.Println(end.Sub(start).Milliseconds())
	fmt.Println("数据抓取结束")
}

func getTagLinks(bigTag string) []string {
	baseURL := "https://book.douban.com"
	startURL := baseURL + "/tag/?view=type&icn=index-sorttags-all"
	doc := getDoc(startURL)

	linkList := make([]string, 0)
	doc.Find("#content > div > div.article > div:nth-child(2) a[name=" + bigTag + "]").Next().Find("a").Each(func(index int, ele *goquery.Selection) {
		link, ok := ele.Attr("href")
		if ok {
			linkList = append(linkList, baseURL+link)
		}
	})

	return linkList
}

func parseBookDesc(url string, criteria searchCriteria, wg *sync.WaitGroup) {
	defer func() { wg.Done() }()
	doc := getDoc(url)
	pageCount, err := strconv.Atoi(doc.Find("#subject_list > div.paginator >a").Last().Text())
	if err != nil {
		fmt.Printf("url:%s , 页码转换错误: %s", url, err)
		pageCount = 1
	}

	c := make(chan struct{})

	for i := 0; i < pageCount; i++ {
		go func(i int) {
			lockObj <- struct{}{}
			link := fmt.Sprintf("%s?start=%v&type=T", url, i*20)
			docListPage := getDoc(link)

			docListPage.Find("#subject_list > ul > li").Each(func(index int, ele *goquery.Selection) {
				title := ele.Find(".info>h2>a").Text()
				title = strings.Replace(strings.Replace(title, "\n", "", -1), " ", "", -1)
				link, _ := ele.Find(".info>h2>a").Attr("href")

				score, scoreErr := strconv.ParseFloat(ele.Find(".rating_nums").Text(), 64)
				pNumsStr := ele.Find(".pl").Text()
				pNumsStr = strings.Replace(pNumsStr, " ", "", -1)
				pNumsStr = strings.Replace(pNumsStr, "(", "", -1)
				pNumsStr = strings.Replace(pNumsStr, "\n", "", -1)
				pNumsStr = strings.Replace(pNumsStr, "人评价)", "", -1)
				pNums, pNumsErr := strconv.ParseInt(pNumsStr, 10, 64)

				if scoreErr != nil {
					score = 0
				}

				if pNumsErr != nil {
					pNums = 0
				}

				if score >= criteria.Score && pNums >= criteria.People {
					lockMap <- struct{}{}
					bookObj := new(book)
					bookObj.Title = title
					bookObj.Score = score
					bookObj.People = pNums
					bookObj.Link = link
					if _, ok := crawlResult[title]; !ok {
						fmt.Printf("%v,%v,%v\n", bookObj.Title, bookObj.Score, bookObj.People)
						crawlResult[title] = bookObj
					}

					<-lockMap
				}

			})
			c <- struct{}{}
			<-lockObj
		}(i)
	}

	for i := 0; i < pageCount; i++ {
		<-c
	}
}

func getDoc(rawURL string) *goquery.Document {
	client := &http.Client{}

	req, err := http.NewRequest("GET", rawURL, nil)

	if err != nil {
		log.Fatalf("url:%s , 创建请求对象报错:%s", rawURL, err)
	}

	req.Header.Add("User-Agent", "Chrome/81.0")
	// req.Header.Add("Cookie", "__yadk_uid=")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("url:%s , 抓取报错:%s", rawURL, err)
	}

	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		log.Fatal(err)
	}

	return doc
}
