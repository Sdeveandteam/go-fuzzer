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

 [ ADVANCED GO WEB FUZZER ENGINE v2.2 ]
 Author: sdev (2026) - Full Scan Mode
============================================`
	fmt.Println(banner)
}

func simpanCrash(payload string, iterasi uint64, msg string) {
	file, err := os.OpenFile("crash_report.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		defer file.Close()
		fmt.Fprintf(file, "[!] ANOMALI DETECTED | Iterasi: %d | Payload: %s | Detail: %s\n", iterasi, payload, msg)
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
	var totalAnomali uint64
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
		Timeout:   3 * time.Second, // Timeout dipotong ke 3 detik agar tidak kelamaan menunggu target macet
	}

	var wg sync.WaitGroup
	jumlahThread := 8
	seedChan := make(chan string, 100)

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
				if time.Since(mulai) >= timeoutDurasi {
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
					// Jika timeout, catat anomalinya tapi scan tetap jalan terus
					atomic.AddUint64(&totalAnomali, 1)
					fmt.Printf("\n[!] Thread-%d Mendeteksi Timeout pada payload: %s\n", threadID, payload)
					simpanCrash(payload, iter, "Network Timeout / No Response")
					continue
				}

				status := respon.StatusCode
				respon.Body.Close()

				if status == 500 || status == 403 {
					// Jika respon status server error, catat dan lanjut scan
					atomic.AddUint64(&totalAnomali, 1)
					fmt.Printf("\n[!] Thread-%d Menemukan Status %d pada payload: %s\n", threadID, status, payload)
					simpanCrash(payload, iter, fmt.Sprintf("HTTP Status %d", status))
					continue
				}

				if threadID == 0 && lokalIterasi%2 == 0 {
					ops := float64(atomic.LoadUint64(&totalIterasi)) / time.Since(mulai).Seconds()
					// FIX: Menggunakan LoadUint64 agar tidak memicu error data type saat kompilasi
					fmt.Printf("[*] Hasil Tes: %d payload | Anomali: %d | Speed: %.0f exec/sec\r", atomic.LoadUint64(&totalIterasi), atomic.LoadUint64(&totalAnomali), ops)
				}
			}
		}(tID)
	}

	wg.Wait()
	fmt.Printf("\n\n[*] Selesai! Total Pengujian: %d iterasi.\n", atomic.LoadUint64(&totalIterasi))
	fmt.Printf("[*] Total Celah/Anomali tercatat: %d item.\n", atomic.LoadUint64(&totalAnomali))
	fmt.Println("[*] Silakan cek file \"crash_report.txt\" untuk melihat daftar lengkap celah.")
}
