package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
)

type IPInfo struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
	Ret      int    `json:"ret"`
	Reason   string `json:"reason"`
}

type ClassifiedIPs struct {
	Province string   `json:"province"`
	ISP      string   `json:"isp"`
	IPs      []string `json:"ips"`
}

var (
	ipv4URL = "http://ipip-service.internal/ipv4?ip="
	ipv6URL = "http://ipip-service.internal/ipv6?ip="
	unknown = "unknown"
)

func classifyIPs(ips []string) map[string]map[string][]string {
	result := make(map[string]map[string][]string)
	var mu sync.Mutex // mutex to handle concurrent map writes

	wg := sync.WaitGroup{}
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			var ipInfo IPInfo
			if strings.Contains(ip, ":") {
				ipInfo = getIPInfo(ipv6URL + ip)
			} else {
				ipInfo = getIPInfo(ipv4URL + ip)
			}

			province := ipInfo.Province
			isp := ipInfo.ISP

			if ipInfo.Ret != 1 {
				province = unknown
				isp = unknown
			}

			mu.Lock()
			defer mu.Unlock()

			if _, ok := result[province]; !ok {
				result[province] = make(map[string][]string)
			}
			if _, ok := result[province][isp]; !ok {
				result[province][isp] = make([]string, 0)
			}

			result[province][isp] = append(result[province][isp], ip)
		}(ip)
	}
	wg.Wait()
	return result
}

func getIPInfo(url string) IPInfo {
	resp, err := http.Get(url)
	if err != nil {
		return IPInfo{Ret: 0, Reason: err.Error()}
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return IPInfo{Ret: 0, Reason: err.Error()}
	}

	var ipInfo IPInfo
	err = json.Unmarshal(body, &ipInfo)
	if err != nil {
		return IPInfo{Ret: 0, Reason: err.Error()}
	}
	return ipInfo
}

func readIPsFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ips = append(ips, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ips, nil
}

func main() {
	file := flag.String("f", "", "input file with IP addresses")
	flag.Parse()

	var ips []string
	if *file != "" {
		fileIPs, err := readIPsFromFile(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
		ips = fileIPs
	} else {
		ips = flag.Args()
	}

	ips = unique(ips)
	classified := classifyIPs(ips)

	output, err := json.MarshalIndent(classified, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating JSON output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(output))
}

func unique(ips []string) []string {
	keys := make(map[string]bool)
	var list []string
	for _, entry := range ips {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
