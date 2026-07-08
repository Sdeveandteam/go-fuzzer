package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func cetakBanner() {
	banner := `
============================================
  _____  _____       _   _  _____ 
 |  ___||  ___|     | | | ||  ___|
 | |_   | |_   _   _| | | || |_   
 |  _|  |  _| | | | | | | ||  _|  
 |_|    |_|   | |_| | |_| ||_|    
               \__,_|\___/        

 [ ADVANCED GO WEB FUZZER ENGINE v2.1 ]
 Author: sdev (2026)
============================================`
	fmt.Println(banner)
}

func simpanCrash(payload string, iterasi uint64, msg string) {
	file, err := os.OpenFile("crash_report.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		defer file.Close()
		fmt.Fprintf(file, "[!] ANOMALI DETECTED\nIterasi: %d\nPayload: %s\nDetail: %s\n----------------\n", iterasi, payload, msg)
	}
}

func main() {
	cetakBanner()

	args := os.Args
	if len(args) < 3 {
		fmt.Printf("Cara Penggunaan: %s <target_url_dengan_FUZZ> <timeout_detik>\n", args[0])
		fmt.Printf("Contoh         : %s http://target-olshop.com/FUZZ 60\n", args[0])
		os.Exit(1)
	}

	targetURL := args[1]
	if !strings.Contains(targetURL, "FUZZ") {
		fmt.Println("[-] Error: Target URL harus mengandung kata 'FUZZ' sebagai penanda injeksi.")
		os.Exit(1)
	}

	timeoutSecs, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		fmt.Println("[-] Error: Timeout harus berupa angka.")
		os.Exit(1)
	}

	kamus := []string{
		"ADMIN", "bypass_token", "../", "' OR '1'='1", "%%30%30", "null",
		strings.Repeat("A", 100), strings.Repeat("A", 500), strings.Repeat("A", 1000),
		`<script>alert(1)</script>`, "&& sleep 10",
	}

	if file, err := os.Open("dictionary.txt"); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			teks := strings.TrimSpace(scanner.Text())
			if teks != "" {
				kamus = append(kamus, teks)
			}
		}
		file.Close()
	}

	var totalIterasi uint64
	var berjalan int32 = 1
	mulai := time.Now()
	timeoutDurasi := time.Duration(timeoutSecs) * time.Second

	fmt.Println("[*] Membaca database kata...")
	fmt.Printf("[+] Total entry dictionary: %d\n", len(kamus))
	fmt.Println("[*] Menjalankan 8 thread sekaligus...")
	fmt.Printf("[*] Batas waktu pengerjaan: %d detik\n\n", timeoutSecs)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   5 * time.Second,
	}

	var wg sync.WaitGroup
	jumlahThread := 8
	seedChan := make(chan string, 100)

	// Perbaikan: Mengisi channel di goroutine terpisah agar tidak mengunci (deadlock)
	go func() {
		for i := 0; i < len(kamus); i++ {
			seedChan <- kamus[i]
			for j := 0; j < len(kamus); j++ {
				if i != j {
					seedChan <- kamus[i] + kamus[j]
				}
			}
		}
		close(seedChan)
	}()

	for tID := 0; tID < jumlahThread; tID++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()
			lokalIterasi := 0

			for payload := range seedChan {
				if atomic.LoadInt32(&berjalan) == 0 || time.Since(mulai) >= timeoutDurasi {
					return
				}

				iter := atomic.AddUint64(&totalIterasi, 1)
				lokalIterasi++

				urlTujuan := strings.Replace(targetURL, "FUZZ", payload, -1)
				req, err := http.NewRequest("GET", urlTujuan, nil)
				if err != nil {
					continue
				}

				respon, err := client.Do(req)
				if err != nil {
					if atomic.CompareAndSwapInt32(&berjalan, 1, 0) {
						durasi := time.Since(mulai)
						fmt.Printf("\n%s\n", strings.Repeat("!", 50))
						fmt.Printf("[!] TARGET DOWN / TIMEOUT OLEH THREAD-%d!\n", threadID)
						fmt.Printf("[!] Total Iterasi Semua Thread : %d\n", iter)
						fmt.Printf("[!] Payload Pemicu             : %s\n", payload)
						fmt.Printf("[!] Error                      : %v\n", err)
						fmt.Printf("[!] Waktu Eksekusi             : %.4f detik\n", durasi.Seconds())
						fmt.Printf("[*] Menyimpan payload ke \"crash_report.txt\"...\n")
						fmt.Printf("%s\n", strings.Repeat("!", 50))
						simpanCrash(payload, iter, "Network Timeout/Target Down")
						os.Exit(0)
					}
					return
				}

				status := respon.StatusCode
				respon.Body.Close()

				if status == 500 || status == 403 {
					if atomic.CompareAndSwapInt32(&berjalan, 1, 0) {
						durasi := time.Since(mulai)
						fmt.Printf("\n%s\n", strings.Repeat("!", 50))
						fmt.Printf("[!] ANOMALI/CRASH DITEMUKAN OLEH THREAD-%d!\n", threadID)
						fmt.Printf("[!] Total Iterasi Semua Thread : %d\n", iter)
						fmt.Printf("[!] Payload Pemicu             : %s\n", payload)
						fmt.Printf("[!] HTTP Status Target         : %d\n", status)
						fmt.Printf("[!] Waktu Eksekusi             : %.4f detik\n", durasi.Seconds())
						fmt.Printf("[*] Menyimpan payload ke \"crash_report.txt\"...\n")
						fmt.Printf("%s\n", strings.Repeat("!", 50))
						simpanCrash(payload, iter, fmt.Sprintf("HTTP Status %d", status))
						os.Exit(0)
					}
					return
				}

				if threadID == 0 && lokalIterasi%2 == 0 {
					ops := float64(atomic.LoadUint64(&totalIterasi)) / time.Since(mulai).Seconds()
					fmt.Printf("[*] Hasil Tes: %d payload | Kecepatan: %.0f exec/sec\r", atomic.LoadUint64(&totalIterasi), ops)
				}
			}
		}(tID)
	}

	wg.Wait()
	fmt.Printf("\n[*] Selesai. Total Pengujian: %d iterasi.\n", atomic.LoadUint64(&totalIterasi))
}
