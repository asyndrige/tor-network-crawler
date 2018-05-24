package main

import (
	"io"
	"log"
	"net/http"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/proxy"
)

const (
	maxRoutines = 5
)

func main() {
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9150", nil, proxy.Direct)
	if err != nil {
		log.Fatal(err)
	}

	httpProxy := &http.Transport{Dial: dialer.Dial}
	client := &http.Client{Transport: httpProxy}

	res, err := client.Get("http://wiki5kauuihowqi5.onion/")
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	for {
		select {
		case link := <-extractLinks(&res.Body):
			_, err := client.Get(link)
			if err != nil {
				log.Println(err)
				continue
			}
		}
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

func extractLinks(body *io.ReadCloser) <-chan string {
	links := make(chan string)
	doc, err := goquery.NewDocumentFromReader(*body)
	if err != nil {
		log.Print(err)
		return nil
	}
	go func() {
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			if val, ok := s.Attr("href"); ok {
				links <- val
			}
		})
	}()

	return links
}

/*
~/tor-browser-linux64-7.5.4_ru/tor-browser_ru/Browser$ ./start-tor-browser --detach
*/
