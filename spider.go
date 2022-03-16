package main

import (
    "errors"
    "net/http"
    "io/ioutil"
    "encoding/json"
    "fmt"
    "flag"
    "sync"
    "strconv"
    "strings"
    "math"
    "os"
    "github.com/PuerkitoBio/goquery"
)

const (
    apiUserList string = "https://www.huya.com/cache.php?m=LiveList&do=getLiveListByPage&page=%d"
)

type Game struct {
    Gid string
    GName string
}

type User struct {
    Uid string
    Name string `json:"nick"`
    Gid   string
    GName string `json:"gameFullName"`
    Room string `json:"profileRoom"`
}

type UserList struct {
    Page int
    TotalPage int
    Users []User `json:"datas"`
}

type UserListResponse struct {
    Status int
    Message string
    Data UserList
}

func getUserList(page int) (*UserList, error) {
    resp, err := http.Get(fmt.Sprintf(apiUserList, page))
    if err != nil {
        fmt.Println(err)
        return nil, err
    }

    buf, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        fmt.Println(err)
        return nil, err
    }

    o := &UserListResponse{}
    json.Unmarshal(buf, o)

    if o.Status != 200 {
        return nil, errors.New(o.Message)
    }
    return &o.Data, nil
}


var mu sync.Mutex
func fetchAllUser() {
    ch := make(chan *UserList)

    o, err := getUserList(1)
    if err != nil {
        fmt.Printf("error occur: %d %s\n", 1, err)
        return
    }
    total := o.TotalPage

    var wg sync.WaitGroup
    wg.Add(total)
    go func() {
        ch <- o
        wg.Done()
    }()
    for i := 2; i <= total; i++ {
        go func(i int) {
            defer wg.Done()
            o, err := getUserList(i)
            if err != nil {
                fmt.Printf("error occur: %d %s\n", i, err)
                return
            }
            
            ch <- o
        }(i)
    }

    go func() {
        wg.Wait()
        close(ch)
    }()

    games := make(map[string]bool)
    users := make(map[string]bool)

    for page := range ch {
        fmt.Printf("# fetch from page %d\n", page.Page)
        for _, user := range page.Users {
            if _, exists := users[user.Uid]; exists == false {
                if _, exists = games[user.Gid]; exists == false {
                    games[user.Gid] = true
                    fmt.Printf("insert into game values (%s, '%s');\n", user.Gid, user.GName)
                }
                users[user.Uid] = true
                fmt.Printf("insert into user values (%s, '%s', %s, %s);\n", 
                        user.Uid, user.Name, user.Gid, user.Room)
            }
        }
    }
}

const (
    apiVideoInfo string = "https://v.huya.com/index.php?r=user/liveinfo&uid=%d"
    apiVideoList string = "https://v.huya.com/u/%d/video.html?p=%d"
    apiVideo string = "https://v.huya.com%s"
)
// fetch Videos
type VideoInfo struct {
    Uid int
    Sum int
}

func getVideoInfo(uid int) (*VideoInfo, error) {
    resp, err := http.Get(fmt.Sprintf(apiVideoInfo, uid))
    if err != nil {
        fmt.Println(err)
        return nil, err
    }

    m := make(map[string]interface{})
    buf, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        fmt.Println(err)
        return nil, err
    }

    o := new(VideoInfo)
    json.Unmarshal(buf, &m)
    ss := strings.Split(m["user_video_sum"].(string), ",")
    var sum int
    for _, s := range ss {
        i, err := strconv.Atoi(s)
        if err != nil {
            return nil, err
        }
        sum = 1000*sum + i
    }
    o.Uid = int(m["uid"].(float64))
    o.Sum = sum
    return o, nil
}

const VIDEOPAGESIZE = 15
func getVideoList(uid, p int, path string) error {
    resp, err := http.Get(fmt.Sprintf(apiVideoList, uid, p))
    if err != nil {
        fmt.Println(err)
        return err
    }

    doc, err := goquery.NewDocumentFromReader(resp.Body)
    if err != nil {
        fmt.Println(err)
        return err
    }

    doc.Find(".user-videos-content .content-list ul li a").Each(func(i int, s *goquery.Selection) {
        href, _ := s.Attr("href")
        title, _ := s.Attr("title")
        id := strings.TrimRight(strings.TrimLeft(href, "/play/"), ".html")
        out := path + "/" +title + "-" + id + ".mp4"

        fetchVideo(href, out)
    })
    return nil
}

func fetchVideo(href string, out string) error {
    fmt.Println(fmt.Sprintf(apiVideo, href))
    resp, err := http.Get(fmt.Sprintf(apiVideo, href))
    if err != nil {
        fmt.Println(err)
        return err
    }

    doc, err := goquery.NewDocumentFromReader(resp.Body)
    if err != nil {
        fmt.Println(err)
        return err
    }

    doc.Find("video").Each(func(i int, s *goquery.Selection) {
        // 此处视频元素 <video> 由javascript生成
        // 这种情况, 大概率真实视频链接地址是由ajax获取的
        // 再分析 fetch/xhr 网络请求 发现有加密消息
        // 爬取陷入死局
        //link, _ := s.Attr("src")
        //fmt.Println(link)
    })
    return nil
}

func fetchAllVideo(uid int, path string) {
    info, err := getVideoInfo(uid)
    if err != nil {
        fmt.Println(err)
        return
    }
    fmt.Printf("uid:%d sum:%d\n", info.Uid, info.Sum)
    maxPage := int(math.Ceil(float64(info.Sum)/VIDEOPAGESIZE))
    for i := 1; i <= maxPage; i++ {
        getVideoList(uid, i, path)
    }
}


func main() {
    cmd := flag.String("cmd", "", "command")
    uid := flag.Int("uid", 0, "uid")
    out := flag.String("out", "videos", "videos output path")
    flag.Parse() 
    switch (*cmd) {
    case "users":
        fetchAllUser()
    case "videos":
        os.Mkdir(*out, 0755)
        fetchAllVideo(*uid, *out)
    default:
        fmt.Println("unrecognized command")
    }
}
