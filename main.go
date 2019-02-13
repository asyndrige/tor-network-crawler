package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/proxy"
)

const (
	maxRoutines = 5
)

func init() {
	var path string
	flag.StringVar(&path, "tp", "", "Path to tor executable")
	flag.Parse()
	if path != "" {
		log.Println(path)
		cmd := exec.Command(path, "--detach")
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
		time.Sleep(5 * time.Second)
	}
}

func main() {
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9150", nil, proxy.Direct)
	if err != nil {
		log.Fatal(err)
	}

	httpProxy := &http.Transport{Dial: dialer.Dial}
	client := &http.Client{Transport: httpProxy, Timeout: 10 * time.Second}

	sources := []string{
		"http://wiki5kauuihowqi5.onion/",
		"http://gxamjbnu7uknahng.onion/",
		"http://mijpsrtgf54l7um6.onion/",
		"http://dirnxxdraygbifgc.onion/",
		"http://torlinkbgs6aabns.onion/",
	}

	var (
		wg       = new(sync.WaitGroup)
		linksMap = new(sync.Map)
	)
	for _, source := range sources {
		wg.Add(1)

		go func(source string, wg *sync.WaitGroup) {
			defer wg.Done()

			res, err := client.Get(source)
			if err != nil {
				log.Fatal(err)
			}
			defer res.Body.Close()

			// quit := make(chan struct{}, 1)
			// defer close(quit)
			for link := range extractLinks(&res.Body) {
				res, err := client.Get(link)
				if err != nil {
					log.Println(err)
					continue
				}
				defer res.Body.Close()
				log.Println(extractTitle(&res.Body), "-", link)

				linksMap.LoadOrStore(link, extractTitle(&res.Body))
			}
		}(source, wg)

	}

	wg.Wait()

	var i int
	linksMap.Range(func(key, value interface{}) bool {
		fmt.Printf("%#v\n %#v", key, value)
		i++

		if i > 20 {
			return false
		}
		return true
	})

}

func extractTitle(body *io.ReadCloser) string {
	var t string
	doc, err := goquery.NewDocumentFromReader(*body)
	if err != nil {
		log.Print(err)
		return t
	}
	t = doc.Find("title").Text()
	return t
}

func extractLinks(body *io.ReadCloser) <-chan string {
	links := make(chan string)
	doc, err := goquery.NewDocumentFromReader(*body)
	if err != nil {
		log.Print(err)
		return nil
	}

	go func() {
		defer close(links)
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			if val, ok := s.Attr("href"); ok {
				if hostName := trimHostName(val); hostName != "" {
					links <- trimHostName(val)
				}
			}
		})
	}()

	return links
}

func trimHostName(link string) string {
	sep := ".onion/"
	if strings.Contains(link, sep) {
		if strings.HasSuffix(link, sep) {
			return link
		}
		return strings.Split(link, sep)[0]
	}
	return ""
}
