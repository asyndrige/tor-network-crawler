package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
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

const (
	socks5Version      = 5
	socks5AuthNone     = 0
	socks5AuthPassword = 2
	socks5Connect      = 1
	socks5IP4          = 1
	socks5Domain       = 3
	socks5IP6          = 4
)

var socks5Errors = []string{
	"",
	"general failure",
	"connection forbidden",
	"network unreachable",
	"host unreachable",
	"connection refused",
	"TTL expired",
	"command not supported",
	"address type not supported",
}

func Dial(proxyNetwork, proxyAddress string) func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		switch network {
		case "tcp", "tcp6", "tcp4":
		default:
			return nil, errors.New("proxy: no support for SOCKS5 proxy connections of type " + network)
		}

		conn, err := net.Dial(proxyNetwork, proxyAddress)
		if err != nil {
			return nil, err
		}
		if err := connect(conn, addr, proxyAddress); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}
}

func connect(conn net.Conn, target, proxyAddress string) error {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return errors.New("proxy: failed to parse port number: " + portStr)
	}
	if port < 1 || port > 0xffff {
		return errors.New("proxy: port number out of range: " + portStr)
	}

	// the size here is just an estimate
	buf := make([]byte, 0, 6+len(host))

	buf = append(buf, socks5Version)
	buf = append(buf, 1 /* num auth methods */, socks5AuthNone)

	// fmt.Printf("2 %#v\n", buf)
	if _, err := conn.Write(buf); err != nil {
		return errors.New("proxy: failed to write greeting to SOCKS5 proxy at " + proxyAddress + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return errors.New("proxy: failed to read greeting from SOCKS5 proxy at " + proxyAddress + ": " + err.Error())
	}
	if buf[0] != 5 {
		return errors.New("proxy: SOCKS5 proxy at " + proxyAddress + " has unexpected version " + strconv.Itoa(int(buf[0])))
	}
	if buf[1] == 0xff {
		return errors.New("proxy: SOCKS5 proxy at " + proxyAddress + " requires authentication")
	}

	// See RFC 1929
	buf = buf[:0]
	buf = append(buf, socks5Version, socks5Connect, 0 /* reserved */)

	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			buf = append(buf, socks5IP4)
			ip = ip4
		} else {
			buf = append(buf, socks5IP6)
		}
		buf = append(buf, ip...)
	} else {
		if len(host) > 255 {
			return errors.New("proxy: destination host name too long: " + host)
		}
		buf = append(buf, socks5Domain)
		buf = append(buf, byte(len(host)))
		buf = append(buf, host...)
	}
	buf = append(buf, byte(port>>8), byte(port))

	// fmt.Printf("4 %#v\n", buf)
	if _, err := conn.Write(buf); err != nil {
		return errors.New("proxy: failed to write connect request to SOCKS5 proxy at " + proxyAddress + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return errors.New("proxy: failed to read connect reply from SOCKS5 proxy at " + proxyAddress + ": " + err.Error())
	}

	failure := "unknown error"
	if int(buf[1]) < len(socks5Errors) {
		failure = socks5Errors[buf[1]]
	}

	if len(failure) > 0 {
		return errors.New("proxy: SOCKS5 proxy at " + proxyAddress + " failed to connect: " + failure)
	}

	bytesToDiscard := 0
	switch buf[3] {
	case socks5IP4:
		bytesToDiscard = net.IPv4len
	case socks5IP6:
		bytesToDiscard = net.IPv6len
	case socks5Domain:
		_, err := io.ReadFull(conn, buf[:1])
		if err != nil {
			return errors.New("proxy: failed to read domain length from SOCKS5 proxy at " + proxyAddress + ": " + err.Error())
		}
		bytesToDiscard = int(buf[0])
	default:
		return errors.New("proxy: got unknown address type " + strconv.Itoa(int(buf[3])) + " from SOCKS5 proxy at " + proxyAddress)
	}

	if cap(buf) < bytesToDiscard {
		buf = make([]byte, bytesToDiscard)
	} else {
		buf = buf[:bytesToDiscard]
	}
	if _, err := io.ReadFull(conn, buf); err != nil {
		return errors.New("proxy: failed to read address from SOCKS5 proxy at " + proxyAddress + ": " + err.Error())
	}

	// Also need to discard the port number
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return errors.New("proxy: failed to read port from SOCKS5 proxy at " + proxyAddress + ": " + err.Error())
	}

	/*
		[]byte{0x5, 0x1, 0x0}
		[]byte{0x5, 0x1, 0x0, 0x3, 0x16, 0x33, 0x32, 0x72, 0x66, 0x63, 0x6b, 0x77, 0x75, 0x6f, 0x72, 0x6c, 0x66, 0x34, 0x64, 0x6c, 0x76, 0x2e, 0x6f, 0x6e, 0x69, 0x6f, 0x6e, 0x0, 0x50}

		0x3 - reserved
		0x16 - addr length
		then addr
	*/

	return nil
}

func main() {
	dial := Dial("tcp", "127.0.0.1:9150")
	httpProxy := &http.Transport{Dial: dial}
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
				return
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
					links <- hostName
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
