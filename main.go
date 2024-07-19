package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"
)

// IPAddress represents a struct to hold the JSON output
type IPAddress struct {
	CLIIP string `json:"cli_ip"`
}

// IPIPResponse represents the response from the IPIP service
type IPIPResponse struct {
	IP        string  `json:"ip"`
	Country   string  `json:"country"`
	Province  string  `json:"province"`
	City      string  `json:"city"`
	ISP       string  `json:"isp"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Ret       int     `json:"ret"`
	Reason    string  `json:"reason"`
}

// Classification represents the classification of IPs
type Classification struct {
	Province string   `json:"province"`
	ISP      string   `json:"isp"`
	IPs      []string `json:"ips"`
}

func main() {
	// 打开并读取 Excel 文件
	f, err := excelize.OpenFile("data.xlsx")
	if err != nil {
		log.Fatalf("Error opening Excel file: %v", err)
	}

	// 获取第一个工作表名称
	sheetName := f.GetSheetName(0)

	// 读取所有行
	rows, err := f.GetRows(sheetName)
	if err != nil {
		log.Fatalf("Error getting rows from sheet: %v", err)
	}

	ipSet := make(map[string]struct{}) // 用于去重
	var ipList []string

	// 迭代行，从第二行开始
	for i, row := range rows {
		if i == 0 {
			continue // 跳过表头
		}
		if len(row) > 1 && row[1] != "" {
			ip := row[1]
			if _, exists := ipSet[ip]; !exists {
				ipSet[ip] = struct{}{}
				ipList = append(ipList, ip)
			}
		}
	}

	classifications := make(map[string]map[string][]string)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Fetch IP details and classify them
	for _, ip := range ipList {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()

			// Determine if IP is IPv4 or IPv6
			var url string
			if isIPv6(ip) {
				url = fmt.Sprintf("http://ipip-service.internal/ipv6?ip=%s", ip)
			} else {
				url = fmt.Sprintf("http://ipip-service.internal/ipv4?ip=%s", ip)
			}

			resp, err := http.Get(url)
			if err != nil {
				log.Printf("Error getting IP details for %s: %v", ip, err)
				return
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Error reading response body for %s: %v", ip, err)
				return
			}

			var ipInfo IPIPResponse
			err = json.Unmarshal(body, &ipInfo)
			if err != nil {
				log.Printf("Error unmarshaling JSON response for %s: %v", ip, err)
				return
			}

			if ipInfo.Ret != 1 {
				log.Printf("Non-successful return for %s: %s", ip, ipInfo.Reason)
				return
			}

			mu.Lock()
			if _, exists := classifications[ipInfo.Province]; !exists {
				classifications[ipInfo.Province] = make(map[string][]string)
			}
			classifications[ipInfo.Province][ipInfo.ISP] = append(classifications[ipInfo.Province][ipInfo.ISP], ipInfo.IP)
			mu.Unlock()
		}(ip)
	}

	wg.Wait()

	classifiedList := []Classification{}

	for province, isps := range classifications {
		for isp, ips := range isps {
			classifiedList = append(classifiedList, Classification{
				Province: province,
				ISP:      isp,
				IPs:      ips,
			})
		}
	}

	outputFile, err := os.Create("classified_ips.json")
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer outputFile.Close()

	jsonData, err := json.MarshalIndent(classifiedList, "", "  ")
	if err != nil {
		log.Fatalf("Error marshalling classified data to JSON: %v", err)
	}

	_, err = outputFile.Write(jsonData)
	if err != nil {
		log.Fatalf("Error writing JSON to file: %v", err)
	}

	fmt.Println("Classification complete. Output saved to classified_ips.json")
}

// isIPv6 checks if the given IP address is IPv6
func isIPv6(ip string) bool {
	return net.ParseIP(ip) != nil && strings.Contains(ip, ":")
}
