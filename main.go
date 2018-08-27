package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
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

	res, err := client.Get("http://wiki5kauuihowqi5.onion/")
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	quit := make(chan struct{}, 1)
	defer close(quit)
	for link := range extractLinks(&res.Body, quit) {
		res, err := client.Get(link)
		if err != nil {
			log.Println(err)
			continue
		}
		log.Println(extractTitle(&res.Body), "-", link)
		res.Body.Close()
	}

	// conn, err := dialer.Dial("tcp", "3g2upl4pq6kufc4m.onion:80")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer conn.Close()
	// buf := make([]byte, 0, 4096)
	// tmp := make([]byte, 0, 256)
	// for {
	// n, err := conn.Read(buf)
	// println("HERE")
	// if err != nil {
	// 	if err != io.EOF {
	// 		fmt.Println("read error:", err)
	// 	}
	// 	break
	// }
	// fmt.Println("got", n, "bytes.")
	// buf = append(buf, tmp[:n]...)

	// }
	// fmt.Println("total size:", len(buf))
	// fmt.Println(string(buf))
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

func extractLinks(body *io.ReadCloser, quit chan struct{}) <-chan string {
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
