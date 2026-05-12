// Package wxauth 微信小游戏登录链路：用 code 换 openid
// 通过 https://api.weixin.qq.com/sns/jscode2session 接口实现。
// AppID / AppSecret 从环境变量 WX_APPID / WX_SECRET 读取，未配置时返回错误，由调用方降级到 X-WX-OPENID 头。
package wxauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Code2SessionResp 微信 jscode2session 响应体
type code2SessionResp struct {
	Openid     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

// httpClient 复用 3 秒超时的 client
var httpClient = &http.Client{Timeout: 3 * time.Second}

// Configured 是否已配置 AppID / AppSecret
func Configured() bool {
	return os.Getenv("WX_APPID") != "" && os.Getenv("WX_SECRET") != ""
}

// Code2Session 用 code 换取 openid
func Code2Session(code string) (string, error) {
	if code == "" {
		return "", errors.New("空 code")
	}
	appID := os.Getenv("WX_APPID")
	secret := os.Getenv("WX_SECRET")
	if appID == "" || secret == "" {
		return "", errors.New("WX_APPID/WX_SECRET 未配置")
	}
	q := url.Values{}
	q.Set("appid", appID)
	q.Set("secret", secret)
	q.Set("js_code", code)
	q.Set("grant_type", "authorization_code")
	endpoint := "https://api.weixin.qq.com/sns/jscode2session?" + q.Encode()
	resp, err := httpClient.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("jscode2session http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("jscode2session read: %w", err)
	}
	var r code2SessionResp
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("jscode2session decode: %w (raw=%s)", err, string(body))
	}
	if r.ErrCode != 0 {
		return "", fmt.Errorf("jscode2session errcode=%d errmsg=%s", r.ErrCode, r.ErrMsg)
	}
	if r.Openid == "" {
		return "", errors.New("jscode2session openid 为空")
	}
	return r.Openid, nil
}
