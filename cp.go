package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/elazarl/goproxy"
)

const (
	wlanInterface = "eth0"
	lanInterface  = "lan0"
	proxyPort     = "8080"
	checkInterval = 30 * time.Second
)

var captivePortalURL string

func main() {
	go monitorInterfaces()

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true

	proxy.OnRequest().DoFunc(func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if captivePortalURL != "" {
			log.Printf("Detected request while captive portal is active: %s", r.URL)
		}
		return r, nil
	})

	proxy.OnResponse().DoFunc(func(r *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if captivePortalURL != "" {
			r.Header.Set("X-Captive-Portal-URL", captivePortalURL)
			log.Printf("Added captive portal URL to response headers: %s", captivePortalURL)
		}
		return r
	})

	log.Printf("Starting proxy server on port %s", proxyPort)
	log.Fatal(http.ListenAndServe(":"+proxyPort, proxy))
}

func monitorInterfaces() {
	for {
		if detectCaptivePortal() {
			log.Println("Captive portal detected:", captivePortalURL)
		} else if captivePortalURL != "" {
			log.Println("Captive portal no longer detected")
			captivePortalURL = ""
		}
		time.Sleep(checkInterval)
	}
}

func detectCaptivePortal() bool {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get("http://captive.apple.com")
	if err != nil {
		log.Println("Error detecting captive portal:", err)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response body:", err)
		return false
	}

	if !strings.Contains(string(body), "<BODY>Success</BODY>") {
		captivePortalURL = resp.Request.URL.String()
		return true
	}

	return false
}
