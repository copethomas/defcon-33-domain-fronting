package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// IPRange represents an IP range with start and end addresses
type IPRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// CDNIPMap maps CDN names to their IP ranges
type CDNIPMap map[string][]IPRange

type CSVDump struct {
	CDN       string `csv:"cdn"`
	DomainSLD string `csv:"domain_sld"`
	IP        string `csv:"ip_addr"`
}

// Global variables
var cdnIPMap CDNIPMap
var threadCount = 50 // Number of concurrent goroutines to use for processing domains

var noCDN = errors.New("no cdn found for this IP")

// List of public DNS servers from around the world
var publicDNSServers = []string{
	// North America
	"8.8.8.8:53",        // Google DNS (USA)
	"1.1.1.1:53",        // Cloudflare (USA)
	"9.9.9.9:53",        // Quad9 (USA)
	"208.67.222.222:53", // OpenDNS (USA)
	"64.6.64.6:53",      // Verisign (USA)
	"8.26.56.26:53",     // Comodo Secure DNS (USA)

	// Europe
	"84.200.69.80:53", // DNS.WATCH (Germany)
	"77.88.8.8:53",    // Yandex DNS (Russia)
	"80.80.80.80:53",  // Freenom World (Netherlands)
	"195.46.39.39:53", // SafeDNS (UK)

	// Asia
	"119.29.29.29:53",    // DNSPod (China)
	"114.114.114.114:53", // 114DNS (China)
	"223.5.5.5:53",       // AliDNS (China)
	"180.76.76.76:53",    // Baidu DNS (China)
	"101.226.4.6:53",     // DNSPai (China)
	"1.2.4.8:53",         // CNNIC SDNS (China)
	"168.95.1.1:53",      // Chunghwa Telecom (Taiwan)
	"202.181.224.2:53",   // PCCW (Hong Kong)
	"101.101.101.101:53", // Korea Telecom (South Korea)

	// Australia/Oceania
	"203.50.2.71:53", // Telstra (Australia)

	// Africa
	"196.213.41.10:53", // Internet Solutions (South Africa)

	// South America
	"200.56.224.11:53", // Ultranet (Mexico)
	"200.85.37.254:53", // Telecom Argentina (Argentina)
}

// init initializes the random number generator
func init() {
	rand.Seed(time.Now().UnixNano())
}

// lookupIPWithRetry performs DNS lookup with retry using different public DNS servers
func lookupIPWithRetry(domain string) ([]net.IP, error) {
	// Create a copy of the DNS servers list to shuffle
	servers := make([]string, len(publicDNSServers))
	copy(servers, publicDNSServers)

	// Shuffle the servers to randomize the order
	rand.Shuffle(len(servers), func(i, j int) {
		servers[i], servers[j] = servers[j], servers[i]
	})

	var lastErr error
	// Try each DNS server until one succeeds
	for _, server := range servers {
		log.Debug().Msgf("Attempting DNS lookup for %s using server %s", domain, server)

		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Second * 2,
				}
				return d.DialContext(ctx, "udp", server)
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()

		ips, err := r.LookupIP(ctx, "ip", domain)
		if err != nil {
			lastErr = err
			log.Debug().Msgf("DNS lookup failed with server %s: %v", server, err)
			continue
		}

		if len(ips) > 0 {
			log.Debug().Msgf("Successfully resolved %s using server %s", domain, server)
			return ips, nil
		}
	}

	return nil, fmt.Errorf("all DNS servers failed to resolve %s: %v", domain, lastErr)
}

// isIPInRange checks if an IP address is within a given range
func isIPInRange(ip, start, end string) bool {
	ipParsed := net.ParseIP(ip)
	startParsed := net.ParseIP(start)
	endParsed := net.ParseIP(end)

	if ipParsed == nil || startParsed == nil || endParsed == nil {
		return false
	}

	return bytes.Compare(ipParsed, startParsed) >= 0 && bytes.Compare(ipParsed, endParsed) <= 0
}

// getCDNForIP determines which CDN an IP address belongs to
func getCDNForIP(ip string) (string, error) {
	for cdn, ranges := range cdnIPMap {
		for _, ipRange := range ranges {
			if isIPInRange(ip, ipRange.Start, ipRange.End) {
				return cdn, nil
			}
		}
	}
	return "unknown", noCDN
}

// loadCDNIPMap loads the CDN to IP mapping from the JSON file
func loadCDNIPMap(filePath string) error {
	// Read the JSON file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading CDN IP map file: %v", err)
	}

	// Parse the JSON
	if err := json.Unmarshal(data, &cdnIPMap); err != nil {
		return fmt.Errorf("error parsing CDN IP map: %v", err)
	}

	log.Info().Msgf("Loaded CDN IP map with %d CDNs", len(cdnIPMap))
	return nil
}

// processDomain processes a single domain
func processDomain(domain string) (CSVDump, error) {
	// Perform DNS lookup with retry using different public DNS servers
	ips, err := lookupIPWithRetry(domain)
	if err != nil {
		return CSVDump{}, fmt.Errorf("DNS lookup failed for %s after trying all DNS servers: %v", domain, err)
	}

	if len(ips) == 0 {
		return CSVDump{}, fmt.Errorf("no IP addresses found for %s", domain)
	}

	// Use the first IP address
	ip := ips[0].String()
	// Determine if a CDN is associated with the IP
	cdn, err := getCDNForIP(ip)
	if err != nil {
		return CSVDump{}, err
	}

	// Create and return the CSVDump structure
	resources := CSVDump{
		CDN:       cdn,
		DomainSLD: domain,
		IP:        ip,
	}
	log.Info().Any("result", resources).Msgf("resolved cdn")
	return resources, nil
}

func main() {
	// Define command-line flags
	inputFile := flag.String("input", "top-1m.csv", "Path to the input domain list file")
	cdnMapFile := flag.String("cdn-map", "cdn_asn_to_ip_map.json", "Path to the CDN to IP mapping file")
	outputFile := flag.String("output", "domains_to_cdn.csv", "Path to the output CSV file")

	// Parse flags
	flag.Parse()

	// Set up zerolog
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFilePath := fmt.Sprintf("scape_%s.log", timestamp)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		fmt.Printf("Error creating log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	// Configure zerolog to write to both stdout and the log file
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	multi := zerolog.MultiLevelWriter(consoleWriter, logFile)

	// Set global logger
	log.Logger = zerolog.New(multi).With().Timestamp().Logger()

	// Set log level to debug
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Validate flags
	if *inputFile == "" || *cdnMapFile == "" || *outputFile == "" {
		flag.Usage()
		log.Fatal().Msg("All file paths are required")
	}

	// Load the CDN to IP mapping
	if err := loadCDNIPMap(*cdnMapFile); err != nil {
		log.Fatal().Msgf("Error loading CDN IP map: %v", err)
	}

	// Open the domain list file
	file, err := os.Open(*inputFile)
	if err != nil {
		log.Fatal().Msgf("Error opening input file: %v", err)
	}
	defer file.Close()

	// Count total number of domains for progress tracking
	totalDomains := 0
	countScanner := bufio.NewScanner(file)
	for countScanner.Scan() {
		totalDomains++
	}
	log.Info().Msgf("Total domains to process: %d", totalDomains)

	// Reset file pointer to beginning
	file.Seek(0, 0)

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Progress tracking variables
	processedCount := 0
	processingTimes := make([]time.Duration, 0, 10) // Store last 10 processing times

	// Create output CSV file
	outFile, err := os.Create(*outputFile)
	if err != nil {
		log.Fatal().Msgf("Error creating output file: %v", err)
	}
	defer outFile.Close()

	// Create CSV writer
	writer := csv.NewWriter(outFile)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"cdn", "domain_sld", "ip_addr"}); err != nil {
		log.Fatal().Msgf("Error writing CSV header: %v", err)
	}

	// Create channels for job distribution and result collection
	type Job struct {
		domain string
		line   string
	}
	type Result struct {
		resources CSVDump
		err       error
		domain    string
		duration  time.Duration
	}

	jobs := make(chan Job, threadCount)
	results := make(chan Result, threadCount)

	// Use WaitGroup to track when all workers are done
	var wg sync.WaitGroup

	// Start worker pool
	for w := 1; w <= threadCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				startTime := time.Now()
				resources, err := processDomain(job.domain)
				duration := time.Since(startTime)

				results <- Result{
					resources: resources,
					err:       err,
					domain:    job.domain,
					duration:  duration,
				}
			}
		}(w)
	}

	// Start a goroutine to send jobs to the workers
	go func() {
		for scanner.Scan() {
			line := scanner.Text()

			// Parse the domain from the CSV line (format: rank,domain)
			parts := strings.Split(line, ",")
			if len(parts) < 2 {
				log.Warn().Msgf("Invalid line format: %s", line)
				continue
			}

			domain := parts[1]
			jobs <- Job{domain: domain, line: line}
		}
		close(jobs) // Close the jobs channel when all domains have been sent
	}()

	// Start a goroutine to close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results as they come in
	for result := range results {
		if result.err != nil {
			if errors.Is(result.err, noCDN) {
				log.Warn().Err(result.err).Str("domain", result.domain).Msg("no cdn")
			} else {
				log.Error().Err(result.err).Str("domain", result.domain).Msg("Error processing domain")
			}
			// Update progress tracking no matter the result
			processedCount++
			continue
		}

		// Write the result to the CSV file immediately
		if err := writer.Write([]string{result.resources.CDN, result.resources.DomainSLD, result.resources.IP}); err != nil {
			log.Error().Msgf("Error writing record to CSV: %v", err)
		}
		writer.Flush() // Flush after each write to ensure data is written immediately

		// Track all processing times for a continuously updating ETA
		processingTimes = append(processingTimes, result.duration)

		// Log progress every 10 domains
		if processedCount%10 == 0 {
			remaining := totalDomains - processedCount
			if remaining < 0 {
				remaining = 0
			}
			percentComplete := float64(processedCount) / float64(totalDomains) * 100

			// Calculate ETA based on the average of all processing times so far
			var avgProcessingTime time.Duration
			if len(processingTimes) > 0 {
				totalTime := time.Duration(0)
				for _, t := range processingTimes {
					totalTime += t
				}
				avgProcessingTime = totalTime / time.Duration(len(processingTimes))
				eta := avgProcessingTime * time.Duration(remaining)

				log.Info().Msgf("Progress: %d/%d domains processed (%.2f%%) - %d remaining - ETA: %s",
					processedCount, totalDomains, percentComplete, remaining, eta.Round(time.Second))
			} else {
				log.Info().Msgf("Progress: %d/%d domains processed (%.2f%%) - %d remaining",
					processedCount, totalDomains, percentComplete, remaining)
			}
		}
	}

	log.Info().Msgf("Processing complete! CSV data has been written to %s", *outputFile)
	log.Info().Msgf("Finished")
}
