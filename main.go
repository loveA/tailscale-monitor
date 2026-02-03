package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Tailscale API 响应结构
type Device struct {
	ID        string `json:"id"`
	Hostname  string `json:"hostname"`
	Connected bool   `json:"connected"`
}

type DevicesResponse struct {
	Devices []Device `json:"devices"`
}

// 全局缓存，记录设备上次的在线状态
var statusCache = make(map[string]bool)
var isFirstRun = true

func main() {
	fmt.Println("🚀 Tailscale 监控服务启动...")
	
	intervalStr := os.Getenv("CHECK_INTERVAL")
	if intervalStr == "" {
		intervalStr = "60"
	}
	interval, _ := time.ParseDuration(intervalStr + "s")

	for {
		checkDevices()
		isFirstRun = false // 第一次运行后，后续变化才发通知
		time.Sleep(interval)
	}
}

func checkDevices() {
	clientID := os.Getenv("TS_CLIENT_ID")
	clientSecret := os.Getenv("TS_CLIENT_SECRET")
	tailnet := os.Getenv("TS_TAILNET")

	// 1. 获取 Access Token (OAuth2)
	token, err := getAccessToken(clientID, clientSecret)
	if err != nil {
		fmt.Printf("❌ 获取 Token 失败: %v\n", err)
		return
	}

	// 2. 获取设备列表
	apiURL := fmt.Sprintf("https://api.tailscale.com/api/v2/tailnet/%s/devices", tailnet)
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.SetBasicAuth(token, "") // API 使用 Token 作为用户名

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ 请求 API 失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data DevicesResponse
	json.NewDecoder(resp.Body).Decode(&data)

	// 3. 对比状态变化
	for _, dev := range data.Devices {
		lastStatus, exists := statusCache[dev.ID]
		
		if exists && lastStatus != dev.Connected {
			// 状态发生切换
			sendNotification(dev.Hostname, dev.Connected)
		} else if !exists && !isFirstRun {
			// 发现新设备加入网络
			sendNotification(dev.Hostname, dev.Connected)
		}
		
		// 更新缓存
		statusCache[dev.ID] = dev.Connected
	}
}

func getAccessToken(id, secret string) (string, error) {
	data := url.Values{}
	data.Set("client_id", id)
	data.Set("client_secret", secret)
	data.Set("grant_type", "client_credentials")

	resp, err := http.PostForm("https://api.tailscale.com/api/v2/oauth/token", data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	return res.AccessToken, nil
}

func sendNotification(name string, online bool) {
	barkURL := os.Getenv("BARK_URL")
	if barkURL == "" {
		return
	}

	statusMsg := "已上线 🟢"
	if !online {
		statusMsg = "已离线 🔴"
	}

	title := url.PathEscape("Tailscale 状态变动")
	body := url.PathEscape(fmt.Sprintf("设备 [%s] %s", name, statusMsg))
	
	// 组装 Bark 链接
	fullURL := fmt.Sprintf("%s/%s/%s?group=Tailscale&icon=https://tailscale.com/favicon.png", 
		strings.TrimSuffix(barkURL, "/"), title, body)

	http.Get(fullURL)
	fmt.Printf("🔔 通知已发送: %s %s\n", name, statusMsg)
}
