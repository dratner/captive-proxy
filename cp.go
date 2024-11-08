package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// HTTPClient interface for mocking in tests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var (
	wanInterface     string
	lanInterface     string
	captivePortalURL string
	isCaptivePortal  bool
	mu               sync.Mutex
	handle           *pcap.Handle

	// Function variables for easier mocking in tests
	detectCaptivePortalFunc          func() (bool, error)
	detectCaptivePortalWithRetryFunc func() (bool, error)

	// HTTP client variable for mocking
	httpClient HTTPClient
)

func init() {
	// Initialize the function variables with the actual implementations
	detectCaptivePortalFunc = detectCaptivePortalImpl
	detectCaptivePortalWithRetryFunc = detectCaptivePortalWithRetryImpl

	// Initialize the HTTP client
	httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}
}

func main() {
	flag.StringVar(&wanInterface, "wan", "wlan0", "WAN interface name")
	flag.StringVar(&lanInterface, "lan", "wlan1", "LAN interface name")
	flag.Parse()

	log.Printf("Starting captive portal handler with WAN interface %s and LAN interface %s\n", wanInterface, lanInterface)

	go pollCaptivePortal()

	select {}
}

func pollCaptivePortal() {
	initialInterval := 30 * time.Second
	activeInterval := 3 * time.Second
	currentInterval := initialInterval

	for {
		detected, err := detectCaptivePortalWithRetryFunc()

		mu.Lock()
		previousState := isCaptivePortal
		isCaptivePortal = detected
		mu.Unlock()

		if err != nil {
			log.Println("Error detecting captive portal:", err)
		} else if detected {
			log.Println("Captive portal detected")
			currentInterval = activeInterval
			if !previousState {
				go startCapturingPackets()
			}
		} else {
			log.Println("No captive portal detected")
			currentInterval = initialInterval
			if previousState {
				stopCapturingPackets()
			}
		}

		time.Sleep(currentInterval)
	}
}

func detectCaptivePortalWithRetryImpl() (bool, error) {
	for i := 0; i < 2; i++ {
		detected, err := detectCaptivePortalFunc()
		if err == nil {
			return detected, nil
		}
		if i == 0 {
			log.Println("Retrying captive portal detection...")
			time.Sleep(1 * time.Second)
		}
	}
	return false, fmt.Errorf("failed to detect captive portal after retry")
}

func detectCaptivePortalImpl() (bool, error) {
	req, err := http.NewRequest("GET", "http://captive.apple.com", nil)
	if err != nil {
		return false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println("Error detecting captive portal:", err)
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response body:", err)
		return false, err
	}

	if !strings.Contains(string(body), "<BODY>Success</BODY>") {
		captivePortalURL = resp.Request.URL.String()
		return true, nil
	}
	return false, nil
}

func startCapturingPackets() {
	var err error
	handle, err = pcap.OpenLive(lanInterface, 1600, true, pcap.BlockForever)
	if err != nil {
		log.Fatal(err)
	}

	err = handle.SetBPFFilter("tcp and (port 80 or port 443)")
	if err != nil {
		log.Fatal(err)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := range packetSource.Packets() {
		mu.Lock()
		if isCaptivePortal {
			handlePacket(packet)
		} else {
			mu.Unlock()
			break
		}
		mu.Unlock()
	}
}

func stopCapturingPackets() {
	if handle != nil {
		handle.Close()
	} else {
		log.Println("Handle is nil")
	}
	// Restore normal routing
	cmd := exec.Command("iptables", "-F")
	err := cmd.Run()
	if err != nil {
		log.Println("Error clearing iptables rules:", err)
	}
}

func handlePacket(packet gopacket.Packet) {
	ipLayer := packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return
	}
	ip, _ := ipLayer.(*layers.IPv4)

	tcpLayer := packet.Layer(layers.LayerTypeTCP)
	if tcpLayer == nil {
		return
	}
	tcp, _ := tcpLayer.(*layers.TCP)

	if tcp.SYN && !tcp.ACK {
		go sendRedirectResponse(ip.SrcIP, tcp.SrcPort, ip.DstIP, tcp.DstPort, tcp.Seq)
	}
}

func sendRedirectResponse(srcIP net.IP, srcPort layers.TCPPort, dstIP net.IP, dstPort layers.TCPPort, seq uint32) {
	conn, err := net.Dial("ip4:tcp", dstIP.String())
	if err != nil {
		log.Println("Error dialing:", err)
		return
	}
	defer conn.Close()

	ipLayer := &layers.IPv4{
		SrcIP:    dstIP,
		DstIP:    srcIP,
		Protocol: layers.IPProtocolTCP,
	}

	tcpLayer := &layers.TCP{
		SrcPort: dstPort,
		DstPort: srcPort,
		Seq:     seq,
		ACK:     true,
		Window:  65535,
		Ack:     seq + 1,
	}
	tcpLayer.SetNetworkLayerForChecksum(ipLayer)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	err = gopacket.SerializeLayers(buf, opts, tcpLayer)
	if err != nil {
		log.Println("Error serializing packet:", err)
		return
	}

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		log.Println("Error sending packet:", err)
		return
	}

	// Send HTTP redirect
	httpResponse := fmt.Sprintf("HTTP/1.1 302 Found\r\nLocation: %s\r\n\r\n", captivePortalURL)
	tcpLayer.PSH = true
	tcpLayer.Seq += 1

	buf = gopacket.NewSerializeBuffer()
	err = gopacket.SerializeLayers(buf, opts, tcpLayer, gopacket.Payload([]byte(httpResponse)))
	if err != nil {
		log.Println("Error serializing HTTP response:", err)
		return
	}

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		log.Println("Error sending HTTP response:", err)
		return
	}
}
