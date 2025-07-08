package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// IPRange represents an IP range with start and end addresses
type IPRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// CDNIPMap maps CDN names to their IP ranges
type CDNIPMap map[string][]IPRange

// ResourceInfo represents information about a resource loaded from a website
type ResourceInfo struct {
	CDN            string `json:"cdn"`
	OriginalDomain string `json:"original_domain"`
	ContentType    string `json:"content_type"`
	ResourceURL    string `json:"resource_url"`
	ServerIP       string `json:"server_ip"`
}

type CommonCrawlCDX struct {
	Urlkey       string `json:"urlkey"`
	Timestamp    string `json:"timestamp"`
	URL          string `json:"url"`
	Mime         string `json:"mime"`
	MimeDetected string `json:"mime-detected"`
	Status       string `json:"status"`
	Digest       string `json:"digest"`
	Length       string `json:"length"`
	Offset       string `json:"offset"`
	Filename     string `json:"filename"`
	Redirect     string `json:"redirect"`
}

// WAT represents the structure of the WAT file response
type WAT struct {
	Envelope struct {
		WARC struct {
			Header struct {
				ContentType string `json:"Content-Type"`
			} `json:"header"`
		} `json:"WARC-Header-Metadata"`
		Payload struct {
			RequestHeaders struct {
				Headers map[string]string `json:"headers"`
			} `json:"request-headers"`
			ResponseHeaders struct {
				Headers map[string]string `json:"headers"`
			} `json:"response-headers"`
			ResponseBody struct {
				Links []struct {
					URL string `json:"url"`
				} `json:"links"`
			} `json:"response-body"`
		} `json:"payload-metadata"`
	} `json:"Envelope"`
}

// Global variables
var cdnIPMap CDNIPMap
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// Store the latest collection info to avoid fetching it multiple times
var latestCollectionInfo *CollectionInfo

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
func getCDNForIP(ip string) string {
	for cdn, ranges := range cdnIPMap {
		for _, ipRange := range ranges {
			if isIPInRange(ip, ipRange.Start, ipRange.End) {
				return cdn
			}
		}
	}
	return "unknown"
}

// CollectionInfo represents the structure of the Common Crawl collection info API response
type CollectionInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	TimestampStr  string `json:"timegate"`
	CDXServer     string `json:"cdx-api"`
	IndexURL      string `json:"cdx-server"`
	CollectionURL string `json:"collection"`
}

// getLatestCommonCrawlIndex fetches the latest Common Crawl index for a domain
func getLatestCommonCrawlIndex(domain string) (*CommonCrawlCDX, error) {
	// Check if we already have the latest collection info
	if latestCollectionInfo == nil {
		// First, get the list of available indexes from the Common Crawl API
		collInfoURL := "https://index.commoncrawl.org/collinfo.json"

		resp, err := httpClient.Get(collInfoURL)
		if err != nil {
			panic(err)
			return nil, fmt.Errorf("error fetching Common Crawl collection info: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			panic(resp.StatusCode)
			return nil, fmt.Errorf("received non-OK response from Common Crawl collection info: %s", resp.Status)
		}

		var collections []CollectionInfo
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&collections); err != nil {
			return nil, fmt.Errorf("error decoding Common Crawl collection info: %v", err)
		}

		if len(collections) == 0 {
			return nil, fmt.Errorf("no Common Crawl collections found")
		}

		// Store the first (most recent) collection in the global variable
		latestCollectionInfo = &collections[0]
	}

	// Now query for the domain using the latest collection's index URL
	domainURL := fmt.Sprintf("%s?url=%s&output=json", latestCollectionInfo.CDXServer, domain)

	resp, err := httpClient.Get(domainURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching Common Crawl index: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-OK response from Common Crawl: %s", resp.Status)
	}

	var indexes []CommonCrawlCDX
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue // Skip empty lines
		}

		var index CommonCrawlCDX
		if err := json.Unmarshal([]byte(line), &index); err != nil {
			return nil, fmt.Errorf("error decoding Common Crawl index line: %v", err)
		}
		indexes = append(indexes, index)
	}

	if len(indexes) == 0 {
		return nil, fmt.Errorf("no Common Crawl index found for domain: %s", domain)
	}

	if &indexes[0].Redirect != nil {
		return nil, fmt.Errorf("redirect detected, need to fix, skipping : %s", indexes[0].Redirect)
	}

	// Return the first (most recent) index
	return &indexes[0], nil
}

// getContentTypeFromHeaders extracts the Content-Type from HTTP headers
func getContentTypeFromHeaders(headers map[string]string) string {
	// Check for Content-Type header (case-insensitive)
	for key, value := range headers {
		if strings.ToLower(key) == "content-type" {
			// Return just the media type part if there are parameters
			if semicolon := strings.Index(value, ";"); semicolon != -1 {
				return value[:semicolon]
			}
			return value
		}
	}

	// If no Content-Type header found
	return ""
}

// getContentTypeFromWAT extracts content type from WAT metadata for a specific URL
func getContentTypeFromWAT(wat *WAT, url string) string {
	// First check if this is the main page
	mainPageContentType := ""
	for key, value := range wat.Envelope.Payload.ResponseHeaders.Headers {
		if strings.ToLower(key) == "content-type" {
			if semicolon := strings.Index(value, ";"); semicolon != -1 {
				mainPageContentType = value[:semicolon]
			} else {
				mainPageContentType = value
			}
			break
		}
	}

	// If this is the main page content, return its content type
	if mainPageContentType != "" && strings.HasSuffix(url, "/") {
		return mainPageContentType
	}

	// For other resources, we need to make a HEAD request to get the content type
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return guessContentTypeFromURL(url)
	}

	resp, err := client.Do(req)
	if err != nil {
		return guessContentTypeFromURL(url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return guessContentTypeFromURL(url)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		return guessContentTypeFromURL(url)
	}

	// Return just the media type part if there are parameters
	if semicolon := strings.Index(contentType, ";"); semicolon != -1 {
		return contentType[:semicolon]
	}

	return contentType
}

// getResourcesFromWAT fetches and processes the WAT file to extract resources
func getResourcesFromWAT(index *CommonCrawlCDX, domain string) ([]ResourceInfo, error) {
	// Construct the URL to the WAT file
	watURL := fmt.Sprintf("https://data.commoncrawl.org/%s", index.Filename)

	resp, err := httpClient.Get(watURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching WAT file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-OK response from WAT file: %s", resp.Status)
	}

	// Create a gzip reader to decompress the WAT file
	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error creating gzip reader for WAT file: %v", err)
	}
	defer gzipReader.Close()

	var resources []ResourceInfo
	scanner := bufio.NewScanner(gzipReader)

	// Increase the buffer size to handle large lines
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	// WAT files contain multiple JSON records, one per line
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue // Skip empty lines
		}

		// Parse the JSON record
		var wat WAT
		if err := json.Unmarshal([]byte(line), &wat); err != nil {
			// Skip records that can't be parsed
			continue
		}

		// Process each link in the WAT record
		for _, link := range wat.Envelope.Payload.ResponseBody.Links {
			// Skip empty URLs
			if link.URL == "" {
				continue
			}

			// Resolve the server IP for the resource
			host := extractHostFromURL(link.URL)
			ips, err := net.LookupIP(host)
			if err != nil || len(ips) == 0 {
				continue
			}

			// Use the first IP address
			ip := ips[0].String()

			// Determine the CDN
			cdn := getCDNForIP(ip)

			// Get content type from WAT metadata or by making a HEAD request
			contentType := getContentTypeFromWAT(&wat, link.URL)

			// Create the resource info
			resource := ResourceInfo{
				CDN:            cdn,
				OriginalDomain: domain,
				ContentType:    contentType,
				ResourceURL:    link.URL,
				ServerIP:       ip,
			}

			resources = append(resources, resource)
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning WAT file: %v", err)
	}

	return resources, nil
}

// extractHostFromURL extracts the host part from a URL
func extractHostFromURL(urlStr string) string {
	// Simple implementation, in a real app you'd use url.Parse
	if strings.HasPrefix(urlStr, "http://") {
		urlStr = strings.TrimPrefix(urlStr, "http://")
	} else if strings.HasPrefix(urlStr, "https://") {
		urlStr = strings.TrimPrefix(urlStr, "https://")
	}

	// Remove path part
	if idx := strings.Index(urlStr, "/"); idx != -1 {
		urlStr = urlStr[:idx]
	}

	return urlStr
}

// guessContentTypeFromURL makes a simple guess of content type based on URL extension
func guessContentTypeFromURL(urlStr string) string {
	if strings.HasSuffix(urlStr, ".js") {
		return "text/javascript"
	} else if strings.HasSuffix(urlStr, ".css") {
		return "text/css"
	} else if strings.HasSuffix(urlStr, ".jpg") || strings.HasSuffix(urlStr, ".jpeg") {
		return "image/jpeg"
	} else if strings.HasSuffix(urlStr, ".png") {
		return "image/png"
	} else if strings.HasSuffix(urlStr, ".gif") {
		return "image/gif"
	} else if strings.HasSuffix(urlStr, ".html") || strings.HasSuffix(urlStr, ".htm") {
		return "text/html"
	}

	return "application/octet-stream"
}

// processDomain processes a single domain
func processDomain(domain string) ([]ResourceInfo, error) {
	fmt.Printf("Processing domain: %s\n", domain)

	// Get the latest Common Crawl index for the domain
	index, err := getLatestCommonCrawlIndex(domain)
	if err != nil {
		return nil, fmt.Errorf("error getting Common Crawl index: %v", err)
	}

	// Get resources from the WAT file
	resources, err := getResourcesFromWAT(index, domain)
	if err != nil {
		return nil, fmt.Errorf("error getting resources from WAT: %v", err)
	}

	return resources, nil
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

	fmt.Printf("Loaded CDN IP map with %d CDNs\n", len(cdnIPMap))
	return nil
}

func main() {
	// Define command-line flags
	inputFile := flag.String("input", "top-1m.csv", "Path to the input domain list file")
	cdnMapFile := flag.String("cdn-map", "cdn_asn_to_ip_map.json", "Path to the CDN to IP mapping file")
	outputFile := flag.String("output", "resources.json", "Path to the output JSON file")

	// Parse flags
	flag.Parse()

	// Validate flags
	if *inputFile == "" || *cdnMapFile == "" || *outputFile == "" {
		flag.Usage()
		log.Fatal("All file paths are required")
	}

	// Load the CDN to IP mapping
	if err := loadCDNIPMap(*cdnMapFile); err != nil {
		log.Fatalf("Error loading CDN IP map: %v", err)
	}

	// Open the domain list file
	file, err := os.Open(*inputFile)
	if err != nil {
		log.Fatalf("Error opening input file: %v", err)
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Collect all resources
	var allResources []ResourceInfo

	// Process each domain
	for scanner.Scan() {
		line := scanner.Text()

		// Parse the domain from the CSV line (format: rank,domain)
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			log.Printf("Invalid line format: %s", line)
			continue
		}

		domain := parts[1]

		// Process the domain
		resources, err := processDomain(domain)
		if err != nil {
			log.Printf("Error processing domain %s: %v", domain, err)
			continue
		}

		// Add the resources to the collection
		allResources = append(allResources, resources...)

		// Limit the number of domains processed (for testing)
		if len(allResources) > 100 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input file: %v", err)
	}

	// Convert the resources to JSON
	jsonData, err := json.MarshalIndent(allResources, "", "  ")
	if err != nil {
		log.Fatalf("Error converting to JSON: %v", err)
	}

	// Write the JSON to the output file
	if err := ioutil.WriteFile(*outputFile, jsonData, 0644); err != nil {
		log.Fatalf("Error writing to output file: %v", err)
	}

	fmt.Printf("Successfully processed %d domains and wrote %d resources to %s\n",
		len(allResources), len(allResources), *outputFile)
}
